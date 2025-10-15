package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// GetAppDataDir 获取应用数据目录
// macOS: /Users/<user>/.config/mimi
// Windows: C:\Users\<user>\AppData\Roaming\mimi
// Linux: /home/<user>/.config/mimi
func GetAppDataDir() (string, error) {
	var appDir string
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	switch runtime.GOOS {
	case "windows":
		// Windows: C:\Users\<user>\AppData\Roaming\mimi
		appDataDir := os.Getenv("APPDATA")
		if appDataDir != "" {
			appDir = filepath.Join(appDataDir, "mimi")
		} else {
			appDir = filepath.Join(homeDir, "AppData", "Roaming", "mimi")
		}
	case "darwin":
		// macOS: /Users/<user>/.config/mimi
		appDir = filepath.Join(homeDir, ".config", "mimi")
	default:
		// Linux: /home/<user>/.config/mimi (XDG 标准)
		configDir := os.Getenv("XDG_CONFIG_HOME")
		if configDir == "" {
			configDir = filepath.Join(homeDir, ".config")
		}
		appDir = filepath.Join(configDir, "mimi")
	}

	// 确保目录存在
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return "", err
	}

	return appDir, nil
}

// GetMihomoDir 获取 mihomo 配置目录
// 返回: ~/.config/mimi/mihomo/
func GetMihomoDir() (string, error) {
	appDir, err := GetAppDataDir()
	if err != nil {
		return "", err
	}

	mihomoDir := filepath.Join(appDir, "mihomo")

	// 确保 mihomo 子目录存在
	if err := os.MkdirAll(mihomoDir, 0755); err != nil {
		return "", err
	}

	return mihomoDir, nil
}

// GetConfigPath 获取配置文件路径
func GetConfigPath() (string, error) {
	mihomoDir, err := GetMihomoDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(mihomoDir, "config.yaml"), nil
}

// InitAppDirs 初始化应用目录
func InitAppDirs() error {
	// 创建主目录
	_, err := GetAppDataDir()
	if err != nil {
		return err
	}

	// 创建 mihomo 子目录
	_, err = GetMihomoDir()
	return err
}
