package main

import (
	"context"
	"maps"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/metacubex/bbolt"
	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	"github.com/metacubex/mihomo/adapter/outboundgroup"
	"github.com/metacubex/mihomo/adapter/provider"
	"github.com/metacubex/mihomo/component/profile"
	"github.com/metacubex/mihomo/component/profile/cachefile"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
)

func newSelectionTestCache(t *testing.T) *cachefile.CacheFile {
	t.Helper()
	previous := profile.StoreSelected.Load()
	profile.StoreSelected.Store(true)
	t.Cleanup(func() { profile.StoreSelected.Store(previous) })
	db, err := bbolt.Open(filepath.Join(t.TempDir(), "cache.db"), 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	cache := &cachefile.CacheFile{DB: db}
	t.Cleanup(func() {
		if err := cache.Close(); err != nil {
			t.Error(err)
		}
	})
	return cache
}

func newSelectionTestGroup(t *testing.T, kind, name, url string, children ...C.Proxy) C.Proxy {
	t.Helper()
	hc := provider.NewHealthCheck(children, url, 1000, 0, false, nil)
	pd, err := provider.NewCompatibleProvider(name, children, hc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pd.Close() })
	option := outboundgroup.GroupCommonOption{Name: name, URL: url}
	providers := []P.ProxyProvider{pd}
	var group C.ProxyAdapter
	switch kind {
	case "select":
		group, err = outboundgroup.NewSelector(option, outboundgroup.SelectorOption{}, children[0], providers)
	case "fallback":
		group, err = outboundgroup.NewFallback(option, outboundgroup.FallbackOption{}, children[0], providers)
	case "url-test":
		group, err = outboundgroup.NewURLTest(option, outboundgroup.URLTestOption{}, children[0], providers)
	default:
		t.Fatalf("unsupported test group type: %s", kind)
	}
	if err != nil {
		t.Fatal(err)
	}
	return adapter.NewProxy(group)
}

func TestSetProxySelectionCachesParentWithoutChangingChild(t *testing.T) {
	cache := newSelectionTestCache(t)
	leaf := adapter.NewProxy(outbound.NewDirect())
	child := newSelectionTestGroup(t, "fallback", "自动选择", "", leaf)
	parent := newSelectionTestGroup(t, "select", "节点选择", "", leaf, child)
	cache.SetSelected(child.Name(), leaf.Name())
	cache.SetSelected(parent.Name(), leaf.Name())

	if err := setProxySelection(parent.Adapter(), child.Name(), cache); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{parent.Name(): child.Name(), child.Name(): leaf.Name()}
	if got := cache.SelectedMap(); !maps.Equal(got, want) {
		t.Fatalf("selection cache = %v, want %v", got, want)
	}
	if got := parent.Adapter().(*outboundgroup.Selector).Now(); got != child.Name() {
		t.Fatalf("selected proxy = %q, want %q", got, child.Name())
	}
	if err := setProxySelection(parent.Adapter(), "不存在的节点", cache); err == nil {
		t.Fatal("invalid selection unexpectedly succeeded")
	}
	if got := cache.SelectedMap(); !maps.Equal(got, want) {
		t.Fatalf("invalid selection changed cache: %v", got)
	}
}

func TestRepairProxySelectionCacheRestoresNestedFallback(t *testing.T) {
	cache := newSelectionTestCache(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	dead1 := adapter.NewProxy(outbound.NewRejectWithOption(outbound.RejectOption{Name: "自定义1"}))
	dead2 := adapter.NewProxy(outbound.NewRejectWithOption(outbound.RejectOption{Name: "自定义2"}))
	healthy := adapter.NewProxy(outbound.NewDirect())
	custom := newSelectionTestGroup(t, "url-test", "自定义", server.URL, dead1, dead2)
	global := newSelectionTestGroup(t, "url-test", "全球", server.URL, healthy)
	auto := newSelectionTestGroup(t, "fallback", "自动选择", server.URL, custom, global)
	parent := newSelectionTestGroup(t, "select", "节点选择", server.URL, auto)
	proxies := map[string]C.Proxy{custom.Name(): custom, global.Name(): global, auto.Name(): auto, parent.Name(): parent}
	for name, selected := range map[string]string{
		custom.Name(): custom.Name(), auto.Name(): auto.Name(), global.Name(): healthy.Name(), parent.Name(): auto.Name(),
	} {
		cache.SetSelected(name, selected)
	}
	probe := func(proxy C.Proxy, wantAlive bool) {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		_, err := proxy.URLTest(ctx, server.URL, nil)
		if got := err == nil && proxy.AliveForTestUrl(server.URL); got != wantAlive {
			t.Fatalf("%s probe alive = %v, want %v; error: %v", proxy.Name(), got, wantAlive, err)
		}
	}
	for _, proxy := range []C.Proxy{dead1, dead2, custom} {
		probe(proxy, false)
	}
	probe(global, true)
	fallback := auto.Adapter().(*outboundgroup.Fallback)
	fallback.ForceSet(cache.SelectedMap()[auto.Name()])
	probe(parent, false)

	repairProxySelectionCache(proxies, cache)
	want := map[string]string{custom.Name(): "", auto.Name(): "", global.Name(): healthy.Name(), parent.Name(): auto.Name()}
	if got := cache.SelectedMap(); !maps.Equal(got, want) {
		t.Fatalf("repaired cache = %v, want %v", got, want)
	}
	// Replay persisted selections as Mihomo does when applying a configuration.
	for name, selected := range cache.SelectedMap() {
		proxies[name].Adapter().(outboundgroup.SelectAble).ForceSet(selected)
	}
	if got := fallback.Now(); got != global.Name() {
		t.Fatalf("fallback selected %q, want healthy group %q", got, global.Name())
	}
	probe(parent, true)
	repairProxySelectionCache(proxies, cache)
	if got := cache.SelectedMap(); !maps.Equal(got, want) {
		t.Fatalf("second repair changed cache: %v", got)
	}
}

func TestRepairProxySelectionCachePreservesOtherSelections(t *testing.T) {
	cache := newSelectionTestCache(t)
	leaf := adapter.NewProxy(outbound.NewDirect())
	proxies := map[string]C.Proxy{leaf.Name(): leaf}
	want := map[string]string{leaf.Name(): leaf.Name(), "尚未加载的组": "尚未加载的组"}
	for _, test := range []struct{ kind, name, selected string }{
		{"fallback", "固定备用组", leaf.Name()},
		{"url-test", "固定测速组", leaf.Name()},
		{"fallback", "备用组订阅待加载", "暂时消失的节点"},
		{"url-test", "测速组订阅待加载", "暂时消失的节点"},
		{"select", "手选组", "手选组"},
	} {
		proxies[test.name] = newSelectionTestGroup(t, test.kind, test.name, "", leaf)
		want[test.name] = test.selected
	}
	for name, selected := range want {
		cache.SetSelected(name, selected)
	}
	repairProxySelectionCache(proxies, cache)
	if got := cache.SelectedMap(); !maps.Equal(got, want) {
		t.Fatalf("repair changed unrelated selections: %v, want %v", got, want)
	}
}

func TestProxySelectionCacheDisabled(t *testing.T) {
	cache := newSelectionTestCache(t)
	leaf := adapter.NewProxy(outbound.NewDirect())
	child := newSelectionTestGroup(t, "fallback", "自动选择", "", leaf)
	parent := newSelectionTestGroup(t, "select", "节点选择", "", leaf, child)
	want := map[string]string{child.Name(): child.Name(), parent.Name(): leaf.Name()}
	for name, selected := range want {
		cache.SetSelected(name, selected)
	}
	profile.StoreSelected.Store(false)
	repairProxySelectionCache(map[string]C.Proxy{child.Name(): child}, cache)
	if err := setProxySelection(parent.Adapter(), child.Name(), cache); err != nil {
		t.Fatal(err)
	}
	profile.StoreSelected.Store(true)
	if got := cache.SelectedMap(); !maps.Equal(got, want) {
		t.Fatalf("disabled persistence changed cache: %v, want %v", got, want)
	}
}
