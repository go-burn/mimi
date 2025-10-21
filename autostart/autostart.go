package autostart

import (
	"fmt"
	"mimi/env"
	"os"
	"path/filepath"
)

// AutoStart 开机启动管理器
type AutoStart struct {
	appName string
	appPath string
}

// New 创建开机启动管理器
func New() *AutoStart {
	execPath, _ := os.Executable()
	appName := "com.goburn.mimi"
	if !env.IsProduction() {
		appName = "com.goburn.mimidev"
	}
	return &AutoStart{
		appName: appName,
		appPath: execPath,
	}
}

// IsEnabled 检查是否已启用开机启动
func (a *AutoStart) IsEnabled() (bool, error) {
	return a.isEnabled()
}

// Enable 启用开机启动
func (a *AutoStart) Enable() error {
	return a.enable()
}

// Disable 禁用开机启动
func (a *AutoStart) Disable() error {
	return a.disable()
}

// getLaunchAgentPath 获取 macOS LaunchAgent plist 文件路径
func (a *AutoStart) getLaunchAgentPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	launchAgentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	if err := os.MkdirAll(launchAgentsDir, 0755); err != nil {
		return "", err
	}

	return filepath.Join(launchAgentsDir, fmt.Sprintf("%s.plist", a.appName)), nil
}

// getPlistContent 生成 macOS plist 文件内容
func (a *AutoStart) getPlistContent() string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <false/>
    <key>ProcessType</key>
    <string>Interactive</string>
    <key>StandardOutPath</key>
    <string>/tmp/%s.out.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/%s.err.log</string>
</dict>
</plist>`, a.appName, a.appPath, a.appName, a.appName)
}

func (a *AutoStart) State() bool {
	enabled, err := a.isEnabled()
	if err != nil {
		return false
	}
	return enabled
}
