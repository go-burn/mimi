package trafficmonitor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestAggregateRankingSupportsMetricSorting(t *testing.T) {
	monitor := newSortingTestMonitor(t)

	tests := []struct {
		name  string
		sort  string
		order string
		want  []string
	}{
		{name: "total descending", sort: "total", order: "desc", want: []string{"beta.example", "alpha.example", "gamma.example"}},
		{name: "total ascending", sort: "total", order: "asc", want: []string{"gamma.example", "alpha.example", "beta.example"}},
		{name: "upload descending", sort: "upload", order: "desc", want: []string{"alpha.example", "gamma.example", "beta.example"}},
		{name: "upload ascending", sort: "upload", order: "asc", want: []string{"beta.example", "gamma.example", "alpha.example"}},
		{name: "download descending", sort: "download", order: "desc", want: []string{"beta.example", "gamma.example", "alpha.example"}},
		{name: "download ascending", sort: "download", order: "asc", want: []string{"alpha.example", "gamma.example", "beta.example"}},
		{name: "proxy descending", sort: "proxy", order: "desc", want: []string{"beta.example", "alpha.example", "gamma.example"}},
		{name: "proxy ascending", sort: "proxy", order: "asc", want: []string{"gamma.example", "alpha.example", "beta.example"}},
		{name: "direct descending", sort: "direct", order: "desc", want: []string{"gamma.example", "beta.example", "alpha.example"}},
		{name: "direct ascending", sort: "direct", order: "asc", want: []string{"alpha.example", "beta.example", "gamma.example"}},
		{name: "reject descending", sort: "reject", order: "desc", want: []string{"alpha.example", "beta.example", "gamma.example"}},
		{name: "reject ascending", sort: "reject", order: "asc", want: []string{"beta.example", "gamma.example", "alpha.example"}},
		{name: "connections descending", sort: "connections", order: "desc", want: []string{"gamma.example", "beta.example", "alpha.example"}},
		{name: "connections ascending", sort: "connections", order: "asc", want: []string{"alpha.example", "beta.example", "gamma.example"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rows := requestAggregateRows(t, monitor, url.Values{
				"sort":  {test.sort},
				"order": {test.order},
			})
			if got := aggregateKeys(rows); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("unexpected ranking: got %v, want %v", got, test.want)
			}
		})
	}
}

func TestAggregateRankingInvalidSortFallsBackSafely(t *testing.T) {
	monitor := newSortingTestMonitor(t)
	malicious := `total DESC; DROP TABLE traffic_minute; --`

	rows := requestAggregateRows(t, monitor, url.Values{
		"sort":  {malicious},
		"order": {`asc; DROP TABLE traffic_minute; --`},
	})
	want := []string{"beta.example", "alpha.example", "gamma.example"}
	if got := aggregateKeys(rows); !reflect.DeepEqual(got, want) {
		t.Fatalf("invalid sort must use total descending: got %v, want %v", got, want)
	}

	// A subsequent valid query also proves the attempted SQL was never executed.
	rows = requestAggregateRows(t, monitor, url.Values{
		"sort":  {"upload"},
		"order": {"desc"},
	})
	if got := aggregateKeys(rows); !reflect.DeepEqual(got, []string{"alpha.example", "gamma.example", "beta.example"}) {
		t.Fatalf("database changed after invalid sort: %v", got)
	}
}

func TestOverviewProxyReportsUseProxyOnlyTraffic(t *testing.T) {
	monitor := newSortingTestMonitor(t)

	domains := requestAggregateRows(t, monitor, url.Values{
		"dimension": {"domain"},
		"route":     {"proxy"},
		"sort":      {"proxy"},
		"order":     {"desc"},
		"limit":     {"1"},
	})
	if len(domains) != 1 || domains[0].Key != "beta.example" || domains[0].ProxyBytes != 1500 || domains[0].DirectBytes != 0 {
		t.Fatalf("unexpected proxy domain report: %+v", domains)
	}

	regions := requestAggregateRows(t, monitor, url.Values{
		"dimension": {"node_region"},
		"route":     {"proxy"},
		"sort":      {"proxy"},
		"order":     {"desc"},
	})
	if got := aggregateKeys(regions); !reflect.DeepEqual(got, []string{"日本", "香港"}) {
		t.Fatalf("unexpected proxy node region report: %v", got)
	}
	if regions[0].ProxyBytes != 1500 || regions[1].ProxyBytes != 100 {
		t.Fatalf("unexpected proxy node region totals: %+v", regions)
	}
}

func newSortingTestMonitor(t *testing.T) *Monitor {
	t.Helper()
	database, err := openStore(filepath.Join(t.TempDir(), "traffic.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.close(); err != nil {
			t.Error(err)
		}
	})

	minute := time.Now().Truncate(time.Minute).Unix()
	buckets := []minuteBucket{
		{Minute: minute, Domain: "alpha.example", NodeRegion: "香港", Route: RouteProxy, UploadBytes: 100, ConnectionCount: 1},
		{Minute: minute, Domain: "alpha.example", NodeRegion: "直连", Route: RouteDirect, UploadBytes: 300, ConnectionCount: 1},
		{Minute: minute, Domain: "alpha.example", NodeRegion: "拒绝", Route: RouteReject, UploadBytes: 500, DownloadBytes: 100, ConnectionCount: 1},
		{Minute: minute, Domain: "beta.example", NodeRegion: "日本", Route: RouteProxy, UploadBytes: 50, DownloadBytes: 1450, ConnectionCount: 2},
		{Minute: minute, Domain: "beta.example", NodeRegion: "直连", Route: RouteDirect, UploadBytes: 50, DownloadBytes: 450, ConnectionCount: 3},
		{Minute: minute, Domain: "gamma.example", NodeRegion: "直连", Route: RouteDirect, UploadBytes: 300, DownloadBytes: 250, ConnectionCount: 9},
	}
	if err := database.upsertBuckets(context.Background(), buckets); err != nil {
		t.Fatal(err)
	}
	return &Monitor{store: database}
}

func requestAggregateRows(t *testing.T, monitor *Monitor, values url.Values) []AggregateRow {
	t.Helper()
	if values.Get("dimension") == "" {
		values.Set("dimension", "domain")
	}
	values.Set("minutes", "1440")
	if values.Get("limit") == "" {
		values.Set("limit", "10")
	}
	request := httptest.NewRequest(http.MethodGet, "/api/traffic?"+values.Encode(), nil)
	response := httptest.NewRecorder()
	monitor.handleAggregate(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected response: status=%d body=%s", response.Code, response.Body.String())
	}
	var rows []AggregateRow
	if err := json.NewDecoder(response.Body).Decode(&rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

func aggregateKeys(rows []AggregateRow) []string {
	keys := make([]string, len(rows))
	for index, row := range rows {
		keys[index] = row.Key
	}
	return keys
}
