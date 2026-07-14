package trafficmonitor

import "testing"

func TestClassifyNodeRegion(t *testing.T) {
	tests := []struct {
		name string
		node string
		want string
	}{
		{name: "direct", node: "DIRECT", want: "直连"},
		{name: "reject", node: "REJECT-DROP", want: "拒绝"},
		{name: "hong kong chinese", node: "香港 IEPL 01", want: "香港"},
		{name: "hong kong code", node: "premium-HK-01", want: "香港"},
		{name: "taiwan flag", node: "🇹🇼 TW-01", want: "台湾"},
		{name: "taiwan traditional", node: "台灣 01", want: "台湾"},
		{name: "japan city", node: "Tokyo Premium", want: "日本"},
		{name: "japan near miss", node: "JPG-01", want: "其他"},
		{name: "singapore code", node: "SG01", want: "新加坡"},
		{name: "united states", node: "United States - Seattle", want: "美国"},
		{name: "unrelated substring", node: "Status Premium", want: "其他"},
		{name: "english marker boundary", node: "Indianapolis", want: "其他"},
		{name: "ambiguous airport code", node: "FRA-01", want: "其他"},
		{name: "ambiguous codes", node: "US-CA-01", want: "其他"},
		{name: "ambiguous relay", node: "HK-US Relay", want: "其他"},
		{name: "marker and foreign code", node: "香港-US Relay", want: "其他"},
		{name: "marker and ambiguous foreign code", node: "香港-CA Relay", want: "其他"},
		{name: "explicit marker beats code", node: "United States CA-01", want: "美国"},
		{name: "unknown", node: "专线 A", want: "其他"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ClassifyNodeRegion(test.node); got != test.want {
				t.Fatalf("ClassifyNodeRegion(%q) = %q, want %q", test.node, got, test.want)
			}
		})
	}
}

func TestNodeRegionForRoute(t *testing.T) {
	if got := NodeRegionForRoute("自定义直连", RouteDirect); got != "直连" {
		t.Fatalf("direct region = %q, want 直连", got)
	}
	if got := NodeRegionForRoute("自定义拒绝", RouteReject); got != "拒绝" {
		t.Fatalf("reject region = %q, want 拒绝", got)
	}
	if got := NodeRegionForRoute("香港节点", RouteProxy); got != "香港" {
		t.Fatalf("proxy region = %q, want 香港", got)
	}
}
