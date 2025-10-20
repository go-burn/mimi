package main

import (
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// ProxyStatusChecker 代理状态检测器
type ProxyStatusChecker struct {
	cachedRunning bool
	lastCheckTime time.Time
	mutex         sync.RWMutex
}

// NewProxyStatusChecker 创建代理状态检测器
func NewProxyStatusChecker() *ProxyStatusChecker {
	return &ProxyStatusChecker{}
}

// testConnectivity 测试代理连通性(访问Google)
func (p *ProxyStatusChecker) testConnectivity() bool {
	// 检查是否已初始化配置
	if mcfg == nil || mcfg.General == nil {
		return false
	}

	var client *http.Client

	// 判断是否启用了TUN模式
	if mcfg.General.Tun.Enable {
		// TUN模式下,直接访问(流量会被虚拟网卡接管)
		client = &http.Client{
			Timeout: 3 * time.Second,
		}
	} else {
		// 系统代理模式下,通过应用代理访问
		proxyURL := fmt.Sprintf("http://127.0.0.1:%d", mcfg.General.MixedPort)
		proxyURLParsed, err := url.Parse(proxyURL)
		if err != nil {
			MLog.Error("解析代理URL失败", "error", err)
			return false
		}

		client = &http.Client{
			Timeout: 3 * time.Second,
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURLParsed),
			},
		}
	}

	resp, err := client.Get("https://www.google.com/generate_204")
	if err != nil {
		MLog.Debug("代理连通性测试失败", "error", err)
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == 204 || resp.StatusCode == 200
}

// Check 检查代理运行状态
func (p *ProxyStatusChecker) Check() bool {
	// 检查是否启用了系统代理或TUN模式
	hasProxy := systemProxyService != nil && systemProxyService.StateProxy()
	hasTun := mcfg != nil && mcfg.General != nil && mcfg.General.Tun.Enable

	// 如果都没有启用,返回false
	if !hasProxy && !hasTun {
		return false
	}

	// 测试Google连通性
	return p.testConnectivity()
}

// Update 更新缓存的代理状态
func (p *ProxyStatusChecker) Update() {
	isRunning := p.Check()

	p.mutex.Lock()
	p.cachedRunning = isRunning
	p.lastCheckTime = time.Now()
	p.mutex.Unlock()
}

// GetCachedStatus 获取缓存的代理状态
func (p *ProxyStatusChecker) GetCachedStatus() bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.cachedRunning
}

// GetStatusText 获取状态显示文本
func (p *ProxyStatusChecker) GetStatusText() (icon string, text string) {
	// 检查服务是否已初始化
	if systemProxyService == nil {
		return "⚪", "未代理"
	}

	// 检查是否启用了系统代理或TUN模式
	hasProxy := systemProxyService.StateProxy()
	hasTun := mcfg != nil && mcfg.General != nil && mcfg.General.Tun.Enable

	// 如果都没有启用
	if !hasProxy && !hasTun {
		return "⚪", "未代理"
	}

	// 已启用代理或TUN,根据连通性显示状态
	if p.GetCachedStatus() {
		return "🟢", "运行中"
	}
	return "🔴", "失败"
}

// StartMonitor 启动代理状态后台监控
func (p *ProxyStatusChecker) StartMonitor() {
	MLog.Info("启动代理状态监控")

	// 立即检查一次状态
	p.Update()

	// 每30秒检查一次状态
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for range ticker.C {
			// 检查是否启用了系统代理或TUN模式
			hasProxy := systemProxyService != nil && systemProxyService.StateProxy()
			hasTun := mcfg != nil && mcfg.General != nil && mcfg.General.Tun.Enable

			// 只在启用了任一代理模式时检查
			if hasProxy || hasTun {
				p.Update()
			}
		}
	}()
}
