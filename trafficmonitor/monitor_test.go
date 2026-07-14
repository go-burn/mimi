package trafficmonitor

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeSource struct {
	mu          sync.RWMutex
	connections []Connection
}

func (f *fakeSource) Snapshot() []Connection {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return append([]Connection(nil), f.connections...)
}

func (f *fakeSource) set(connections ...Connection) {
	f.mu.Lock()
	f.connections = append([]Connection(nil), connections...)
	f.mu.Unlock()
}

func TestMonitorAggregatesHistoricalTraffic(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "traffic.sqlite")
	source := &fakeSource{}
	monitor, err := New(Options{
		DatabasePath: databasePath, ListenAddress: "127.0.0.1:0", SampleInterval: 10 * time.Millisecond,
	}, source)
	if err != nil {
		t.Fatal(err)
	}
	if err := monitor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	time.Sleep(30 * time.Millisecond)
	source.set(
		Connection{
			ID: "connection-1", Domain: "video.example", DestinationIP: "203.0.113.10",
			Country: "CN", ASN: "AS4134", Node: "香港节点", ProxyChain: "节点选择 → 香港节点",
			Rule: "DomainSuffix", RulePayload: "example", Network: "tcp", Process: "browser",
			Route: RouteProxy, UploadTotal: 1024, DownloadTotal: 4096,
		},
		Connection{ID: "connection-2", Domain: "video.example", Node: "DIRECT", Rule: "Domain", Route: RouteDirect, DownloadTotal: 2048},
		Connection{ID: "connection-3", Domain: "video.example", Node: "REJECT", Rule: "Domain", Route: RouteReject, DownloadTotal: 100},
	)
	time.Sleep(60 * time.Millisecond)
	if err := monitor.Close(); err != nil {
		t.Fatal(err)
	}

	reader, err := New(Options{DatabasePath: databasePath}, &fakeSource{})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := reader.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	rows := getAggregate(t, reader, "domain")
	if len(rows) != 1 || rows[0].Key != "video.example" || rows[0].TotalBytes != 7268 ||
		rows[0].ProxyBytes != 5120 || rows[0].DirectBytes != 2048 || rows[0].RejectBytes != 100 {
		t.Fatalf("unexpected aggregate rows: %+v", rows)
	}
	candidates := getDirectCandidates(t, reader)
	if len(candidates) != 1 || candidates[0].Confidence != "high" || candidates[0].TotalBytes != 5120 {
		t.Fatalf("unexpected DIRECT candidates: %+v", candidates)
	}
	if candidates[0].SuggestedRule != "DOMAIN,video.example,DIRECT" {
		t.Fatalf("unexpected rule: %s", candidates[0].SuggestedRule)
	}
	points := getTimeSeries(t, reader)
	var seriesTotal int64
	for _, point := range points {
		seriesTotal += point.ProxyBytes + point.DirectBytes + point.RejectBytes
	}
	if seriesTotal != 7268 {
		t.Fatalf("unexpected time series total: %d", seriesTotal)
	}
	summary := getSummary(t, reader, "proxy")
	if summary.ProxyBytes != 5120 || summary.DirectBytes != 0 || summary.RejectBytes != 0 {
		t.Fatalf("unexpected filtered summary: %+v", summary)
	}
	regionRows := getAggregate(t, reader, "node_region")
	assertAggregateTotal(t, regionRows, "香港", 5120)
	assertAggregateTotal(t, regionRows, "直连", 2048)
	assertAggregateTotal(t, regionRows, "拒绝", 100)
	countryRows := getAggregate(t, reader, "country")
	assertAggregateTotal(t, countryRows, "CN", 5120)
	assertAggregateTotal(t, countryRows, "(未知)", 2148)
}

func TestDashboardServesHistoricalPageAndEmptyArrays(t *testing.T) {
	monitor, err := New(Options{
		DatabasePath: filepath.Join(t.TempDir(), "traffic.sqlite"), ListenAddress: "127.0.0.1:0",
		SampleInterval: 20 * time.Millisecond,
	}, &fakeSource{})
	if err != nil {
		t.Fatal(err)
	}
	if err := monitor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer monitor.Close()

	response, err := http.Get(monitor.DashboardURL())
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "历史流量报表") ||
		!strings.Contains(string(body), "代理流量 Top 域名") || !strings.Contains(string(body), "节点地区消耗") {
		t.Fatalf("unexpected dashboard response: status=%d", response.StatusCode)
	}

	rows := getAggregate(t, monitor, "domain")
	if rows == nil {
		t.Fatal("empty aggregate list must be encoded as [] instead of null")
	}
	candidates := getDirectCandidates(t, monitor)
	if candidates == nil {
		t.Fatal("empty candidate list must be encoded as [] instead of null")
	}
}

func TestMonitorMigratesExistingTrafficDatabase(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "traffic.sqlite")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(`CREATE TABLE traffic_minute (
		minute INTEGER NOT NULL, domain TEXT NOT NULL, destination_ip TEXT NOT NULL,
		node TEXT NOT NULL, proxy_chain TEXT NOT NULL, rule TEXT NOT NULL, rule_payload TEXT NOT NULL,
		network TEXT NOT NULL, process TEXT NOT NULL, route TEXT NOT NULL,
		upload_bytes INTEGER NOT NULL, download_bytes INTEGER NOT NULL, connection_count INTEGER NOT NULL,
		PRIMARY KEY (minute, domain, destination_ip, node, proxy_chain, rule, rule_payload, network, process, route)
	)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(`INSERT INTO traffic_minute (
		minute, domain, destination_ip, node, proxy_chain, rule, rule_payload, network, process, route,
		upload_bytes, download_bytes, connection_count
	) VALUES
		(1, 'legacy.example', '', 'legacy-node', '', 'Domain', '', 'tcp', 'legacy', 'proxy', 1, 2, 1),
		(2, 'legacy-direct.example', '', '自定义直连', '', 'Domain', '', 'tcp', 'legacy', 'direct', 3, 4, 1)`)
	if err != nil {
		t.Fatal(err)
	}
	database.Close()

	monitor, err := New(Options{DatabasePath: databasePath}, &fakeSource{})
	if err != nil {
		t.Fatal(err)
	}
	if err := monitor.Close(); err != nil {
		t.Fatal(err)
	}

	database, err = sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec(`SELECT destination_country, destination_asn, node_region FROM traffic_minute LIMIT 0`); err != nil {
		t.Fatalf("migration did not add GeoIP/ASN/node region columns: %v", err)
	}
	var nodeRegion string
	if err := database.QueryRow(`SELECT node_region FROM traffic_minute WHERE domain = 'legacy.example'`).Scan(&nodeRegion); err != nil {
		t.Fatal(err)
	}
	if nodeRegion != "其他" {
		t.Fatalf("migration backfilled node_region = %q, want 其他", nodeRegion)
	}
	if err := database.QueryRow(`SELECT node_region FROM traffic_minute WHERE domain = 'legacy-direct.example'`).Scan(&nodeRegion); err != nil {
		t.Fatal(err)
	}
	if nodeRegion != "直连" {
		t.Fatalf("migration backfilled direct node_region = %q, want 直连", nodeRegion)
	}
	var autoVacuum int
	if err := database.QueryRow(`PRAGMA auto_vacuum`).Scan(&autoVacuum); err != nil {
		t.Fatal(err)
	}
	if autoVacuum != 2 {
		t.Fatalf("migration did not enable incremental auto-vacuum: %d", autoVacuum)
	}
	var rows int
	if err := database.QueryRow(`SELECT COUNT(*) FROM traffic_minute`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 2 {
		t.Fatalf("migration did not preserve legacy traffic rows: %d", rows)
	}
	var integrity string
	if err := database.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil {
		t.Fatal(err)
	}
	if integrity != "ok" {
		t.Fatalf("database integrity check failed after migration: %s", integrity)
	}
}

func assertAggregateTotal(t *testing.T, rows []AggregateRow, key string, want int64) {
	t.Helper()
	for _, row := range rows {
		if row.Key == key {
			if row.TotalBytes != want {
				t.Fatalf("aggregate %q total = %d, want %d", key, row.TotalBytes, want)
			}
			return
		}
	}
	t.Fatalf("aggregate %q not found in %+v", key, rows)
}

func getAggregate(t *testing.T, monitor *Monitor, dimension string) []AggregateRow {
	t.Helper()
	response, err := http.Get(monitor.DashboardURL() + "/api/traffic?dimension=" + dimension + "&minutes=1440&limit=10")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var rows []AggregateRow
	if err := json.NewDecoder(response.Body).Decode(&rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

func getDirectCandidates(t *testing.T, monitor *Monitor) []DirectCandidate {
	t.Helper()
	response, err := http.Get(monitor.DashboardURL() + "/api/direct-candidates?minutes=1440&limit=10")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var candidates []DirectCandidate
	if err := json.NewDecoder(response.Body).Decode(&candidates); err != nil {
		t.Fatal(err)
	}
	return candidates
}

func getTimeSeries(t *testing.T, monitor *Monitor) []TimeSeriesPoint {
	t.Helper()
	response, err := http.Get(monitor.DashboardURL() + "/api/timeseries?dimension=domain&minutes=1440&search=video.example")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var points []TimeSeriesPoint
	if err := json.NewDecoder(response.Body).Decode(&points); err != nil {
		t.Fatal(err)
	}
	return points
}

func getSummary(t *testing.T, monitor *Monitor, route string) Summary {
	t.Helper()
	response, err := http.Get(monitor.DashboardURL() + "/api/summary?dimension=domain&minutes=1440&search=video.example&route=" + route)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var summary Summary
	if err := json.NewDecoder(response.Body).Decode(&summary); err != nil {
		t.Fatal(err)
	}
	return summary
}
