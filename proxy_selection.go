package main

import (
	"fmt"

	"github.com/metacubex/mihomo/adapter/outboundgroup"
	"github.com/metacubex/mihomo/component/profile/cachefile"
	"github.com/metacubex/mihomo/constant"
)

func setProxySelection(group constant.ProxyAdapter, selected string, cache *cachefile.CacheFile) error {
	selector, ok := group.(outboundgroup.SelectAble)
	if !ok {
		return fmt.Errorf("代理组 %s 不支持选择节点", group.Name())
	}
	if err := selector.Set(selected); err != nil {
		return err
	}
	cache.SetSelected(group.Name(), selected)
	return nil
}

func repairProxySelectionCache(proxies map[string]constant.Proxy, cache *cachefile.CacheFile) {
	for name, selected := range cache.SelectedMap() {
		if selected != name {
			continue
		}
		proxy, ok := proxies[name]
		if !ok || (proxy.Type() != constant.Fallback && proxy.Type() != constant.URLTest) {
			continue
		}
		// 旧版托盘把子组名称同时写作缓存键和值；fallback 会因此一直选第一项。
		// 仅清除自动组的自身固定，不按当前成员列表验证尚未加载的订阅节点。
		cache.SetSelected(name, "")
	}
}
