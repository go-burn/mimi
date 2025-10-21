package sysproxy

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// WindowsProxyService Windows系统代理服务
type WindowsProxyService struct{}

// NewProxyService 创建Windows代理服务实例
func NewProxyService() (*WindowsProxyService, error) {
	return &WindowsProxyService{}, nil
}

// SetProxy 设置系统代理
func (s *WindowsProxyService) SetProxy(config ProxyConfig) error {
	// 打开注册表项
	key, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("打开注册表失败: %w", err)
	}
	defer key.Close()

	if config.Enable {
		// 启用代理
		if err := key.SetDWordValue("ProxyEnable", 1); err != nil {
			return fmt.Errorf("设置ProxyEnable失败: %w", err)
		}

		// 设置代理服务器地址
		// Windows格式: http=host:port;https=host:port;ftp=host:port;socks=host:port
		proxyServer := fmt.Sprintf("http=%s;https=%s;socks=%s",
			config.Server, config.Server, config.Server)
		if err := key.SetStringValue("ProxyServer", proxyServer); err != nil {
			return fmt.Errorf("设置ProxyServer失败: %w", err)
		}

		// 设置绕过列表
		if config.Bypass != "" {
			// Windows使用分号分隔
			bypass := strings.ReplaceAll(config.Bypass, ",", ";")
			if err := key.SetStringValue("ProxyOverride", bypass); err != nil {
				return fmt.Errorf("设置ProxyOverride失败: %w", err)
			}
		} else {
			// 默认绕过本地地址
			if err := key.SetStringValue("ProxyOverride", "localhost;127.*;10.*;192.168.*;<local>"); err != nil {
				return fmt.Errorf("设置ProxyOverride失败: %w", err)
			}
		}
	} else {
		// 禁用代理
		if err := key.SetDWordValue("ProxyEnable", 0); err != nil {
			return fmt.Errorf("设置ProxyEnable失败: %w", err)
		}
	}

	// 通知系统代理设置已更改
	if err := s.notifyProxyChange(); err != nil {
		return fmt.Errorf("通知系统代理变更失败: %w", err)
	}

	return nil
}

// ClearProxy 清除系统代理
func (s *WindowsProxyService) ClearProxy() error {
	// 打开注册表项
	key, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("打开注册表失败: %w", err)
	}
	defer key.Close()

	// 禁用代理
	if err := key.SetDWordValue("ProxyEnable", 0); err != nil {
		return fmt.Errorf("设置ProxyEnable失败: %w", err)
	}

	// 清空代理服务器地址
	if err := key.SetStringValue("ProxyServer", ""); err != nil {
		return fmt.Errorf("清除ProxyServer失败: %w", err)
	}

	// 清空绕过列表
	if err := key.SetStringValue("ProxyOverride", ""); err != nil {
		return fmt.Errorf("清除ProxyOverride失败: %w", err)
	}

	// 通知系统代理设置已更改
	if err := s.notifyProxyChange(); err != nil {
		return fmt.Errorf("通知系统代理变更失败: %w", err)
	}

	return nil
}

// GetProxy 获取当前系统代理配置
func (s *WindowsProxyService) GetProxy() (*ProxyConfig, error) {
	// 打开注册表项
	key, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		registry.QUERY_VALUE)
	if err != nil {
		return nil, fmt.Errorf("打开注册表失败: %w", err)
	}
	defer key.Close()

	config := &ProxyConfig{}

	// 读取是否启用代理
	proxyEnable, _, err := key.GetIntegerValue("ProxyEnable")
	if err == nil {
		config.Enable = proxyEnable == 1
	}

	// 读取代理服务器地址
	proxyServer, _, err := key.GetStringValue("ProxyServer")
	if err == nil && proxyServer != "" {
		// Windows格式可能是: "host:port" 或 "http=host:port;https=host:port"
		// 尝试提取第一个代理地址
		parts := strings.Split(proxyServer, ";")
		if len(parts) > 0 {
			server := parts[0]
			// 如果包含协议前缀,去除它
			if idx := strings.Index(server, "="); idx != -1 {
				server = server[idx+1:]
			}
			config.Server = server
		}
	}

	// 读取绕过列表
	proxyOverride, _, err := key.GetStringValue("ProxyOverride")
	if err == nil && proxyOverride != "" {
		// 将分号转换为逗号
		config.Bypass = strings.ReplaceAll(proxyOverride, ";", ",")
	}

	return config, nil
}

// notifyProxyChange 通知系统代理设置已更改
func (s *WindowsProxyService) notifyProxyChange() error {
	// 加载wininet.dll
	wininet := windows.NewLazySystemDLL("wininet.dll")
	internetSetOption := wininet.NewProc("InternetSetOptionW")

	// INTERNET_OPTION_SETTINGS_CHANGED = 39
	// INTERNET_OPTION_REFRESH = 37
	const (
		INTERNET_OPTION_SETTINGS_CHANGED = 39
		INTERNET_OPTION_REFRESH          = 37
	)

	// 通知设置已更改
	ret, _, _ := internetSetOption.Call(0, INTERNET_OPTION_SETTINGS_CHANGED, 0, 0)
	if ret == 0 {
		return fmt.Errorf("InternetSetOption SETTINGS_CHANGED failed")
	}

	// 刷新设置
	ret, _, _ = internetSetOption.Call(0, INTERNET_OPTION_REFRESH, 0, 0)
	if ret == 0 {
		return fmt.Errorf("InternetSetOption REFRESH failed")
	}

	return nil
}
