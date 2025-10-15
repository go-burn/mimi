//go:build darwin
// +build darwin

package autostart

import (
	"os"
)

// isEnabled 检查 macOS 是否已启用开机启动
func (a *AutoStart) isEnabled() (bool, error) {
	plistPath, err := a.getLaunchAgentPath()
	if err != nil {
		return false, err
	}

	_, err = os.Stat(plistPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// enable 启用 macOS 开机启动
func (a *AutoStart) enable() error {
	plistPath, err := a.getLaunchAgentPath()
	if err != nil {
		return err
	}

	content := a.getPlistContent()
	return os.WriteFile(plistPath, []byte(content), 0644)
}

// disable 禁用 macOS 开机启动
func (a *AutoStart) disable() error {
	plistPath, err := a.getLaunchAgentPath()
	if err != nil {
		return err
	}

	// 忽略文件不存在的错误
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}
