package main

import (
	"fmt"
	"runtime"
)

// 版本信息变量 - 通过构建时 ldflags 注入
var (
	// Version 应用版本号 (例如: v1.0.0 或 git describe 输出)
	Version = "dev"

	// BuildTime 构建时间 (ISO 8601 格式: 2025-01-16T10:30:00Z)
	BuildTime = "unknown"

	// GitCommit Git 提交哈希 (短格式, 例如: abc1234)
	GitCommit = "unknown"

	// GitBranch Git 分支名称 (例如: main, develop)
	GitBranch = "unknown"

	// GoVersion Go 编译器版本 (运行时自动获取)
	GoVersion = runtime.Version()

	// Platform 目标平台 (例如: darwin/arm64, windows/amd64)
	Platform = runtime.GOOS + "/" + runtime.GOARCH
)

// GetVersion 返回简短版本信息
func GetVersion() string {
	return Version
}

// GetVersionInfo 返回完整的版本信息字符串
func GetVersionInfo() string {
	return fmt.Sprintf(
		"版本: %s\n构建时间: %s\n提交: %s\n分支: %s\nGo 版本: %s\n平台: %s",
		Version,
		BuildTime,
		GitCommit,
		GitBranch,
		GoVersion,
		Platform,
	)
}

// GetVersionShort 返回紧凑的版本信息 (单行)
func GetVersionShort() string {
	return fmt.Sprintf("%s (%s %s)", Version, GitCommit, Platform)
}
