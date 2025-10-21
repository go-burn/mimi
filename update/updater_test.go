package update

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"
)

// TestCheckForUpdates 测试检查更新功能
func TestCheckForUpdates(t *testing.T) {
	// 创建日志
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// 设置测试仓库
	RepoOwner = "wbreezeee"
	RepoName = "mimi"

	// 创建更新器 (使用一个很老的版本号来测试)
	updater := New(logger, "v0.0.1")

	// 创建超时上下文
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 检查更新
	info, err := updater.CheckForUpdates(ctx)
	if err != nil {
		t.Logf("检查更新失败(这可能是正常的,如果仓库还没有发布): %v", err)
		return
	}

	t.Logf("当前版本: %s", info.Current)
	t.Logf("最新版本: %s", info.Latest)
	t.Logf("是否有更新: %v", info.HasUpdate)
	t.Logf("下载地址: %s", info.URL)
	t.Logf("发布说明: %s", info.ReleaseNotes)
}

// TestGetVersion 测试获取版本信息
func TestGetVersion(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	updater := New(logger, "v1.0.0")

	version := updater.GetCurrentVersion()
	if version != "v1.0.0" {
		t.Errorf("期望版本 v1.0.0, 实际得到 %s", version)
	}

	t.Logf("当前版本: %s", version)
}

// TestGetPlatformInfo 测试获取平台信息
func TestGetPlatformInfo(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	updater := New(logger, "v1.0.0")

	platform := updater.GetPlatformInfo()
	t.Logf("平台信息: %s", platform)

	if platform == "" {
		t.Error("平台信息不应为空")
	}
}
