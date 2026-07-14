package main

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	appConfig "mimi/config"
	"mimi/trafficmonitor"

	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/tunnel/statistic"
)

var trafficMonitor atomic.Pointer[trafficmonitor.Monitor]

type trafficProxyRouteTable map[string]trafficmonitor.Route

var trafficProxyRoutes atomic.Pointer[trafficProxyRouteTable]

type mihomoTrafficSource struct{}

func (mihomoTrafficSource) Snapshot() []trafficmonitor.Connection {
	manager := statistic.DefaultManager
	if manager == nil {
		return nil
	}
	snapshot := manager.Snapshot()
	connections := make([]trafficmonitor.Connection, 0, len(snapshot.Connections))
	for _, tracker := range snapshot.Connections {
		if tracker == nil || tracker.Metadata == nil {
			continue
		}
		metadata := tracker.Metadata
		domain := metadata.Host
		if domain == "" {
			domain = metadata.SniffHost
		}
		domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
		destinationIP := ""
		if metadata.DstIP.IsValid() {
			destinationIP = metadata.DstIP.String()
		}
		geoIPLabels := normalizeGeoIPLabels(metadata.DstGeoIP)
		chain := append([]string(nil), tracker.Chain...)
		node := ""
		if len(chain) > 0 {
			node = chain[0]
		}
		route := classifyRoute(node)
		connections = append(connections, trafficmonitor.Connection{
			ID:            tracker.UUID.String(),
			Domain:        domain,
			DestinationIP: destinationIP,
			Country:       geoIPLabels,
			ASN:           metadata.DstIPASN,
			Node:          node,
			NodeRegion:    trafficmonitor.NodeRegionForRoute(node, route),
			ProxyChain:    displayProxyChain(chain),
			Rule:          tracker.Rule,
			RulePayload:   tracker.RulePayload,
			Network:       metadata.NetWork.String(),
			Process:       metadata.Process,
			Route:         route,
			UploadTotal:   tracker.UploadTotal.Load(),
			DownloadTotal: tracker.DownloadTotal.Load(),
		})
	}
	return connections
}

func startTrafficMonitor() error {
	if trafficMonitor.Load() != nil {
		return nil
	}
	appDataDir, err := appConfig.GetAppDataDir()
	if err != nil {
		return err
	}
	monitor, err := trafficmonitor.New(trafficmonitor.Options{
		DatabasePath:   filepath.Join(appDataDir, "traffic.sqlite"),
		ListenAddress:  "127.0.0.1:0",
		SampleInterval: time.Second,
		Retention:      30 * 24 * time.Hour,
		Logger:         MLog,
	}, mihomoTrafficSource{})
	if err != nil {
		return err
	}
	if err := monitor.Start(context.Background()); err != nil {
		_ = monitor.Close()
		return err
	}
	if !trafficMonitor.CompareAndSwap(nil, monitor) {
		_ = monitor.Close()
	}
	return nil
}

func stopTrafficMonitor() error {
	monitor := trafficMonitor.Swap(nil)
	if monitor == nil {
		return nil
	}
	return monitor.Close()
}

func displayProxyChain(chain []string) string {
	if len(chain) == 0 {
		return ""
	}
	display := append([]string(nil), chain...)
	for left, right := 0, len(display)-1; left < right; left, right = left+1, right-1 {
		display[left], display[right] = display[right], display[left]
	}
	return strings.Join(display, " → ")
}

func classifyRoute(node string) trafficmonitor.Route {
	switch strings.ToUpper(node) {
	case "", "DIRECT", "PASS", "COMPATIBLE":
		return trafficmonitor.RouteDirect
	case "REJECT", "REJECT-DROP":
		return trafficmonitor.RouteReject
	}
	if routes := trafficProxyRoutes.Load(); routes != nil {
		if route, exists := (*routes)[node]; exists {
			return route
		}
	}
	return trafficmonitor.RouteProxy
}

func setTrafficProxyRoutes(proxies map[string]C.Proxy) {
	routes := make(trafficProxyRouteTable)
	for name, proxy := range proxies {
		if proxy == nil {
			continue
		}
		if route, ok := routeForAdapterType(proxy.Type()); ok {
			routes[name] = route
		}
	}
	trafficProxyRoutes.Store(&routes)
}

func routeForAdapterType(adapterType C.AdapterType) (trafficmonitor.Route, bool) {
	switch adapterType {
	case C.Direct, C.Compatible, C.Pass, C.PassRule:
		return trafficmonitor.RouteDirect, true
	case C.Reject, C.RejectDrop:
		return trafficmonitor.RouteReject, true
	default:
		return "", false
	}
}

// DstGeoIP is populated only when Mihomo has queried GeoIP during rule
// matching. Normalize the observed labels so case and ordering do not split
// the same label set into separate report groups.
func normalizeGeoIPLabels(labels []string) string {
	unique := make(map[string]struct{}, len(labels))
	for _, label := range labels {
		label = strings.ToUpper(strings.TrimSpace(label))
		if label != "" {
			unique[label] = struct{}{}
		}
	}
	if len(unique) == 0 {
		return ""
	}
	normalized := make([]string, 0, len(unique))
	for label := range unique {
		normalized = append(normalized, label)
	}
	sort.Strings(normalized)
	return strings.Join(normalized, "/")
}
