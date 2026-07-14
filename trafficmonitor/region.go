package trafficmonitor

import "strings"

type nodeRegionRule struct {
	region  string
	markers []string
	codes   []string
}

var nodeRegionRules = []nodeRegionRule{
	{region: "香港", markers: []string{"🇭🇰", "香港", "HONG KONG", "HONGKONG"}, codes: []string{"HK", "HKG"}},
	{region: "台湾", markers: []string{"🇹🇼", "台湾", "台灣", "臺灣", "TAIWAN", "TAIPEI"}, codes: []string{"TW", "TPE"}},
	{region: "日本", markers: []string{"🇯🇵", "日本", "JAPAN", "TOKYO", "OSAKA"}, codes: []string{"JP", "JPN", "NRT", "KIX"}},
	{region: "新加坡", markers: []string{"🇸🇬", "新加坡", "狮城", "獅城", "SINGAPORE"}, codes: []string{"SG", "SGP"}},
	{region: "韩国", markers: []string{"🇰🇷", "韩国", "韓國", "KOREA", "SEOUL"}, codes: []string{"KR", "KOR", "ICN"}},
	{region: "美国", markers: []string{"🇺🇸", "美国", "美國", "UNITED STATES", "LOS ANGELES", "SAN JOSE", "SEATTLE", "NEW YORK", "SILICON VALLEY"}, codes: []string{"US", "USA", "LAX", "SJC", "SEA", "NYC"}},
	{region: "加拿大", markers: []string{"🇨🇦", "加拿大", "CANADA", "TORONTO", "VANCOUVER"}, codes: []string{"CA", "CAN", "YYZ", "YVR"}},
	{region: "英国", markers: []string{"🇬🇧", "英国", "英國", "UNITED KINGDOM", "LONDON"}, codes: []string{"UK", "GB", "GBR", "LHR"}},
	{region: "德国", markers: []string{"🇩🇪", "德国", "德國", "GERMANY", "FRANKFURT"}, codes: []string{"DE", "DEU", "FRA"}},
	{region: "法国", markers: []string{"🇫🇷", "法国", "法國", "FRANCE", "PARIS"}, codes: []string{"FR", "CDG"}},
	{region: "荷兰", markers: []string{"🇳🇱", "荷兰", "荷蘭", "NETHERLANDS", "AMSTERDAM"}, codes: []string{"NL", "NLD", "AMS"}},
	{region: "澳大利亚", markers: []string{"🇦🇺", "澳大利亚", "澳大利亞", "澳洲", "AUSTRALIA", "SYDNEY", "MELBOURNE"}, codes: []string{"AU", "AUS", "SYD", "MEL"}},
	{region: "印度", markers: []string{"🇮🇳", "印度", "INDIA", "MUMBAI"}, codes: []string{"IN", "IND", "BOM"}},
	{region: "俄罗斯", markers: []string{"🇷🇺", "俄罗斯", "俄羅斯", "RUSSIA", "MOSCOW"}, codes: []string{"RU", "RUS", "MOW"}},
	{region: "中国大陆", markers: []string{"🇨🇳", "中国大陆", "中國大陸", "MAINLAND CHINA", "BEIJING", "SHANGHAI"}, codes: []string{"CN", "CHN", "PEK", "PVG"}},
}

// ClassifyNodeRegion conservatively derives a display region from a proxy node
// name. Short country and airport codes are recognized only at ASCII token
// boundaries so unrelated words are not accidentally classified.
func ClassifyNodeRegion(node string) string {
	name := strings.TrimSpace(node)
	upper := strings.ToUpper(name)
	switch upper {
	case "", "DIRECT", "PASS", "COMPATIBLE":
		return "直连"
	case "REJECT", "REJECT-DROP":
		return "拒绝"
	}

	markerMatches := make(map[string]struct{})
	for _, rule := range nodeRegionRules {
		for _, marker := range rule.markers {
			if containsNodeRegionMarker(upper, marker) {
				markerMatches[rule.region] = struct{}{}
				break
			}
		}
	}
	if len(markerMatches) > 1 {
		return "其他"
	}
	if region := singleRegion(markerMatches); region != "" {
		for _, rule := range nodeRegionRules {
			if rule.region == region {
				continue
			}
			for _, code := range rule.codes {
				if !containsASCIIToken(upper, code) {
					continue
				}
				// CA is also a common US state abbreviation. FRA can mean
				// either France or Frankfurt, so only accept it alongside an
				// explicit France/Germany marker.
				if (code == "CA" && region == "美国") || (code == "FRA" && (region == "法国" || region == "德国")) {
					continue
				}
				return "其他"
			}
		}
		return region
	}
	// FRA is both France's ISO alpha-3 code and Frankfurt's airport code.
	// Require an explicit country/city marker instead of guessing.
	if containsASCIIToken(upper, "FRA") {
		return "其他"
	}

	codeMatches := make(map[string]struct{})
	for _, rule := range nodeRegionRules {
		for _, code := range rule.codes {
			if containsASCIIToken(upper, code) {
				codeMatches[rule.region] = struct{}{}
				break
			}
		}
	}
	if region := singleRegion(codeMatches); region != "" {
		return region
	}
	return "其他"
}

// NodeRegionForRoute keeps DIRECT/REJECT traffic out of proxy-name
// classification. Only proxy traffic is inferred from the node name.
func NodeRegionForRoute(node string, route Route) string {
	switch route {
	case RouteDirect:
		return "直连"
	case RouteReject:
		return "拒绝"
	default:
		return ClassifyNodeRegion(node)
	}
}

func containsNodeRegionMarker(value, marker string) bool {
	if marker != "" && isASCIIAlpha(marker[0]) {
		return containsASCIIToken(value, marker)
	}
	return strings.Contains(value, marker)
}

func singleRegion(matches map[string]struct{}) string {
	if len(matches) != 1 {
		return ""
	}
	for region := range matches {
		return region
	}
	return ""
}

func containsASCIIToken(value, token string) bool {
	for start := 0; start <= len(value)-len(token); {
		index := strings.Index(value[start:], token)
		if index < 0 {
			return false
		}
		index += start
		beforeOK := index == 0 || !isASCIIAlpha(value[index-1])
		after := index + len(token)
		afterOK := after == len(value) || !isASCIIAlpha(value[after])
		if beforeOK && afterOK {
			return true
		}
		start = index + 1
	}
	return false
}

func isASCIIAlpha(value byte) bool {
	return value >= 'A' && value <= 'Z'
}
