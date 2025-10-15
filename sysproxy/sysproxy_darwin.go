package sysproxy

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// MacOSProxyService macOS系统代理服务
type MacOSProxyService struct {
	networkServices []string          // 网络服务列表 (如 Wi-Fi, Ethernet)
	savedConfig     *ProxyConfig      // 保存的代理配置
	configMutex     sync.RWMutex      // 配置读写锁
	watcher         *fsnotify.Watcher // 文件系统监听器
	stopChan        chan struct{}     // 停止信号通道
}

// NewProxyService 创建macOS代理服务实例
func NewProxyService() (*MacOSProxyService, error) {
	service := &MacOSProxyService{
		stopChan: make(chan struct{}),
	}
	if err := service.detectNetworkServices(); err != nil {
		return nil, fmt.Errorf("检测网络服务失败: %w", err)
	}

	// 启动网络监听
	if err := service.startNetworkMonitor(); err != nil {
		logger.Warn("启动网络监听失败", "error", err)
		// 不返回错误,继续使用基本功能
	}

	return service, nil
}

// detectNetworkServices 检测系统中的网络服务
func (s *MacOSProxyService) detectNetworkServices() error {
	cmd := exec.Command("networksetup", "-listallnetworkservices")
	output, err := cmd.Output()
	if err != nil {
		return err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	s.networkServices = []string{}

	// 跳过第一行提示信息
	for i, line := range lines {
		if i == 0 {
			continue
		}
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "*") {
			s.networkServices = append(s.networkServices, line)
		}
	}

	if len(s.networkServices) == 0 {
		return fmt.Errorf("未找到任何网络服务")
	}

	return nil
}

// SetProxy 设置系统代理
func (s *MacOSProxyService) SetProxy(config ProxyConfig) error {
	if len(s.networkServices) == 0 {
		return fmt.Errorf("没有可用的网络服务")
	}

	// 解析服务器地址和端口
	parts := strings.Split(config.Server, ":")
	if len(parts) != 2 {
		return fmt.Errorf("无效的代理服务器地址格式，应为 host:port")
	}
	host := parts[0]
	port := parts[1]

	var errors []string

	// 为每个网络服务设置代理
	for _, service := range s.networkServices {
		// 设置HTTP代理
		if err := s.setWebProxy(service, host, port, config.Enable); err != nil {
			errors = append(errors, fmt.Sprintf("%s HTTP代理设置失败: %v", service, err))
		}

		// 设置HTTPS代理
		if err := s.setSecureWebProxy(service, host, port, config.Enable); err != nil {
			errors = append(errors, fmt.Sprintf("%s HTTPS代理设置失败: %v", service, err))
		}

		// 设置SOCKS代理
		if err := s.setSocksFirewallProxy(service, host, port, config.Enable); err != nil {
			errors = append(errors, fmt.Sprintf("%s SOCKS代理设置失败: %v", service, err))
		}

		// 设置绕过列表
		if config.Bypass != "" && config.Enable {
			if err := s.setProxyBypass(service, config.Bypass); err != nil {
				errors = append(errors, fmt.Sprintf("%s 绕过列表设置失败: %v", service, err))
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("部分代理设置失败:\n%s", strings.Join(errors, "\n"))
	}

	// 保存配置,用于网络变化时自动恢复
	s.configMutex.Lock()
	s.savedConfig = &config
	s.configMutex.Unlock()

	return nil
}

// ClearProxy 清除系统代理
func (s *MacOSProxyService) ClearProxy() error {
	if len(s.networkServices) == 0 {
		return fmt.Errorf("没有可用的网络服务")
	}

	var errors []string

	for _, service := range s.networkServices {
		// 关闭HTTP代理
		if err := s.setWebProxyState(service, "off"); err != nil {
			errors = append(errors, fmt.Sprintf("%s HTTP代理关闭失败: %v", service, err))
		}

		// 关闭HTTPS代理
		if err := s.setSecureWebProxyState(service, "off"); err != nil {
			errors = append(errors, fmt.Sprintf("%s HTTPS代理关闭失败: %v", service, err))
		}

		// 关闭SOCKS代理
		if err := s.setSocksFirewallProxyState(service, "off"); err != nil {
			errors = append(errors, fmt.Sprintf("%s SOCKS代理关闭失败: %v", service, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("部分代理清除失败:\n%s", strings.Join(errors, "\n"))
	}

	// 清除保存的配置
	s.configMutex.Lock()
	s.savedConfig = nil
	s.configMutex.Unlock()

	return nil
}

// GetProxy 获取当前系统代理配置
func (s *MacOSProxyService) GetProxy() (*ProxyConfig, error) {
	if len(s.networkServices) == 0 {
		return nil, fmt.Errorf("没有可用的网络服务")
	}

	// 获取第一个网络服务的代理配置
	service := s.networkServices[0]

	config := &ProxyConfig{}

	// 检查HTTP代理状态
	cmd := exec.Command("networksetup", "-getwebproxy", service)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("获取HTTP代理配置失败: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	var server, port string
	enabled := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Enabled:") {
			enabled = strings.Contains(line, "Yes")
		} else if strings.HasPrefix(line, "Server:") {
			server = strings.TrimSpace(strings.TrimPrefix(line, "Server:"))
		} else if strings.HasPrefix(line, "Port:") {
			port = strings.TrimSpace(strings.TrimPrefix(line, "Port:"))
		}
	}

	config.Enable = enabled
	if server != "" && port != "" {
		config.Server = fmt.Sprintf("%s:%s", server, port)
	}

	// 获取绕过列表
	cmd = exec.Command("networksetup", "-getproxybypassdomains", service)
	output, err = cmd.Output()
	if err == nil {
		bypass := strings.TrimSpace(string(output))
		if bypass != "" && bypass != "There aren't any bypass domains set on" {
			config.Bypass = strings.ReplaceAll(bypass, "\n", ",")
		}
	}

	return config, nil
}

// setWebProxy 设置HTTP代理
func (s *MacOSProxyService) setWebProxy(service, host, port string, enable bool) error {
	state := "off"
	if enable {
		state = "on"
	}
	cmd := exec.Command("networksetup", "-setwebproxy", service, host, port)
	if err := cmd.Run(); err != nil {
		return err
	}

	return s.setWebProxyState(service, state)
}

// setSecureWebProxy 设置HTTPS代理
func (s *MacOSProxyService) setSecureWebProxy(service, host, port string, enable bool) error {
	state := "off"
	if enable {
		state = "on"
	}

	cmd := exec.Command("networksetup", "-setsecurewebproxy", service, host, port)
	if err := cmd.Run(); err != nil {
		return err
	}

	return s.setSecureWebProxyState(service, state)
}

// setSocksFirewallProxy 设置SOCKS代理
func (s *MacOSProxyService) setSocksFirewallProxy(service, host, port string, enable bool) error {
	state := "off"
	if enable {
		state = "on"
	}

	cmd := exec.Command("networksetup", "-setsocksfirewallproxy", service, host, port)
	if err := cmd.Run(); err != nil {
		return err
	}

	return s.setSocksFirewallProxyState(service, state)
}

// setWebProxyState 设置HTTP代理状态
func (s *MacOSProxyService) setWebProxyState(service, state string) error {
	cmd := exec.Command("networksetup", "-setwebproxystate", service, state)
	return cmd.Run()
}

// setSecureWebProxyState 设置HTTPS代理状态
func (s *MacOSProxyService) setSecureWebProxyState(service, state string) error {
	cmd := exec.Command("networksetup", "-setsecurewebproxystate", service, state)
	return cmd.Run()
}

// setSocksFirewallProxyState 设置SOCKS代理状态
func (s *MacOSProxyService) setSocksFirewallProxyState(service, state string) error {
	cmd := exec.Command("networksetup", "-setsocksfirewallproxystate", service, state)
	return cmd.Run()
}

// setProxyBypass 设置代理绕过列表
func (s *MacOSProxyService) setProxyBypass(service, bypass string) error {
	// 将逗号分隔的列表转换为空格分隔
	domains := strings.Split(bypass, ",")
	for i, domain := range domains {
		domains[i] = strings.TrimSpace(domain)
	}

	args := append([]string{"-setproxybypassdomains", service}, domains...)
	cmd := exec.Command("networksetup", args...)
	return cmd.Run()
}

// startNetworkMonitor 启动网络配置文件监听
func (s *MacOSProxyService) startNetworkMonitor() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("创建文件监听器失败: %w", err)
	}

	s.watcher = watcher

	// macOS 网络配置目录
	configPath := "/Library/Preferences/SystemConfiguration"
	if err := watcher.Add(configPath); err != nil {
		watcher.Close()
		return fmt.Errorf("添加监听路径失败: %w", err)
	}

	logger.Info("已启动网络配置监听", "path", configPath)

	// 在后台运行监听
	go s.watchNetworkChanges()

	return nil
}

// watchNetworkChanges 监听网络配置变化
func (s *MacOSProxyService) watchNetworkChanges() {
	defer s.watcher.Close()

	// 防抖动: 合并短时间内的多次变化
	var debounceTimer *time.Timer
	const debounceDelay = 2 * time.Second

	for {
		select {
		case event, ok := <-s.watcher.Events:
			if !ok {
				return
			}

			// 只关注网络配置相关文件的写入事件
			if event.Op&fsnotify.Write == fsnotify.Write {
				filename := filepath.Base(event.Name)
				// preferences.plist 包含网络配置
				if filename == "preferences.plist" || filename == "NetworkInterfaces.plist" {
					// 使用防抖动,避免频繁触发
					if debounceTimer != nil {
						debounceTimer.Stop()
					}
					debounceTimer = time.AfterFunc(debounceDelay, func() {
						s.onNetworkChange()
					})
				}
			}

		case err, ok := <-s.watcher.Errors:
			if !ok {
				return
			}
			logger.Error("网络监听错误", "error", err)

		case <-s.stopChan:
			logger.Info("停止网络监听")
			return
		}
	}
}

// onNetworkChange 网络配置变化时的回调处理
func (s *MacOSProxyService) onNetworkChange() {
	logger.Info("检测到网络配置变化")

	// 重新检测网络服务
	oldServices := s.networkServices
	if err := s.detectNetworkServices(); err != nil {
		logger.Error("重新检测网络服务失败", "error", err)
		return
	}

	// 比较网络服务列表是否发生变化
	if !s.isServicesChanged(oldServices, s.networkServices) {
		logger.Info("网络服务列表未变化,跳过代理恢复")
		return
	}

	logger.Info("网络服务列表已变化", "old", oldServices, "new", s.networkServices)

	// 如果有保存的代理配置且已启用,重新应用
	s.configMutex.RLock()
	savedConfig := s.savedConfig
	s.configMutex.RUnlock()

	if savedConfig != nil && savedConfig.Enable {
		logger.Info("重新应用代理配置", "server", savedConfig.Server)
		if err := s.SetProxy(*savedConfig); err != nil {
			logger.Error("重新应用代理设置失败", "error", err)
		} else {
			logger.Info("代理设置已自动恢复")
		}
	}
}

// isServicesChanged 检查网络服务列表是否发生变化
func (s *MacOSProxyService) isServicesChanged(old, new []string) bool {
	if len(old) != len(new) {
		return true
	}

	// 创建映射用于快速查找
	oldMap := make(map[string]bool)
	for _, service := range old {
		oldMap[service] = true
	}

	// 检查是否有新服务
	for _, service := range new {
		if !oldMap[service] {
			return true
		}
	}

	return false
}

// Stop 停止网络监听(用于清理资源)
func (s *MacOSProxyService) Stop() {
	if s.stopChan != nil {
		close(s.stopChan)
	}
}
