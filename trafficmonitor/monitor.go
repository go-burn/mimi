package trafficmonitor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

type connectionCounter struct {
	upload   int64
	download int64
}

type bucketKey struct {
	minute        int64
	domain        string
	destinationIP string
	node          string
	proxyChain    string
	rule          string
	rulePayload   string
	network       string
	process       string
	route         Route
}

type aggregateBucket struct {
	upload      int64
	download    int64
	country     string
	asn         string
	nodeRegion  string
	connections map[string]struct{}
}

// Monitor 将采样、分钟聚合、SQLite 查询和内置 Web 隐藏在一个小接口后。
type Monitor struct {
	options Options
	source  Source
	store   *store
	logger  *slog.Logger

	stateMu   sync.Mutex
	started   bool
	cancel    context.CancelFunc
	server    *http.Server
	url       string
	wg        sync.WaitGroup
	closeOnce sync.Once
	closeErr  error

	initialized        bool
	previous           map[string]connectionCounter
	buckets            map[bucketKey]*aggregateBucket
	lastCleanup        time.Time
	lastCleanupAttempt time.Time
}

func New(options Options, source Source) (*Monitor, error) {
	if source == nil {
		return nil, errors.New("流量采集源不能为空")
	}
	if options.DatabasePath == "" {
		return nil, errors.New("流量数据库路径不能为空")
	}
	if options.ListenAddress == "" {
		options.ListenAddress = "127.0.0.1:0"
	}
	if options.SampleInterval <= 0 {
		options.SampleInterval = time.Second
	}
	if options.Retention <= 0 {
		options.Retention = 30 * 24 * time.Hour
	}
	if options.Logger == nil {
		options.Logger = slog.Default()
	}

	database, err := openStore(options.DatabasePath)
	if err != nil {
		return nil, err
	}
	if database.maintenanceErr != nil {
		options.Logger.Warn("流量数据库压缩初始化失败，仍将继续删除过期数据", "error", database.maintenanceErr)
	}
	return &Monitor{
		options:  options,
		source:   source,
		store:    database,
		logger:   options.Logger,
		previous: make(map[string]connectionCounter),
		buckets:  make(map[bucketKey]*aggregateBucket),
	}, nil
}

func (m *Monitor) Start(parent context.Context) error {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	if m.started {
		return nil
	}

	listener, err := net.Listen("tcp", m.options.ListenAddress)
	if err != nil {
		return fmt.Errorf("启动历史流量面板失败: %w", err)
	}
	ctx, cancel := context.WithCancel(parent)
	m.cancel = cancel
	m.url = "http://" + listener.Addr().String()
	m.server = &http.Server{
		Handler:           m.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	m.started = true

	m.wg.Add(2)
	go m.sampleLoop(ctx)
	go func() {
		defer m.wg.Done()
		if serveErr := m.server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			m.logger.Error("历史流量面板异常退出", "error", serveErr)
		}
	}()
	m.logger.Info("历史流量统计已启动", "database", m.options.DatabasePath, "dashboard", m.url)
	return nil
}

func (m *Monitor) Close() error {
	m.closeOnce.Do(func() {
		m.stateMu.Lock()
		cancel := m.cancel
		server := m.server
		started := m.started
		m.stateMu.Unlock()

		if cancel != nil {
			cancel()
		}
		if server != nil {
			ctx, stop := context.WithTimeout(context.Background(), 3*time.Second)
			_ = server.Shutdown(ctx)
			stop()
		}
		if started {
			m.wg.Wait()
		}
		if err := m.flushBuckets(context.Background(), 0); err != nil {
			m.closeErr = err
		}
		if err := m.store.close(); err != nil && m.closeErr == nil {
			m.closeErr = err
		}
	})
	return m.closeErr
}

func (m *Monitor) DashboardURL() string {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	return m.url
}

func (m *Monitor) aggregate(ctx context.Context, query AggregateQuery) ([]AggregateRow, error) {
	return m.store.aggregate(ctx, query, time.Now())
}

func (m *Monitor) summary(ctx context.Context, query AggregateQuery) (Summary, error) {
	return m.store.summary(ctx, query, time.Now())
}

func (m *Monitor) timeSeries(ctx context.Context, query AggregateQuery) ([]TimeSeriesPoint, error) {
	return m.store.timeSeries(ctx, query, time.Now())
}

func (m *Monitor) directCandidates(ctx context.Context, minutes, limit int, search string) ([]DirectCandidate, error) {
	return m.store.directCandidates(ctx, minutes, limit, search, time.Now())
}

func (m *Monitor) sampleLoop(ctx context.Context) {
	defer m.wg.Done()
	m.sample(ctx, time.Now())
	ticker := time.NewTicker(m.options.SampleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			m.sample(ctx, now)
		}
	}
}

func (m *Monitor) sample(ctx context.Context, now time.Time) {
	minute := now.Truncate(time.Minute).Unix()
	if err := m.flushBuckets(ctx, minute); err != nil {
		m.logger.Error("写入分钟流量聚合失败", "error", err)
	}

	connections := m.source.Snapshot()
	current := make(map[string]connectionCounter, len(connections))
	for _, connection := range connections {
		counter := connectionCounter{upload: connection.UploadTotal, download: connection.DownloadTotal}
		current[connection.ID] = counter
		previous, existed := m.previous[connection.ID]
		if !m.initialized {
			continue
		}

		uploadDelta := counter.upload
		downloadDelta := counter.download
		if existed {
			uploadDelta = counter.upload - previous.upload
			downloadDelta = counter.download - previous.download
			if uploadDelta < 0 {
				uploadDelta = counter.upload
			}
			if downloadDelta < 0 {
				downloadDelta = counter.download
			}
		}
		if uploadDelta == 0 && downloadDelta == 0 {
			continue
		}

		m.addToBucket(minute, connection, uploadDelta, downloadDelta)
	}

	m.initialized = true
	m.previous = current

	cleanupDue := m.lastCleanup.IsZero() || now.Sub(m.lastCleanup) >= 24*time.Hour
	retryReady := m.lastCleanupAttempt.IsZero() || now.Sub(m.lastCleanupAttempt) >= time.Hour
	if cleanupDue && retryReady {
		m.lastCleanupAttempt = now
		if err := m.store.cleanup(ctx, now.Add(-m.options.Retention)); err != nil {
			if errors.Is(err, errVacuumPagesRemaining) {
				m.logger.Debug("流量数据库仍有空闲页待回收，将在一小时后继续", "error", err)
			} else {
				m.logger.Warn("历史流量数据库维护失败，将在一小时后重试", "error", err)
			}
		} else {
			m.lastCleanup = now
		}
	}
}

func (m *Monitor) addToBucket(minute int64, connection Connection, upload, download int64) {
	key := bucketKey{
		minute: minute, domain: connection.Domain, destinationIP: connection.DestinationIP,
		node: connection.Node, proxyChain: connection.ProxyChain, rule: connection.Rule,
		rulePayload: connection.RulePayload, network: connection.Network, process: connection.Process,
		route: connection.Route,
	}
	bucket := m.buckets[key]
	if bucket == nil {
		bucket = &aggregateBucket{connections: make(map[string]struct{})}
		m.buckets[key] = bucket
	}
	bucket.upload += upload
	bucket.download += download
	if connection.Country != "" {
		bucket.country = connection.Country
	}
	if connection.ASN != "" {
		bucket.asn = connection.ASN
	}
	nodeRegion := connection.NodeRegion
	if connection.Route == RouteDirect || connection.Route == RouteReject || nodeRegion == "" {
		nodeRegion = NodeRegionForRoute(connection.Node, connection.Route)
	}
	if nodeRegion != "" {
		bucket.nodeRegion = nodeRegion
	}
	bucket.connections[connection.ID] = struct{}{}
}

func (m *Monitor) flushBuckets(ctx context.Context, beforeMinute int64) error {
	var rows []minuteBucket
	var keys []bucketKey
	for key, bucket := range m.buckets {
		if beforeMinute != 0 && key.minute >= beforeMinute {
			continue
		}
		rows = append(rows, minuteBucket{
			Minute: key.minute, Domain: key.domain, DestinationIP: key.destinationIP,
			Country: bucket.country, ASN: bucket.asn, Node: key.node, NodeRegion: bucket.nodeRegion, ProxyChain: key.proxyChain,
			Rule: key.rule, RulePayload: key.rulePayload, Network: key.network, Process: key.process,
			Route: key.route, UploadBytes: bucket.upload, DownloadBytes: bucket.download,
			ConnectionCount: int64(len(bucket.connections)),
		})
		keys = append(keys, key)
	}
	if err := m.store.upsertBuckets(ctx, rows); err != nil {
		return err
	}
	for _, key := range keys {
		delete(m.buckets, key)
	}
	return nil
}
