package sysproxy

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// sysproxy logger
var logger *slog.Logger

func init() {
	// 创建一个简单的 slog logger,会在主程序初始化后被更新
	logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// SetLogger 设置 sysproxy 使用的 logger
func SetLogger(l *slog.Logger) {
	if l != nil {
		logger = l
	}
}

// ProxyConfig 代理配置结构
type ProxyConfig struct {
	Enable bool   `json:"enable"` // 是否启用代理
	Server string `json:"server"` // 代理服务器地址 (例如: "127.0.0.1:7890")
	Bypass string `json:"bypass"` // 绕过代理的地址列表 (例如: "localhost,127.*,10.*,192.168.*")
}

// SystemProxyService 系统代理服务接口
type SystemProxyService interface {
	// SetProxy 设置系统代理
	SetProxy(config ProxyConfig) error
	// ClearProxy 清除系统代理
	ClearProxy() error
	// GetProxy 获取当前系统代理配置
	GetProxy() (*ProxyConfig, error)
}

// SystemProxyService 系统代理服务的Wails绑定
type SystemProxy struct {
	service SystemProxyService
}

// NewSystemProxyService 创建系统代理服务实例
func NewSystemProxy() (*SystemProxy, error) {
	service, err := NewProxyService()
	if err != nil {
		return nil, fmt.Errorf("创建系统代理服务失败: %w", err)
	}

	return &SystemProxy{
		service: service,
	}, nil
}

// SetProxy 设置系统代理
func (s *SystemProxy) SetProxy(config ProxyConfig) error {
	return s.service.SetProxy(config)
}

// ClearProxy 清除系统代理
func (s *SystemProxy) ClearProxy() error {
	return s.service.ClearProxy()
}

// GetProxy 获取当前系统代理配置
func (s *SystemProxy) GetProxy() (*ProxyConfig, error) {
	return s.service.GetProxy()
}

// EnableProxy 启用系统代理 (快捷方法)
func (s *SystemProxy) EnableProxy(server string, bypass ...string) error {
	config := ProxyConfig{
		Enable: true,
		Server: server,
		Bypass: "127.0.0.1/8,192.168.0.0/16,10.0.0.0/8,172.16.0.0/12,localhost,*.local,*.crashlytics.com,<local>",
	}
	if len(bypass) > 0 {
		config.Bypass = strings.Join(bypass, ",")
	}
	return s.service.SetProxy(config)
}

// DisableProxy 禁用系统代理 (快捷方法)
func (s *SystemProxy) DisableProxy() error {
	return s.ClearProxy()
}

func (s *SystemProxy) StateProxy() bool {
	config, err := s.service.GetProxy()
	if err != nil {
		logger.Error("获取系统代理状态失败", "error", err)
		return false
	}
	return config.Enable
}
