package main

import (
	_ "embed"
	"fmt"
	"runtime"
	"strings"
)

//go:embed go.mod
var goModContent string

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

	// MihomoVersion mihomo 核心版本 (从嵌入的 go.mod 解析)
	MihomoVersion = parseMihomoVersion()
)

// parseMihomoVersion 从嵌入的 go.mod 解析 mihomo 版本
func parseMihomoVersion() string {
	lines := strings.Split(goModContent, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "github.com/metacubex/mihomo") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return fields[1] // 返回版本号如 v1.19.15
			}
		}
	}
	return "unknown"
}

// GetMihomoVersion 获取 mihomo 版本
func GetMihomoVersion() string {
	if MihomoVersion != "unknown" {
		return "Meta@" + MihomoVersion
	}
	return "Meta@unknown"
}

// GetVersion 返回简短版本信息
func GetVersion() string {
	return Version
}

// GetVersionInfo 返回完整的版本信息字符串
func GetVersionInfo() string {
	return fmt.Sprintf(
		"版本: %s\n构建时间: %s\n提交: %s\n分支: %s\nGo 版本: %s\n平台: %s\nMihomo: %s",
		Version,
		BuildTime,
		GitCommit,
		GitBranch,
		GoVersion,
		Platform,
		GetMihomoVersion(),
	)
}

// GetVersionShort 返回紧凑的版本信息 (单行)
func GetVersionShort() string {
	return fmt.Sprintf("%s (%s %s)", Version, GitCommit, Platform)
}
