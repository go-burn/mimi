//go:build windows
// +build windows

package autostart

import (
	"fmt"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

const (
	// Windows 注册表开机启动路径
	runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
)

// isEnabled 检查 Windows 是否已启用开机启动
func (a *AutoStart) isEnabled() (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false, err
	}
	defer key.Close()

	_, _, err = key.GetStringValue(a.appName)
	if err != nil {
		if err == registry.ErrNotExist {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// enable 启用 Windows 开机启动
func (a *AutoStart) enable() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()

	// 确保路径被引号包围以处理包含空格的路径
	execPath := a.appPath
	if !filepath.IsAbs(execPath) {
		execPath, err = filepath.Abs(execPath)
		if err != nil {
			return err
		}
	}

	// Windows 路径需要用引号包裹
	execPath = fmt.Sprintf(`"%s"`, execPath)

	return key.SetStringValue(a.appName, execPath)
}

// disable 禁用 Windows 开机启动
func (a *AutoStart) disable() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()

	// 忽略键不存在的错误
	if err := key.DeleteValue(a.appName); err != nil && err != registry.ErrNotExist {
		return err
	}

	return nil
}
