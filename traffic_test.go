package main

import (
	"testing"

	"mimi/trafficmonitor"

	C "github.com/metacubex/mihomo/constant"
)

func TestDisplayProxyChain(t *testing.T) {
	if got := displayProxyChain([]string{"香港节点", "节点选择"}); got != "节点选择 → 香港节点" {
		t.Fatalf("displayProxyChain() = %q", got)
	}
}

func TestClassifyRoute(t *testing.T) {
	previous := trafficProxyRoutes.Load()
	defer trafficProxyRoutes.Store(previous)
	routes := trafficProxyRouteTable{
		"自定义直连": trafficmonitor.RouteDirect,
		"自定义拒绝": trafficmonitor.RouteReject,
	}
	trafficProxyRoutes.Store(&routes)

	tests := map[string]string{
		"DIRECT": "direct",
		"REJECT": "reject",
		"自定义直连":  "direct",
		"自定义拒绝":  "reject",
		"香港节点":   "proxy",
	}
	for node, want := range tests {
		if got := string(classifyRoute(node)); got != want {
			t.Fatalf("classifyRoute(%q) = %q, want %q", node, got, want)
		}
	}
}

func TestRouteForAdapterType(t *testing.T) {
	tests := []struct {
		adapterType C.AdapterType
		want        trafficmonitor.Route
		ok          bool
	}{
		{adapterType: C.Direct, want: trafficmonitor.RouteDirect, ok: true},
		{adapterType: C.RejectDrop, want: trafficmonitor.RouteReject, ok: true},
		{adapterType: C.Vmess, want: "", ok: false},
	}
	for _, test := range tests {
		got, ok := routeForAdapterType(test.adapterType)
		if got != test.want || ok != test.ok {
			t.Fatalf("routeForAdapterType(%v) = (%q, %v), want (%q, %v)", test.adapterType, got, ok, test.want, test.ok)
		}
	}
}

func TestNormalizeGeoIPLabels(t *testing.T) {
	if got := normalizeGeoIPLabels([]string{"cn", "HK", "cn", ""}); got != "CN/HK" {
		t.Fatalf("normalizeGeoIPLabels() = %q, want CN/HK", got)
	}
	if got := normalizeGeoIPLabels(nil); got != "" {
		t.Fatalf("normalizeGeoIPLabels(nil) = %q, want empty", got)
	}
}
