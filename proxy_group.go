package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/metacubex/mihomo/common/utils"
	"github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/tunnel"
)

type ProxyGroup interface {
	MarshalJSON() ([]byte, error)
	URLTest(ctx context.Context, url string, expectedStatus utils.IntRanges[uint16]) (mp map[string]uint16, err error)
}

type ProxyGroupInfo struct {
	Name         string   `json:"name"`
	All          []string `json:"all"`
	Hidden       bool     `json:"hidden"`
	Now          string   `json:"now,omitempty"`
	URLTest      func(ctx context.Context, url string, expectedStatus utils.IntRanges[uint16]) (mp map[string]uint16, err error)
	ProxyAdapter constant.ProxyAdapter
}

func getProxyGroup() []*ProxyGroupInfo {
	proxies := tunnel.Proxies()
	if len(proxies) == 0 {
		return nil
	}
	globalProxies, _ := proxies["GLOBAL"].Adapter().(ProxyGroup)
	var globalInfo ProxyGroupInfo
	b, _ := globalProxies.MarshalJSON()
	_ = json.Unmarshal(b, &globalInfo)
	var gs []*ProxyGroupInfo
	for _, name := range globalInfo.All {
		if p, ok := proxies[name]; ok {
			g, ok := p.Adapter().(ProxyGroup)
			if !ok {
				continue
			}
			var info ProxyGroupInfo
			b, _ := g.MarshalJSON()
			_ = json.Unmarshal(b, &info)
			if info.Hidden {
				continue
			}
			info.Name = p.Name()
			info.URLTest = g.URLTest
			info.ProxyAdapter = p.Adapter()
			gs = append(gs, &info)
		}
	}
	return gs
}

type AllProxyMap map[string]constant.Proxy

func getAllProxy() AllProxyMap {
	allProxies := make(AllProxyMap)
	for name, proxy := range tunnel.Proxies() {
		allProxies[name] = proxy
	}
	for _, p := range tunnel.Providers() {
		for _, proxy := range p.Proxies() {
			allProxies[proxy.Name()] = proxy
		}
	}
	return allProxies
}

func (m AllProxyMap) Get(name string) constant.Proxy {
	return m[name]
}

func (m AllProxyMap) Delay(groupName string) string {
	groupProxy := m[groupName]
	groupDelayHistory := groupProxy.DelayHistory()
	var res string
	if len(groupDelayHistory) > 0 {
		delay := groupDelayHistory[len(groupDelayHistory)-1].Delay
		if delay <= 0 {
			res = "  (timeout)"
		} else {
			res = fmt.Sprintf("  (%dms)", delay)
		}
	}
	return res
}

func (m AllProxyMap) FinalProxy(startProxyName string) string {
	type FinalProxyFun func(startProxyName string, visited map[string]bool) string
	var findFinalProxy FinalProxyFun
	findFinalProxy = func(startProxyName string, visited map[string]bool) string {
		// 防止循环引用
		if visited[startProxyName] {
			return ""
		}
		visited[startProxyName] = true

		proxy, exists := m[startProxyName]
		if !exists {
			return ""
		}

		// 尝试将代理转换为ProxyGroup
		group, ok := proxy.Adapter().(ProxyGroup)
		if !ok {
			// 不是代理组,说明已经是最终节点
			return startProxyName
		}

		// 获取代理组信息
		var info ProxyGroupInfo
		b, err := group.MarshalJSON()
		if err != nil {
			return startProxyName
		}

		if err := json.Unmarshal(b, &info); err != nil {
			return startProxyName
		}

		// 如果now为空或者不存在,返回当前节点
		if info.Now == "" {
			return startProxyName
		}

		// 递归查找now指向的节点
		return findFinalProxy(info.Now, visited)
	}

	return findFinalProxy(startProxyName, make(map[string]bool))
}

func (m AllProxyMap) executeScript() (map[string][]map[string]interface{}, map[string]string) {
	groupAll := make(map[string][]map[string]interface{})
	renameMap := make(map[string]string)
	for _, group := range getProxyGroup() {
		newAll, err := executeProxiesScript(group.All)
		if err != nil {
			MLog.Warn("清洗节点名失败, 使用原始节点名", "error", err)
		}
		groupAll[group.Name] = newAll
		for _, proxy := range newAll {
			renameMap[proxy["_originalName"].(string)] = proxy["name"].(string)
		}
	}
	return groupAll, renameMap
}

func executeProxiesScript(proxyNames []string) ([]map[string]interface{}, error) {
	// 构建结果数组，初始为原始名称
	var allProxies []map[string]interface{}
	var needProxies []map[string]interface{}
	var notNeedProxies []map[string]interface{}
	for _, name := range proxyNames {
		if len(name) <= 0 {
			continue
		}
		curr := map[string]interface{}{
			"name":          name,
			"_originalName": name,
		}
		if name[0] == '[' {
			needProxies = append(needProxies, curr)
		} else {
			notNeedProxies = append(notNeedProxies, curr)
		}
	}

	allProxies = append(allProxies, notNeedProxies...)
	allProxies = append(allProxies, needProxies...)

	// 如果没有需要清洗的节点，直接返回原始数组
	if len(needProxies) == 0 {
		return allProxies, nil
	}

	// 如果没有配置脚本,尝试使用旧的rename.js方式(向后兼容)
	if proxyProcessService.GetScriptCount() == 0 {
		return allProxies, nil
	}

	// 转换为interface{}数组
	proxyInterfaces := make([]interface{}, len(needProxies))
	for i, p := range needProxies {
		proxyInterfaces[i] = p
	}

	// 使用新的处理服务
	processedProxies, err := proxyProcessService.ProcessProxies(proxyInterfaces)
	if err != nil {
		MLog.Warn("处理代理节点失败, 使用原始节点名", "error", err)
		return allProxies, nil
	}

	// 合并处理后的节点和不需要处理的节点
	var newAllProxies []map[string]interface{}
	newAllProxies = append(newAllProxies, notNeedProxies...)

	for _, proxy := range processedProxies {
		if proxyMap, ok := proxy.(map[string]interface{}); ok {
			newAllProxies = append(newAllProxies, proxyMap)
		}
	}
	return newAllProxies, nil
}
