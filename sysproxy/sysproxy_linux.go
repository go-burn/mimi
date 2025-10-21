package sysproxy

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// LinuxProxyService Linux系统代理服务
type LinuxProxyService struct {
	desktopEnvironment string // 桌面环境类型 (GNOME, KDE, 等)
}

// NewLinuxProxyService 创建Linux代理服务实例
func NewProxyService() (*LinuxProxyService, error) {
	service := &LinuxProxyService{}
	service.detectDesktopEnvironment()
	return service, nil
}

// detectDesktopEnvironment 检测桌面环境
func (s *LinuxProxyService) detectDesktopEnvironment() {
	// 检测GNOME
	if os.Getenv("GNOME_DESKTOP_SESSION_ID") != "" || os.Getenv("XDG_CURRENT_DESKTOP") == "GNOME" {
		s.desktopEnvironment = "GNOME"
		return
	}

	// 检测KDE
	if os.Getenv("KDE_FULL_SESSION") != "" || os.Getenv("XDG_CURRENT_DESKTOP") == "KDE" {
		s.desktopEnvironment = "KDE"
		return
	}

	// 默认使用环境变量方式
	s.desktopEnvironment = "ENV"
}

// SetProxy 设置系统代理
func (s *LinuxProxyService) SetProxy(config ProxyConfig) error {
	var errors []string

	// 1. 设置环境变量 (适用于所有环境)
	if err := s.setEnvironmentProxy(config); err != nil {
		errors = append(errors, fmt.Sprintf("设置环境变量失败: %v", err))
	}

	// 2. 根据桌面环境设置系统代理
	switch s.desktopEnvironment {
	case "GNOME":
		if err := s.setGNOMEProxy(config); err != nil {
			errors = append(errors, fmt.Sprintf("设置GNOME代理失败: %v", err))
		}
	case "KDE":
		if err := s.setKDEProxy(config); err != nil {
			errors = append(errors, fmt.Sprintf("设置KDE代理失败: %v", err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("部分代理设置失败:\n%s", strings.Join(errors, "\n"))
	}

	return nil
}

// ClearProxy 清除系统代理
func (s *LinuxProxyService) ClearProxy() error {
	var errors []string

	// 1. 清除环境变量
	if err := s.clearEnvironmentProxy(); err != nil {
		errors = append(errors, fmt.Sprintf("清除环境变量失败: %v", err))
	}

	// 2. 根据桌面环境清除系统代理
	switch s.desktopEnvironment {
	case "GNOME":
		if err := s.clearGNOMEProxy(); err != nil {
			errors = append(errors, fmt.Sprintf("清除GNOME代理失败: %v", err))
		}
	case "KDE":
		if err := s.clearKDEProxy(); err != nil {
			errors = append(errors, fmt.Sprintf("清除KDE代理失败: %v", err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("部分代理清除失败:\n%s", strings.Join(errors, "\n"))
	}

	return nil
}

// GetProxy 获取当前系统代理配置
func (s *LinuxProxyService) GetProxy() (*ProxyConfig, error) {
	config := &ProxyConfig{}

	// 优先从桌面环境获取
	switch s.desktopEnvironment {
	case "GNOME":
		return s.getGNOMEProxy()
	case "KDE":
		return s.getKDEProxy()
	}

	// 从环境变量获取
	httpProxy := os.Getenv("http_proxy")
	if httpProxy == "" {
		httpProxy = os.Getenv("HTTP_PROXY")
	}

	if httpProxy != "" {
		// 解析代理URL (http://host:port)
		httpProxy = strings.TrimPrefix(httpProxy, "http://")
		httpProxy = strings.TrimPrefix(httpProxy, "https://")
		config.Server = httpProxy
		config.Enable = true
	}

	noProxy := os.Getenv("no_proxy")
	if noProxy == "" {
		noProxy = os.Getenv("NO_PROXY")
	}
	config.Bypass = noProxy

	return config, nil
}

// setEnvironmentProxy 设置环境变量代理
func (s *LinuxProxyService) setEnvironmentProxy(config ProxyConfig) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// 写入到 ~/.profile 或 ~/.bashrc
	profilePath := filepath.Join(homeDir, ".profile")
	bashrcPath := filepath.Join(homeDir, ".bashrc")

	proxyLines := []string{
		"# Proxy settings managed by MIMI",
	}

	if config.Enable {
		proxyURL := fmt.Sprintf("http://%s", config.Server)
		proxyLines = append(proxyLines,
			fmt.Sprintf(`export http_proxy="%s"`, proxyURL),
			fmt.Sprintf(`export https_proxy="%s"`, proxyURL),
			fmt.Sprintf(`export HTTP_PROXY="%s"`, proxyURL),
			fmt.Sprintf(`export HTTPS_PROXY="%s"`, proxyURL),
		)

		if config.Bypass != "" {
			proxyLines = append(proxyLines,
				fmt.Sprintf(`export no_proxy="%s"`, config.Bypass),
				fmt.Sprintf(`export NO_PROXY="%s"`, config.Bypass),
			)
		}
	} else {
		proxyLines = append(proxyLines,
			"unset http_proxy",
			"unset https_proxy",
			"unset HTTP_PROXY",
			"unset HTTPS_PROXY",
			"unset no_proxy",
			"unset NO_PROXY",
		)
	}

	// 更新配置文件
	for _, path := range []string{profilePath, bashrcPath} {
		if err := s.updateProxyInFile(path, proxyLines); err != nil {
			// 忽略错误,继续处理下一个文件
			continue
		}
	}

	return nil
}

// clearEnvironmentProxy 清除环境变量代理
func (s *LinuxProxyService) clearEnvironmentProxy() error {
	return s.setEnvironmentProxy(ProxyConfig{Enable: false})
}

// updateProxyInFile 更新文件中的代理配置
func (s *LinuxProxyService) updateProxyInFile(filePath string, newLines []string) error {
	// 读取现有内容
	content, err := os.ReadFile(filePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	lines := strings.Split(string(content), "\n")
	var newContent []string
	inProxySection := false

	// 移除旧的代理配置
	for _, line := range lines {
		if strings.Contains(line, "# Proxy settings managed by MIMI") {
			inProxySection = true
			continue
		}
		if inProxySection {
			if strings.HasPrefix(line, "export") || strings.HasPrefix(line, "unset") {
				continue
			}
			inProxySection = false
		}
		newContent = append(newContent, line)
	}

	// 添加新的代理配置
	newContent = append(newContent, "")
	newContent = append(newContent, newLines...)
	newContent = append(newContent, "")

	// 写回文件
	return os.WriteFile(filePath, []byte(strings.Join(newContent, "\n")), 0644)
}

// setGNOMEProxy 设置GNOME桌面代理
func (s *LinuxProxyService) setGNOMEProxy(config ProxyConfig) error {
	parts := strings.Split(config.Server, ":")
	if len(parts) != 2 {
		return fmt.Errorf("无效的代理服务器地址格式")
	}
	host := parts[0]
	port := parts[1]

	if config.Enable {
		// 设置代理模式为手动
		if err := exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "manual").Run(); err != nil {
			return err
		}

		// 设置HTTP代理
		if err := exec.Command("gsettings", "set", "org.gnome.system.proxy.http", "host", host).Run(); err != nil {
			return err
		}
		if err := exec.Command("gsettings", "set", "org.gnome.system.proxy.http", "port", port).Run(); err != nil {
			return err
		}

		// 设置HTTPS代理
		if err := exec.Command("gsettings", "set", "org.gnome.system.proxy.https", "host", host).Run(); err != nil {
			return err
		}
		if err := exec.Command("gsettings", "set", "org.gnome.system.proxy.https", "port", port).Run(); err != nil {
			return err
		}

		// 设置SOCKS代理
		if err := exec.Command("gsettings", "set", "org.gnome.system.proxy.socks", "host", host).Run(); err != nil {
			return err
		}
		if err := exec.Command("gsettings", "set", "org.gnome.system.proxy.socks", "port", port).Run(); err != nil {
			return err
		}

		// 设置绕过列表
		if config.Bypass != "" {
			// GNOME使用数组格式: ['host1', 'host2']
			domains := strings.Split(config.Bypass, ",")
			for i, domain := range domains {
				domains[i] = fmt.Sprintf("'%s'", strings.TrimSpace(domain))
			}
			bypassList := fmt.Sprintf("[%s]", strings.Join(domains, ", "))
			if err := exec.Command("gsettings", "set", "org.gnome.system.proxy", "ignore-hosts", bypassList).Run(); err != nil {
				return err
			}
		}
	} else {
		// 禁用代理
		if err := exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "none").Run(); err != nil {
			return err
		}
	}

	return nil
}

// clearGNOMEProxy 清除GNOME桌面代理
func (s *LinuxProxyService) clearGNOMEProxy() error {
	return exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "none").Run()
}

// getGNOMEProxy 获取GNOME桌面代理配置
func (s *LinuxProxyService) getGNOMEProxy() (*ProxyConfig, error) {
	config := &ProxyConfig{}

	// 获取代理模式
	cmd := exec.Command("gsettings", "get", "org.gnome.system.proxy", "mode")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	mode := strings.TrimSpace(strings.Trim(string(output), "'"))
	config.Enable = mode == "manual"

	if config.Enable {
		// 获取HTTP代理主机
		cmd = exec.Command("gsettings", "get", "org.gnome.system.proxy.http", "host")
		output, err = cmd.Output()
		if err != nil {
			return nil, err
		}
		host := strings.TrimSpace(strings.Trim(string(output), "'"))

		// 获取HTTP代理端口
		cmd = exec.Command("gsettings", "get", "org.gnome.system.proxy.http", "port")
		output, err = cmd.Output()
		if err != nil {
			return nil, err
		}
		port := strings.TrimSpace(string(output))

		if host != "" && port != "" {
			config.Server = fmt.Sprintf("%s:%s", host, port)
		}

		// 获取绕过列表
		cmd = exec.Command("gsettings", "get", "org.gnome.system.proxy", "ignore-hosts")
		output, err = cmd.Output()
		if err == nil {
			bypass := strings.TrimSpace(string(output))
			// 转换数组格式 ['host1', 'host2'] 为逗号分隔
			bypass = strings.Trim(bypass, "[]")
			bypass = strings.ReplaceAll(bypass, "'", "")
			config.Bypass = bypass
		}
	}

	return config, nil
}

// setKDEProxy 设置KDE桌面代理
func (s *LinuxProxyService) setKDEProxy(config ProxyConfig) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// KDE配置文件路径
	kdeConfigPath := filepath.Join(homeDir, ".config", "kioslaverc")

	proxyType := "0" // 0 = 无代理
	if config.Enable {
		proxyType = "1" // 1 = 手动代理
	}

	// 准备配置内容
	proxyConfig := fmt.Sprintf(`[Proxy Settings]
ProxyType=%s
httpProxy=http://%s
httpsProxy=http://%s
socksProxy=socks://%s
NoProxyFor=%s
`,
		proxyType,
		config.Server,
		config.Server,
		config.Server,
		config.Bypass,
	)

	// 写入配置文件
	if err := os.WriteFile(kdeConfigPath, []byte(proxyConfig), 0644); err != nil {
		return err
	}

	return nil
}

// clearKDEProxy 清除KDE桌面代理
func (s *LinuxProxyService) clearKDEProxy() error {
	return s.setKDEProxy(ProxyConfig{Enable: false})
}

// getKDEProxy 获取KDE桌面代理配置
func (s *LinuxProxyService) getKDEProxy() (*ProxyConfig, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	kdeConfigPath := filepath.Join(homeDir, ".config", "kioslaverc")
	content, err := os.ReadFile(kdeConfigPath)
	if err != nil {
		return &ProxyConfig{}, nil // 文件不存在返回空配置
	}

	config := &ProxyConfig{}
	lines := strings.Split(string(content), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ProxyType=") {
			proxyType := strings.TrimPrefix(line, "ProxyType=")
			config.Enable = proxyType == "1"
		} else if strings.HasPrefix(line, "httpProxy=") {
			proxy := strings.TrimPrefix(line, "httpProxy=")
			proxy = strings.TrimPrefix(proxy, "http://")
			config.Server = proxy
		} else if strings.HasPrefix(line, "NoProxyFor=") {
			config.Bypass = strings.TrimPrefix(line, "NoProxyFor=")
		}
	}

	return config, nil
}
