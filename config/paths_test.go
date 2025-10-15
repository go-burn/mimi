package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetAppDataDir(t *testing.T) {
	appDir, err := GetAppDataDir()
	if err != nil {
		t.Fatalf("获取应用数据目录失败: %v", err)
	}

	t.Logf("应用数据目录: %s", appDir)

	// 验证目录存在
	if _, err := os.Stat(appDir); os.IsNotExist(err) {
		t.Errorf("应用数据目录不存在: %s", appDir)
	}

	// 验证路径包含 .mimi
	if !filepath.IsAbs(appDir) {
		t.Errorf("应用数据目录不是绝对路径: %s", appDir)
	}
}

func TestInitAppDirs(t *testing.T) {
	err := InitAppDirs()
	if err != nil {
		t.Fatalf("初始化应用目录失败: %v", err)
	}

	// 验证主目录存在
	appDir, _ := GetAppDataDir()
	if _, err := os.Stat(appDir); os.IsNotExist(err) {
		t.Errorf("主目录不存在: %s", appDir)
	} else {
		t.Logf("主目录已创建: %s", appDir)
	}
}

func TestGetConfigPath(t *testing.T) {
	configPath, err := GetConfigPath()
	if err != nil {
		t.Fatalf("获取配置文件路径失败: %v", err)
	}

	t.Logf("配置文件路径: %s", configPath)

	// 验证路径包含 config.yaml
	if filepath.Base(configPath) != "config.yaml" {
		t.Errorf("配置文件名不正确: %s", configPath)
	}
}
