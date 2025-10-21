package main

import (
	"mimi/env"
	"os"
	"time"

	appConfig "mimi/config"
	"mimi/sysproxy"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/dock"
)

var app *application.App

// IsFullyInitialized 标记应用是否完全初始化(导出以供其他包使用)
var IsFullyInitialized = false

func main() {
	// === 第一阶段: 快速基础初始化 ===
	// 1. 初始化应用目录结构
	if err := appConfig.InitAppDirs(); err != nil {
		panic("初始化应用目录失败: " + err.Error())
	}

	// 2. 初始化日志系统
	if err := InitLogger(!env.IsProduction()); err != nil {
		panic("初始化日志系统失败: " + err.Error())
	}
	defer CloseLogger()

	// 配置 sysproxy 日志
	sysproxy.SetLogger(MLog)

	MLog.Info("========== 应用快速启动 ==========")

	// 3. 创建应用实例和托盘(优先显示,提升用户体验)
	dockService := dock.New()
	app = application.New(application.Options{
		Name:        "mimi",
		Description: "基于mihomo的代理桌面应用",
		Icon:        Icon,
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
		},
	})

	dockService.HideAppIcon()
	newMenu()
	newTray()

	MLog.Info("系统托盘已创建,应用图标可见")

	// 4. 检查是否需要启用 TUN 模式（通过环境变量）
	shouldEnableTun := false
	if os.Getenv("MIMI_ENABLE_TUN") == "1" {
		MLog.Info("检测到 MIMI_ENABLE_TUN 环境变量")
		if IsRunningAsRoot() {
			MLog.Info("已具有管理员权限，将在应用启动后启用 TUN 模式")
			shouldEnableTun = true
		} else {
			MLog.Warn("检测到 TUN 启用意图但无 root 权限，无法启用 TUN")
		}
		// 清除环境变量，避免影响子进程
		os.Unsetenv("MIMI_ENABLE_TUN")
	}

	// === 第二阶段: 异步完整初始化 ===
	go func() {
		MLog.Info("========== 开始后台初始化 ==========")

		// 5. 获取应用数据目录
		appDataDir, err := appConfig.GetAppDataDir()
		if err != nil {
			MLog.Error("获取应用数据目录失败", "error", err)
			return
		}
		MLog.Info("应用数据目录", "path", appDataDir)

		// 6. 获取 mihomo 配置目录
		mihomoDir, err := appConfig.GetMihomoDir()
		if err != nil {
			MLog.Error("获取 mihomo 目录失败", "error", err)
			return
		}

		// 7. 初始化 mihomo 配置目录
		MLog.Info("正在初始化 mihomo...")
		if err := InitMihomo(mihomoDir); err != nil {
			MLog.Error("初始化 mihomo 失败", "error", err)
			return
		}

		// 8. 配置 mihomo 日志输出
		// 注意: 必须在 InitMihomo 之后调用,因为 mihomo 的 config.Init() 会重置 logrus 配置
		ConfigureMihomoLogger()

		// 9. 处理 config.js 配置
		MLog.Info("正在处理配置文件...")
		_ = ProcessOverwrite()

		// 10. 应用配置并启动服务
		MLog.Info("正在应用配置...")
		apply()

		// 11. 如果需要启用 TUN 模式
		if shouldEnableTun {
			time.Sleep(500 * time.Millisecond)
			MLog.Info("开始启用 TUN 模式")
			if err := EnableTunMode(); err != nil {
				MLog.Error("启用 TUN 模式失败", "error", err)
			} else {
				MLog.Info("TUN 模式已成功启用")
			}
		}

		IsFullyInitialized = true
		MLog.Info("========== 后台初始化完成 ==========")

		// 12. 启动代理状态监控
		startProxyStatusMonitor()

		// 13. 启动后台更新检查
		startBackgroundUpdateChecker()
	}()

	// 12. 设置信号处理器,确保意外退出时也能清理资源
	SetupSignalHandler(shutdown)
	defer shutdown()

	MLog.Info("应用主循环启动")
	err := app.Run()
	if err != nil {
		MLog.Error("应用运行失败", "error", err)
	}
}
