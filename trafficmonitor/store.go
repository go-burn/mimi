package trafficmonitor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type minuteBucket struct {
	Minute          int64
	Domain          string
	DestinationIP   string
	Country         string
	ASN             string
	Node            string
	NodeRegion      string
	ProxyChain      string
	Rule            string
	RulePayload     string
	Network         string
	Process         string
	Route           Route
	UploadBytes     int64
	DownloadBytes   int64
	ConnectionCount int64
}

type store struct {
	db             *sql.DB
	maintenanceErr error
}

// 每次维护最多回收约 16 MiB（按 SQLite 默认 4 KiB 页计算），避免长时间阻塞报表查询。
const maxIncrementalVacuumPages = 4096

const cleanupBatchRows = 5000

var errVacuumPagesRemaining = errors.New("流量数据库仍有待回收空闲页")

func openStore(path string) (*store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开流量数据库失败: %w", err)
	}
	db.SetMaxOpenConns(1)

	s := &store{db: db}
	if err := s.init(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *store) init() error {
	if _, err := s.db.Exec(`PRAGMA auto_vacuum=INCREMENTAL`); err != nil {
		s.maintenanceErr = fmt.Errorf("设置增量压缩模式失败: %w", err)
	}
	statements := []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA synchronous=NORMAL`,
		`PRAGMA busy_timeout=5000`,
		`CREATE TABLE IF NOT EXISTS traffic_minute (
			minute INTEGER NOT NULL,
			domain TEXT NOT NULL,
			destination_ip TEXT NOT NULL,
			destination_country TEXT NOT NULL DEFAULT '',
			destination_asn TEXT NOT NULL DEFAULT '',
			node TEXT NOT NULL,
			node_region TEXT NOT NULL DEFAULT '',
			proxy_chain TEXT NOT NULL,
			rule TEXT NOT NULL,
			rule_payload TEXT NOT NULL,
			network TEXT NOT NULL,
			process TEXT NOT NULL,
			route TEXT NOT NULL,
			upload_bytes INTEGER NOT NULL,
			download_bytes INTEGER NOT NULL,
			connection_count INTEGER NOT NULL,
			PRIMARY KEY (minute, domain, destination_ip, node, proxy_chain, rule, rule_payload, network, process, route)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_traffic_minute_time ON traffic_minute(minute)`,
		`CREATE INDEX IF NOT EXISTS idx_traffic_minute_domain ON traffic_minute(domain, minute)`,
		`CREATE INDEX IF NOT EXISTS idx_traffic_minute_ip ON traffic_minute(destination_ip, minute)`,
		`CREATE INDEX IF NOT EXISTS idx_traffic_minute_node ON traffic_minute(node, minute)`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return fmt.Errorf("初始化流量数据库失败: %w", err)
		}
	}
	if err := s.ensureColumn("traffic_minute", "destination_country", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("升级流量数据库字段失败: %w", err)
	}
	if err := s.ensureColumn("traffic_minute", "destination_asn", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("升级流量数据库字段失败: %w", err)
	}
	if err := s.ensureColumn("traffic_minute", "node_region", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("升级流量数据库字段失败: %w", err)
	}
	if err := s.backfillNodeRegions(context.Background()); err != nil {
		return fmt.Errorf("回填历史流量节点地区失败: %w", err)
	}
	if err := s.ensureIncrementalAutoVacuum(); err != nil {
		s.maintenanceErr = fmt.Errorf("启用流量数据库增量压缩失败: %w", err)
	} else {
		s.maintenanceErr = nil
	}

	return nil
}

func (s *store) backfillNodeRegions(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT node, route FROM traffic_minute
		WHERE node_region IS NULL OR node_region = ''`)
	if err != nil {
		return err
	}
	type nodeRoute struct {
		node  string
		route Route
	}
	var nodes []nodeRoute
	for rows.Next() {
		var item nodeRoute
		if err := rows.Scan(&item.node, &item.route); err != nil {
			rows.Close()
			return err
		}
		nodes = append(nodes, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(nodes) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statement, err := tx.PrepareContext(ctx, `UPDATE traffic_minute SET node_region = ?
		WHERE node = ? AND route = ? AND (node_region IS NULL OR node_region = '')`)
	if err != nil {
		return err
	}
	defer statement.Close()
	for _, item := range nodes {
		if _, err := statement.ExecContext(ctx, NodeRegionForRoute(item.node, item.route), item.node, item.route); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *store) ensureIncrementalAutoVacuum() error {
	connection, err := s.db.Conn(context.Background())
	if err != nil {
		return err
	}
	defer connection.Close()
	return ensureIncrementalAutoVacuumConnection(context.Background(), connection)
}

func ensureIncrementalAutoVacuumConnection(ctx context.Context, connection *sql.Conn) error {
	var mode int
	if err := connection.QueryRowContext(ctx, `PRAGMA auto_vacuum`).Scan(&mode); err != nil {
		return err
	}
	if mode == 2 {
		return nil
	}
	if _, err := connection.ExecContext(ctx, `PRAGMA auto_vacuum=INCREMENTAL`); err != nil {
		return err
	}
	if mode == 0 {
		if err := checkpointWALConnection(ctx, connection); err != nil {
			return err
		}
		if _, err := connection.ExecContext(ctx, `VACUUM`); err != nil {
			return err
		}
		if err := checkpointWALConnection(ctx, connection); err != nil {
			return err
		}
	}
	if err := connection.QueryRowContext(ctx, `PRAGMA auto_vacuum`).Scan(&mode); err != nil {
		return err
	}
	if mode != 2 {
		return fmt.Errorf("auto_vacuum 模式为 %d，期望为 2", mode)
	}
	return nil
}

func (s *store) checkpointWAL(ctx context.Context) error {
	connection, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer connection.Close()
	return checkpointWALConnection(ctx, connection)
}

func checkpointWALConnection(ctx context.Context, connection *sql.Conn) error {
	var busy, logFrames, checkpointedFrames int
	if err := connection.QueryRowContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &logFrames, &checkpointedFrames); err != nil {
		return err
	}
	if busy != 0 {
		return fmt.Errorf("WAL 检查点繁忙，剩余日志页 %d，已写回 %d", logFrames, checkpointedFrames)
	}
	return nil
}

func (s *store) close() error {
	if s == nil || s.db == nil {
		return nil
	}
	_, _ = s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	return s.db.Close()
}

func (s *store) ensureColumn(table, column, definition string) error {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, primaryKey int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	_, err = s.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + definition)
	return err
}

func (s *store) upsertBuckets(ctx context.Context, buckets []minuteBucket) error {
	if len(buckets) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	statement, err := tx.PrepareContext(ctx, `INSERT INTO traffic_minute (
		minute, domain, destination_ip, destination_country, destination_asn, node, node_region, proxy_chain, rule, rule_payload, network, process, route,
		upload_bytes, download_bytes, connection_count
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(minute, domain, destination_ip, node, proxy_chain, rule, rule_payload, network, process, route)
	DO UPDATE SET
		upload_bytes = upload_bytes + excluded.upload_bytes,
		download_bytes = download_bytes + excluded.download_bytes,
		connection_count = connection_count + excluded.connection_count,
		destination_country = CASE WHEN excluded.destination_country != '' THEN excluded.destination_country ELSE destination_country END,
		destination_asn = CASE WHEN excluded.destination_asn != '' THEN excluded.destination_asn ELSE destination_asn END,
		node_region = CASE WHEN excluded.node_region != '' THEN excluded.node_region ELSE node_region END`)
	if err != nil {
		return err
	}
	defer statement.Close()

	for _, bucket := range buckets {
		if _, err := statement.ExecContext(ctx,
			bucket.Minute, bucket.Domain, bucket.DestinationIP, bucket.Country, bucket.ASN, bucket.Node, bucket.NodeRegion, bucket.ProxyChain,
			bucket.Rule, bucket.RulePayload, bucket.Network, bucket.Process, bucket.Route,
			bucket.UploadBytes, bucket.DownloadBytes, bucket.ConnectionCount,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *store) aggregate(ctx context.Context, query AggregateQuery, now time.Time) ([]AggregateRow, error) {
	query, column := normalizeReportQuery(query)
	where, args := reportWhere(query, column, now)
	args = append(args, query.Limit)
	sortExpression, sortDirection := aggregateSortSQL(query)

	statement := fmt.Sprintf(`SELECT CASE WHEN %s = '' THEN '(未知)' ELSE %s END AS dimension_value,
		SUM(upload_bytes) AS upload_bytes,
		SUM(download_bytes) AS download_bytes,
		SUM(upload_bytes + download_bytes) AS total_bytes,
		SUM(connection_count) AS connections,
		SUM(CASE WHEN route = 'proxy' THEN upload_bytes + download_bytes ELSE 0 END) AS proxy_bytes,
		SUM(CASE WHEN route = 'direct' THEN upload_bytes + download_bytes ELSE 0 END) AS direct_bytes,
		SUM(CASE WHEN route = 'reject' THEN upload_bytes + download_bytes ELSE 0 END) AS reject_bytes
		FROM traffic_minute WHERE %s
		GROUP BY %s
		ORDER BY %s %s, dimension_value COLLATE NOCASE ASC, dimension_value ASC, %s ASC
		LIMIT ?`,
		column, column, strings.Join(where, " AND "), column, sortExpression, sortDirection, column)
	rows, err := s.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]AggregateRow, 0)
	for rows.Next() {
		var item AggregateRow
		if err := rows.Scan(
			&item.Key, &item.UploadBytes, &item.DownloadBytes, &item.TotalBytes, &item.Connections,
			&item.ProxyBytes, &item.DirectBytes, &item.RejectBytes,
		); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *store) summary(ctx context.Context, query AggregateQuery, now time.Time) (Summary, error) {
	query, column := normalizeReportQuery(query)
	where, args := reportWhere(query, column, now)
	var summary Summary
	err := s.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(upload_bytes), 0),
		COALESCE(SUM(download_bytes), 0),
		COALESCE(SUM(CASE WHEN route = 'proxy' THEN upload_bytes + download_bytes ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN route = 'direct' THEN upload_bytes + download_bytes ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN route = 'reject' THEN upload_bytes + download_bytes ELSE 0 END), 0)
		FROM traffic_minute WHERE `+strings.Join(where, " AND "), args...).Scan(
		&summary.UploadBytes, &summary.DownloadBytes, &summary.ProxyBytes, &summary.DirectBytes, &summary.RejectBytes,
	)
	return summary, err
}

func (s *store) timeSeries(ctx context.Context, query AggregateQuery, now time.Time) ([]TimeSeriesPoint, error) {
	query, column := normalizeReportQuery(query)
	where, args := reportWhere(query, column, now)
	bucketSeconds := reportBucketSeconds(query.Minutes)
	queryArgs := []any{bucketSeconds, bucketSeconds}
	queryArgs = append(queryArgs, args...)
	rows, err := s.db.QueryContext(ctx, `SELECT
		CAST(minute / ? AS INTEGER) * ? AS bucket,
		SUM(upload_bytes), SUM(download_bytes),
		SUM(CASE WHEN route = 'proxy' THEN upload_bytes + download_bytes ELSE 0 END),
		SUM(CASE WHEN route = 'direct' THEN upload_bytes + download_bytes ELSE 0 END),
		SUM(CASE WHEN route = 'reject' THEN upload_bytes + download_bytes ELSE 0 END)
		FROM traffic_minute WHERE `+strings.Join(where, " AND ")+`
		GROUP BY bucket ORDER BY bucket`, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byTimestamp := make(map[int64]TimeSeriesPoint)
	for rows.Next() {
		var point TimeSeriesPoint
		if err := rows.Scan(
			&point.Timestamp, &point.UploadBytes, &point.DownloadBytes,
			&point.ProxyBytes, &point.DirectBytes, &point.RejectBytes,
		); err != nil {
			return nil, err
		}
		byTimestamp[point.Timestamp] = point
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	start := now.Add(-time.Duration(query.Minutes) * time.Minute).Unix()
	start = (start / bucketSeconds) * bucketSeconds
	end := (now.Unix() / bucketSeconds) * bucketSeconds
	result := make([]TimeSeriesPoint, 0, int((end-start)/bucketSeconds)+1)
	for timestamp := start; timestamp <= end; timestamp += bucketSeconds {
		point := byTimestamp[timestamp]
		point.Timestamp = timestamp
		result = append(result, point)
	}
	return result, nil
}

func normalizeReportQuery(query AggregateQuery) (AggregateQuery, string) {
	columns := map[string]string{
		"domain": "domain", "ip": "destination_ip", "country": "destination_country",
		"node": "node", "node_region": "node_region",
		"proxy": "proxy_chain", "rule": "rule", "process": "process",
	}
	column, ok := columns[query.Dimension]
	if !ok {
		query.Dimension = "domain"
		column = "domain"
	}
	if query.Minutes <= 0 || query.Minutes > 60*24*90 {
		query.Minutes = 1440
	}
	if query.Limit <= 0 || query.Limit > 500 {
		query.Limit = 100
	}
	switch query.Sort {
	case "total", "upload", "download", "proxy", "direct", "reject", "connections":
	default:
		query.Sort = "total"
	}
	switch query.Order {
	case "asc", "desc":
	default:
		query.Order = "desc"
	}
	return query, column
}

// aggregateSortSQL returns SQL fragments selected exclusively from fixed
// allowlists. Query values are never interpolated into the statement.
func aggregateSortSQL(query AggregateQuery) (expression, direction string) {
	expressions := map[string]string{
		"total":       "total_bytes",
		"upload":      "upload_bytes",
		"download":    "download_bytes",
		"proxy":       "proxy_bytes",
		"direct":      "direct_bytes",
		"reject":      "reject_bytes",
		"connections": "connections",
	}
	expression = expressions[query.Sort]
	if expression == "" {
		expression = expressions["total"]
	}
	if query.Order == "asc" {
		return expression, "ASC"
	}
	return expression, "DESC"
}

func reportWhere(query AggregateQuery, column string, now time.Time) ([]string, []any) {
	where := []string{"minute >= ?"}
	args := []any{now.Add(-time.Duration(query.Minutes) * time.Minute).Truncate(time.Minute).Unix()}
	if query.Route == string(RouteProxy) || query.Route == string(RouteDirect) || query.Route == string(RouteReject) {
		where = append(where, "route = ?")
		args = append(args, query.Route)
	}
	if query.Search != "" {
		where = append(where, column+" LIKE ? ESCAPE '\\'")
		args = append(args, "%"+escapeLike(query.Search)+"%")
	}
	return where, args
}

func reportBucketSeconds(minutes int) int64 {
	switch {
	case minutes <= 60:
		return int64((5 * time.Minute).Seconds())
	case minutes <= 360:
		return int64((15 * time.Minute).Seconds())
	case minutes <= 1440:
		return int64(time.Hour.Seconds())
	case minutes <= 10080:
		return int64((6 * time.Hour).Seconds())
	default:
		return int64((24 * time.Hour).Seconds())
	}
}

func (s *store) directCandidates(ctx context.Context, minutes, limit int, search string, now time.Time) ([]DirectCandidate, error) {
	if minutes <= 0 || minutes > 60*24*90 {
		minutes = 1440
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	where := []string{"minute >= ?", "route = 'proxy'", "domain != ''"}
	args := []any{now.Add(-time.Duration(minutes) * time.Minute).Truncate(time.Minute).Unix()}
	if search != "" {
		where = append(where, "domain LIKE ? ESCAPE '\\'")
		args = append(args, "%"+escapeLike(search)+"%")
	}
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, `SELECT domain,
		SUM(upload_bytes), SUM(download_bytes), SUM(upload_bytes + download_bytes), SUM(connection_count),
		MAX(minute), GROUP_CONCAT(DISTINCT node), GROUP_CONCAT(DISTINCT rule),
		GROUP_CONCAT(DISTINCT destination_country), GROUP_CONCAT(DISTINCT destination_asn)
		FROM traffic_minute WHERE `+strings.Join(where, " AND ")+`
		GROUP BY domain ORDER BY SUM(upload_bytes + download_bytes) DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]DirectCandidate, 0)
	for rows.Next() {
		var candidate DirectCandidate
		var lastSeen int64
		if err := rows.Scan(
			&candidate.Domain, &candidate.UploadBytes, &candidate.DownloadBytes, &candidate.TotalBytes,
			&candidate.Connections, &lastSeen, &candidate.Nodes, &candidate.Rules,
			&candidate.Countries, &candidate.ASNs,
		); err != nil {
			return nil, err
		}
		candidate.LastSeen = time.Unix(lastSeen, 0)
		candidate.SuggestedRule = "DOMAIN," + candidate.Domain + ",DIRECT"
		candidate.Confidence, candidate.Reason = classifyDirectCandidate(candidate)
		result = append(result, candidate)
	}
	return result, rows.Err()
}

func classifyDirectCandidate(candidate DirectCandidate) (confidence, reason string) {
	countries := strings.ToUpper(candidate.Countries)
	rules := strings.ToUpper(candidate.Rules)
	switch {
	case containsListValue(countries, "CN"):
		return "high", "历史记录曾出现 CN GeoIP，优先验证直连"
	case strings.HasSuffix(strings.ToLower(candidate.Domain), ".cn"):
		return "medium", "域名使用 .cn 后缀，可能适合直连"
	case strings.Contains(rules, "MATCH") || strings.Contains(rules, "FINAL"):
		return "medium", "历史记录曾由 MATCH / FINAL 规则代理，值得单独验证"
	default:
		return "review", "历史记录中消耗了代理流量，需验证直连可用性"
	}
}

func containsListValue(values, target string) bool {
	for _, value := range strings.FieldsFunc(values, func(r rune) bool { return r == ',' || r == '/' }) {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

func (s *store) cleanup(ctx context.Context, before time.Time) error {
	cutoff := before.Truncate(time.Minute).Unix()
	for {
		result, err := s.db.ExecContext(ctx, `DELETE FROM traffic_minute WHERE rowid IN (
			SELECT rowid FROM traffic_minute WHERE minute < ? ORDER BY minute LIMIT ?
		)`, cutoff, cleanupBatchRows)
		if err != nil {
			return err
		}
		deleted, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if deleted < cleanupBatchRows {
			break
		}
	}

	connection, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer connection.Close()
	if err := ensureIncrementalAutoVacuumConnection(ctx, connection); err != nil {
		return fmt.Errorf("确认流量数据库增量压缩模式失败: %w", err)
	}
	var freePages int
	if err := connection.QueryRowContext(ctx, `PRAGMA freelist_count`).Scan(&freePages); err != nil {
		return fmt.Errorf("读取流量数据库空闲页失败: %w", err)
	}
	if freePages > 0 {
		pages := min(freePages, maxIncrementalVacuumPages)
		rows, err := connection.QueryContext(ctx, fmt.Sprintf(`PRAGMA incremental_vacuum(%d)`, pages))
		if err != nil {
			return fmt.Errorf("回收流量数据库空闲页失败: %w", err)
		}
		for rows.Next() {
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("回收流量数据库空闲页失败: %w", err)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("回收流量数据库空闲页失败: %w", err)
		}
	}
	var remainingPages int
	if err := connection.QueryRowContext(ctx, `PRAGMA freelist_count`).Scan(&remainingPages); err != nil {
		return fmt.Errorf("确认流量数据库剩余空闲页失败: %w", err)
	}
	if err := checkpointWALConnection(ctx, connection); err != nil {
		return fmt.Errorf("截断流量数据库 WAL 失败: %w", err)
	}
	if remainingPages > 0 {
		return fmt.Errorf("%w: %d", errVacuumPagesRemaining, remainingPages)
	}
	return nil
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	return strings.ReplaceAll(value, `_`, `\_`)
}
