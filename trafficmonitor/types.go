package trafficmonitor

import (
	"log/slog"
	"time"
)

type Route string

const (
	RouteProxy  Route = "proxy"
	RouteDirect Route = "direct"
	RouteReject Route = "reject"
)

// Connection 是 Mihomo 采集 seam 上的标准化累计流量快照。
type Connection struct {
	ID            string
	Domain        string
	DestinationIP string
	Country       string
	ASN           string
	Node          string
	NodeRegion    string
	ProxyChain    string
	Rule          string
	RulePayload   string
	Network       string
	Process       string
	Route         Route
	UploadTotal   int64
	DownloadTotal int64
}

type Source interface {
	Snapshot() []Connection
}

type Options struct {
	DatabasePath   string
	ListenAddress  string
	SampleInterval time.Duration
	Retention      time.Duration
	Logger         *slog.Logger
}

type AggregateQuery struct {
	Dimension string
	Minutes   int
	Limit     int
	Route     string
	Search    string
	Sort      string
	Order     string
}

type AggregateRow struct {
	Key           string `json:"key"`
	UploadBytes   int64  `json:"uploadBytes"`
	DownloadBytes int64  `json:"downloadBytes"`
	TotalBytes    int64  `json:"totalBytes"`
	Connections   int64  `json:"connections"`
	ProxyBytes    int64  `json:"proxyBytes"`
	DirectBytes   int64  `json:"directBytes"`
	RejectBytes   int64  `json:"rejectBytes"`
}

type Summary struct {
	UploadBytes   int64 `json:"uploadBytes"`
	DownloadBytes int64 `json:"downloadBytes"`
	ProxyBytes    int64 `json:"proxyBytes"`
	DirectBytes   int64 `json:"directBytes"`
	RejectBytes   int64 `json:"rejectBytes"`
}

type TimeSeriesPoint struct {
	Timestamp     int64 `json:"timestamp"`
	UploadBytes   int64 `json:"uploadBytes"`
	DownloadBytes int64 `json:"downloadBytes"`
	ProxyBytes    int64 `json:"proxyBytes"`
	DirectBytes   int64 `json:"directBytes"`
	RejectBytes   int64 `json:"rejectBytes"`
}

type DirectCandidate struct {
	Domain        string    `json:"domain"`
	UploadBytes   int64     `json:"uploadBytes"`
	DownloadBytes int64     `json:"downloadBytes"`
	TotalBytes    int64     `json:"totalBytes"`
	Connections   int64     `json:"connections"`
	LastSeen      time.Time `json:"lastSeen"`
	Nodes         string    `json:"nodes"`
	Rules         string    `json:"rules"`
	Countries     string    `json:"countries"`
	ASNs          string    `json:"asns"`
	Confidence    string    `json:"confidence"`
	Reason        string    `json:"reason"`
	SuggestedRule string    `json:"suggestedRule"`
}
