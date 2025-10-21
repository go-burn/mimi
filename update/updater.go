package update

import (
	"context"
	"fmt"
	"log/slog"
	"mimi/config"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/rhysd/go-github-selfupdate/selfupdate"
)

var (
	// RepoOwner GitHub 仓库所有者
	RepoOwner = "go-burn"
	// RepoName GitHub 仓库名称
	RepoName = "mimi"
)

const (
	EnvGitHubToken = "GITHUB_TOKEN"
)

// UpdateInfo 更新信息
type UpdateInfo struct {
	Current      string // 当前版本
	Latest       string // 最新版本
	URL          string // 下载地址
	HasUpdate    bool   // 是否有更新
	ReleaseNotes string // 发布说明
}

// Updater 更新管理器
type Updater struct {
	logger    *slog.Logger
	version   string // 当前应用版本
	proxyPort int    // 代理端口,0 表示不使用代理
}

// New 创建更新管理器
// version: 当前应用版本,例如 "v1.0.0"
func New(logger *slog.Logger, version string) *Updater {
	return &Updater{
		logger:    logger,
		version:   version,
		proxyPort: 0, // 默认不使用代理
	}
}

// SetProxy 设置代理端口
// port: 本地代理端口,例如 7890
func (u *Updater) SetProxy(port int) {
	u.proxyPort = port
	if port > 0 {
		u.logger.Info("更新器将使用应用代理", "端口", port)
	}
}

// getGitHubToken 获取 GitHub token
// 优先级: 1. 配置文件 2. 环境变量
func getGitHubToken(logger *slog.Logger) string {
	// 1. 尝试从配置文件读取 (在用户主目录下)
	if token := readTokenFromConfigFile(logger); token != "" {
		logger.Debug("使用配置文件中的 GitHub token")
		return token
	}

	// 2. 从环境变量读取
	if token := os.Getenv(EnvGitHubToken); token != "" {
		logger.Debug("使用环境变量中的 GitHub token")
		return token
	}

	// 未找到 token,使用匿名访问 (60 req/hour 限制)
	logger.Debug("未找到 GitHub token,使用匿名访问 (速率限制: 60 req/hour)")
	return ""
}

// readTokenFromConfigFile 从配置文件读取 token
// 配置文件位置: ~/.config/mimi/github_token
func readTokenFromConfigFile(logger *slog.Logger) string {
	// 获取用户主目录
	homeDir, err := config.GetAppDataDir()
	if err != nil {
		return ""
	}

	data, err := os.ReadFile(homeDir + "/github_token")
	if err != nil {
		return ""
	}

	token := strings.TrimSpace(string(data))
	if token != "" {
		return token
	}
	return ""
}

// createUpdater 创建带认证和超时设置的 Updater
func createUpdater(logger *slog.Logger, ctx context.Context, proxyPort int) *selfupdate.Updater {
	token := getGitHubToken(logger)

	config := selfupdate.Config{
		APIToken: token,
	}

	updater, err := selfupdate.NewUpdater(config)
	if err != nil {
		// 如果创建失败,回退到默认 Updater
		logger.Warn("创建认证 Updater 失败,使用默认配置", "error", err)
		return selfupdate.DefaultUpdater()
	}

	// 代理配置将在执行下载时临时设置
	if proxyPort > 0 {
		logger.Info("更新下载将使用应用代理", "端口", proxyPort)
	}

	if token != "" {
		logger.Info("已启用 GitHub API 认证 (速率限制: 5000 req/hour)")
	} else {
		// 未找到 token 的提示
		logger.Debug("使用匿名访问",
			"速率限制", "60 req/hour",
			"提示", "如果这是私有仓库,请设置 GITHUB_TOKEN 环境变量")
	}

	return updater
}

// CheckForUpdates 检查是否有新版本
func (u *Updater) CheckForUpdates(ctx context.Context) (*UpdateInfo, error) {
	// 判断是否是开发版本 (dev, dev1, dev2 等或空字符串)
	isDev := u.version == "" || strings.HasPrefix(strings.ToLower(u.version), "dev")

	if isDev {
		u.logger.Info("检查更新中...", "当前版本", fmt.Sprintf("%s (开发版本)", u.version))
	} else {
		u.logger.Info("检查更新中...", "当前版本", u.version)
	}

	// 创建带认证的 Updater
	updater := createUpdater(u.logger, ctx, u.proxyPort)

	// 构建仓库 slug
	slug := fmt.Sprintf("%s/%s", RepoOwner, RepoName)

	// 检测最新版本 (使用带认证的 Updater)
	latest, found, err := updater.DetectLatest(slug)
	if err != nil {
		// 检查是否是私有仓库访问被拒绝
		errMsg := err.Error()
		if strings.Contains(errMsg, "403") || strings.Contains(errMsg, "404") {
			token := getGitHubToken(u.logger)
			if token == "" {
				return nil, fmt.Errorf("检测更新失败: %w\n\n提示: 如果这是私有仓库,请配置 GitHub token:\n  1. 创建文件 ~/.mimi/github_token\n  2. 将 token 写入文件", err)
			}
		}
		return nil, fmt.Errorf("检测更新失败: %w", err)
	}

	if !found {
		return nil, fmt.Errorf("未找到任何版本发布")
	}

	// 对于 dev 版本,默认总是有更新
	var hasUpdate bool
	if isDev {
		hasUpdate = true
		u.logger.Info("开发版本,建议更新到最新正式版本", "最新版本", latest.Version.String())
	} else {
		// 解析当前版本
		currentVersion, err := semver.ParseTolerant(u.version)
		if err != nil {
			// 如果解析失败,也当作开发版本处理
			u.logger.Warn("无法解析当前版本,将作为开发版本处理", "版本", u.version, "错误", err)
			hasUpdate = true
		} else {
			// 比较版本
			hasUpdate = latest.Version.GT(currentVersion)

			if hasUpdate {
				u.logger.Info("发现新版本", "最新版本", latest.Version.String())
			} else {
				u.logger.Info("当前已是最新版本")
			}
		}
	}

	info := &UpdateInfo{
		Current:      u.version,
		Latest:       latest.Version.String(),
		URL:          latest.AssetURL,
		HasUpdate:    hasUpdate,
		ReleaseNotes: latest.ReleaseNotes,
	}

	return info, nil
}

// ApplyUpdate 执行更新
func (u *Updater) ApplyUpdate(ctx context.Context) error {
	// 判断是否是开发版本 (dev, dev1, dev2 等或空字符串)
	isDev := u.version == "" || strings.HasPrefix(strings.ToLower(u.version), "dev")

	if isDev {
		u.logger.Info("开始下载更新...", "当前版本", fmt.Sprintf("%s (开发版本)", u.version))
	} else {
		u.logger.Info("开始下载更新...", "当前版本", u.version)
	}

	// 创建带认证的 Updater
	updater := createUpdater(u.logger, ctx, u.proxyPort)

	// 构建仓库 slug
	slug := fmt.Sprintf("%s/%s", RepoOwner, RepoName)

	// 检测最新版本 (使用带认证的 Updater)
	latest, found, err := updater.DetectLatest(slug)
	if err != nil {
		// 检查是否是私有仓库访问被拒绝
		errMsg := err.Error()
		if strings.Contains(errMsg, "403") || strings.Contains(errMsg, "404") {
			token := getGitHubToken(u.logger)
			if token == "" {
				return fmt.Errorf("检测更新失败: %w\n\n提示: 如果这是私有仓库,请配置 GitHub token", err)
			}
		}
		return fmt.Errorf("检测更新失败: %w", err)
	}

	if !found {
		return fmt.Errorf("未找到更新")
	}

	// 对于非开发版本,检查版本号
	if !isDev {
		// 解析当前版本
		currentVersion, err := semver.ParseTolerant(u.version)
		if err != nil {
			// 如果解析失败,记录警告但继续更新(当作开发版本处理)
			u.logger.Warn("无法解析当前版本,将继续更新", "版本", u.version, "错误", err)
		} else {
			// 确保有新版本
			if !latest.Version.GT(currentVersion) {
				return fmt.Errorf("当前已是最新版本")
			}
		}
	}

	// 获取当前可执行文件路径
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取可执行文件路径失败: %w", err)
	}

	u.logger.Info("正在下载更新包...",
		"从", u.version,
		"到", latest.Version.String(),
		"下载地址", latest.AssetURL,
		"文件大小", formatBytes(int64(latest.AssetByteSize)))

	u.logger.Info("开始下载并应用更新...", "目标可执行文件", exe)

	// 临时设置使用 mihomo 代理 (如果配置了)
	var originalTransport http.RoundTripper
	if u.proxyPort > 0 {
		proxyURL, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", u.proxyPort))
		if err == nil {
			u.logger.Info("使用应用代理下载更新", "代理地址", proxyURL.String())

			// 保存原始 transport
			originalTransport = http.DefaultTransport

			// 创建使用 HTTP 代理的 transport
			// 这样所有HTTP请求都会通过 mihomo 的混合端口
			http.DefaultTransport = &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          100,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			}

			// 确保下载完成后恢复原始 transport
			defer func() {
				http.DefaultTransport = originalTransport
				u.logger.Info("已恢复默认网络配置")
			}()
		} else {
			u.logger.Warn("配置代理URL失败", "error", err)
		}
	}

	// 在goroutine中执行下载和更新,以便能够监控超时
	// UpdateTo 会自动: 1. 下载压缩包 2. 解压 3. 替换可执行文件
	downloadDone := make(chan error, 1)
	go func() {
		downloadDone <- updater.UpdateTo(latest, exe)
	}()

	// 等待下载完成或超时
	select {
	case err := <-downloadDone:
		if err != nil {
			return fmt.Errorf("下载更新失败: %w", err)
		}
	case <-ctx.Done():
		return fmt.Errorf("下载更新超时: %w", ctx.Err())
	}

	u.logger.Info("✅ 更新成功!准备重启应用以应用更新")
	return nil
}

// GetCurrentVersion 获取当前版本
func (u *Updater) GetCurrentVersion() string {
	if u.version == "dev" || u.version == "" {
		return "dev"
	}
	return u.version
}

// GetPlatformInfo 获取平台信息(用于调试)
func (u *Updater) GetPlatformInfo() string {
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}

// formatBytes 格式化字节数为可读格式
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
