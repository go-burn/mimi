package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/metacubex/mihomo/adapter/outboundgroup"
	"github.com/metacubex/mihomo/common/utils"
	"github.com/metacubex/mihomo/component/profile/cachefile"
	"github.com/wailsapp/wails/v3/pkg/application"

	"mimi/autostart"
	appConfig "mimi/config"
	"mimi/sysproxy"
	"mimi/update"
)

var menu *application.Menu
var systemProxyService *sysproxy.SystemProxy
var systemProxyCheckbox *application.MenuItem
var tunProxyCheckbox *application.MenuItem
var autoStartService *autostart.AutoStart
var autoStartCheckbox *application.MenuItem
var versionMenuItem *application.MenuItem
var latestUpdateInfo *update.UpdateInfo // 缓存最新的更新信息

// 代理状态检测器
var proxyStatusChecker *ProxyStatusChecker

// getProxyStatusText 获取状态显示文本(桥接函数)
func getProxyStatusText() (icon string, text string) {
	if proxyStatusChecker == nil {
		return "⚪", "未代理"
	}
	return proxyStatusChecker.GetStatusText()
}

// startProxyStatusMonitor 启动代理状态后台监控(桥接函数)
func startProxyStatusMonitor() {
	if proxyStatusChecker == nil {
		proxyStatusChecker = NewProxyStatusChecker()
	}
	proxyStatusChecker.StartMonitor()
}

func newMenu() *application.Menu {
	if menu == nil {
		menu = app.NewMenu()

		// 初始化系统代理服务
		if systemProxyService == nil {
			var err error
			systemProxyService, err = sysproxy.NewSystemProxy()
			if err != nil {
				MLog.Error("创建系统代理服务失败", "error", err)
			}
		}

		// 初始化开机启动服务
		if autoStartService == nil {
			// 获取当前可执行文件路径
			autoStartService = autostart.New()
		}

		commonMenu()
		quitMenu()
	}
	return menu
}

func commonMenu() {
	// 显示代理运行状态
	icon, statusText := getProxyStatusText()
	menu.Add(icon + " " + statusText).SetEnabled(false)
	menu.AddSeparator()

	settingMenu := menu.AddSubmenu("配置管理")
	settingMenu.Add("刷新配置").OnClick(func(_ *application.Context) {
		// 检查是否完全初始化
		if !IsFullyInitialized {
			dialog := application.InfoDialog()
			dialog.SetTitle("初始化中")
			dialog.SetMessage("应用正在后台初始化,请稍候...")
			dialog.Show()
			return
		}

		err := ProcessOverwrite()
		if err != nil {
			dialog := application.InfoDialog()
			dialog.SetMessage(err.Error())
			dialog.Show()
			return
		}
		apply()
	})
	settingMenu.Add("修改覆写").OnClick(func(_ *application.Context) {
		// 获取应用数据目录下的 config.js 文件路径
		appDataDir, err := appConfig.GetAppDataDir()
		if err != nil {
			MLog.Error("获取应用数据目录失败", "error", err)
			return
		}
		path := filepath.Join(appDataDir, ConfigJS)

		// 使用编辑器打开文件
		opener, err := NewEditorOpener()
		if err != nil {
			MLog.Error("创建编辑器打开器失败", "error", err)
			return
		}

		if err := opener.OpenWithEditor(path); err != nil {
			MLog.Error("打开文件失败", "error", err)
		}
	})
	settingMenu.AddSeparator()
	// 开机启动菜单项
	isAutoStartEnabled := autoStartService.State()
	autoStartCheckbox = settingMenu.AddCheckbox("开机启动", isAutoStartEnabled).OnClick(func(_ *application.Context) {
		if isAutoStartEnabled {
			// 当前已启用,则禁用
			if err := autoStartService.Disable(); err != nil {
				MLog.Error("禁用开机启动失败", "error", err)
				return
			}
			MLog.Info("已禁用开机启动")
		} else {
			// 当前已禁用,则启用
			if err := autoStartService.Enable(); err != nil {
				MLog.Error("启用开机启动失败", "error", err)
				return
			}
			MLog.Info("已启用开机启动")
		}
		autoStartCheckbox.SetChecked(!isAutoStartEnabled)
	})

	if windowURL != "" {
		menu.Add("显示面板").
			SetAccelerator("CmdOrCtrl+D").
			OnClick(func(_ *application.Context) {
				createWindow(app)
			})
	}
	menu.AddSeparator()

	// 获取初始系统代理状态
	isProxyEnabled := systemProxyService.StateProxy()
	// 添加系统代理菜单项
	systemProxyCheckbox = menu.AddCheckbox("系统代理", isProxyEnabled).OnClick(func(_ *application.Context) {
		// 检查是否完全初始化
		if !IsFullyInitialized {
			dialog := application.InfoDialog()
			dialog.SetTitle("初始化中")
			dialog.SetMessage("应用正在后台初始化,请稍候...")
			dialog.Show()
			return
		}

		// 动态读取当前系统代理状态,避免使用闭包捕获的变量
		currentProxyState := systemProxyService.StateProxy()
		newProxyState := !currentProxyState
		// 根据目标状态执行相应操作
		if currentProxyState {
			// 当前已启用,则禁用
			if err := systemProxyService.ClearProxy(); err != nil {
				MLog.Error("禁用系统代理失败", "error", err)
				return
			}
		} else {
			// 当前已禁用,则启用
			byPass, _ := OVM.ByPass()
			if err := systemProxyService.EnableProxy(fmt.Sprintf("127.0.0.1:%d", mcfg.General.MixedPort), byPass...); err != nil {
				MLog.Error("启用系统代理失败", "error", err)
				return
			}
		}

		// 更新菜单复选框状态
		systemProxyCheckbox.SetChecked(newProxyState)
	})
}

func quitMenu() {
	menu.AddSeparator()

	// 添加版本信息菜单项 - 可点击查看更新
	versionText := "版本: " + GetVersion()
	if latestUpdateInfo != nil && latestUpdateInfo.HasUpdate {
		versionText = versionText + " ✨"
	}

	versionMenuItem = menu.Add(versionText).OnClick(func(_ *application.Context) {
		// 每次点击都重新检查更新
		checkAndUpdateOrNotify()
	})

	menu.Add("关于").OnClick(func(_ *application.Context) {
		dialog := application.InfoDialog()
		dialog.SetTitle("关于 Mimi")
		dialog.SetIcon(Icon)
		dialog.SetMessage(GetVersionInfo())
		dialog.Show()
	})

	menu.AddSeparator()
	menu.Add("退出应用").
		SetAccelerator("CmdOrCtrl+Q").
		OnClick(func(_ *application.Context) {
			app.Quit()
		})
}

// checkAndUpdateOrNotify 检查更新，有更新则直接下载，无更新则提示
func checkAndUpdateOrNotify() {
	updater := update.New(MLog, GetVersion())
	// 如果 mihomo 已初始化,使用应用代理下载更新
	if mcfg != nil && mcfg.General != nil && mcfg.General.MixedPort > 0 {
		updater.SetProxy(mcfg.General.MixedPort)
	}

	// 在后台检查更新
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// 检查更新
		info, err := updater.CheckForUpdates(ctx)
		if err != nil {
			MLog.Error("检查更新失败", "error", err)
			dialog := application.InfoDialog()
			dialog.SetTitle("检查更新失败")
			dialog.SetMessage(fmt.Sprintf("无法检查更新: %v", err))
			dialog.Show()
			return
		}

		// 缓存更新信息
		latestUpdateInfo = info

		if !info.HasUpdate {
			// 没有更新，显示提示
			dialog := application.InfoDialog()
			dialog.SetTitle("已是最新版本")
			dialog.SetMessage(fmt.Sprintf("当前版本: %s\n\n您已经在使用最新版本了!", info.Current))
			dialog.Show()
			return
		}

		// 有更新，直接开始下载
		dialog := application.InfoDialog()
		dialog.SetTitle("开始更新")
		dialog.SetMessage(fmt.Sprintf("正在下载更新: %s -> %s\n\n请稍候,下载完成后将自动重启...", info.Current, info.Latest))
		dialog.Show()

		// 执行自动更新
		performAutoUpdateAndRefreshMenu(updater)
	}()
}

// checkForUpdatesAndShow 后台检查更新并更新菜单显示（不弹窗提示）
func checkForUpdatesAndShow() {
	updater := update.New(MLog, GetVersion())
	// 如果 mihomo 已初始化,使用应用代理检查更新
	if mcfg != nil && mcfg.General != nil && mcfg.General.MixedPort > 0 {
		updater.SetProxy(mcfg.General.MixedPort)
	}

	// 在后台检查更新,避免阻塞UI
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		info, err := updater.CheckForUpdates(ctx)
		if err != nil {
			MLog.Error("后台检查更新失败", "error", err)
			return
		}

		// 缓存更新信息
		latestUpdateInfo = info

		// 更新版本菜单显示
		updateVersionMenuItem()

		if info.HasUpdate {
			MLog.Info("后台检查发现新版本", "当前", info.Current, "最新", info.Latest)
		} else {
			MLog.Debug("后台检查：当前已是最新版本")
		}
	}()
}

// performAutoUpdateAndRefreshMenu 执行自动更新流程并在完成后刷新菜单
func performAutoUpdateAndRefreshMenu(updater *update.Updater) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// 应用更新
	if err := updater.ApplyUpdate(ctx); err != nil {
		MLog.Error("自动更新失败", "error", err)
		dialog := application.InfoDialog()
		dialog.SetTitle("更新失败")
		dialog.SetMessage(fmt.Sprintf("自动更新失败: %v", err))
		dialog.Show()

		// 更新失败后，重新检查并刷新菜单
		latestUpdateInfo = nil
		updateVersionMenuItem()
		return
	}

	MLog.Info("更新下载完成,准备重启应用")

	// 检测当前是否以管理员权限运行
	needAdmin := IsRunningAsRoot()
	var envVars []string

	// 如果当前有 TUN 模式权限,重启后也需要保持
	if needAdmin {
		MLog.Info("检测到当前以管理员权限运行,重启后将保持权限")
		// 检查 TUN 是否启用,如果启用则传递环境变量
		if mcfg != nil && mcfg.General != nil && mcfg.General.Tun.Enable {
			envVars = append(envVars, "MIMI_ENABLE_TUN=1")
			MLog.Info("TUN 模式已启用,重启后将自动启用")
		}
	}

	// 调度延迟重启(1秒后启动新进程),根据当前权限决定重启方式
	// 新进程会在1秒后启动,给当前进程足够时间优雅退出并释放端口
	if err := RestartApplication(needAdmin, envVars...); err != nil {
		MLog.Error("调度重启失败", "error", err)
		dialog := application.InfoDialog()
		dialog.SetTitle("更新成功")
		dialog.SetMessage("更新已完成,但自动重启失败。\n\n请手动重启应用以应用更新。")
		dialog.Show()
		return
	}

	MLog.Info("已调度延迟重启,立即优雅退出当前进程释放端口")

	// 立即优雅退出当前进程,释放所有端口和资源
	// 新进程会在1秒后自动启动
	GracefulExit()
}

// performAutoUpdate 执行自动更新流程
func performAutoUpdate(updater *update.Updater) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// 应用更新
	if err := updater.ApplyUpdate(ctx); err != nil {
		MLog.Error("自动更新失败", "error", err)
		dialog := application.InfoDialog()
		dialog.SetTitle("更新失败")
		dialog.SetMessage(fmt.Sprintf("自动更新失败: %v\n\n请尝试手动下载更新", err))
		dialog.Show()
		return
	}

	MLog.Info("更新下载完成,准备重启应用")

	// 检测当前是否以管理员权限运行
	needAdmin := IsRunningAsRoot()
	var envVars []string

	// 如果当前有 TUN 模式权限,重启后也需要保持
	if needAdmin {
		MLog.Info("检测到当前以管理员权限运行,重启后将保持权限")
		// 检查 TUN 是否启用,如果启用则传递环境变量
		if mcfg != nil && mcfg.General != nil && mcfg.General.Tun.Enable {
			envVars = append(envVars, "MIMI_ENABLE_TUN=1")
			MLog.Info("TUN 模式已启用,重启后将自动启用")
		}
	}

	// 调度延迟重启(1秒后启动新进程),根据当前权限决定重启方式
	// 新进程会在1秒后启动,给当前进程足够时间优雅退出并释放端口
	if err := RestartApplication(needAdmin, envVars...); err != nil {
		MLog.Error("调度重启失败", "error", err)
		dialog := application.InfoDialog()
		dialog.SetTitle("更新成功")
		dialog.SetMessage("更新已完成,但自动重启失败。\n\n请手动重启应用以应用更新。")
		dialog.Show()
		return
	}

	MLog.Info("已调度延迟重启,立即优雅退出当前进程释放端口")

	// 立即优雅退出当前进程,释放所有端口和资源
	// 新进程会在1秒后自动启动
	GracefulExit()
}

// updateVersionMenuItem 更新版本菜单项的显示
func updateVersionMenuItem() {
	if versionMenuItem == nil {
		return
	}

	versionText := "版本: " + GetVersion()
	if latestUpdateInfo != nil && latestUpdateInfo.HasUpdate {
		versionText = versionText + " ✨"
	}

	versionMenuItem.SetLabel(versionText)
}

// truncateReleaseNotes 截断过长的发布说明
func truncateReleaseNotes(notes string, maxLen int) string {
	if len(notes) <= maxLen {
		return notes
	}
	return notes[:maxLen] + "..."
}

// startBackgroundUpdateChecker 启动后台更新检查
// 在应用完全初始化后调用,定期检查更新
func startBackgroundUpdateChecker() {
	// 等待应用完全初始化
	go func() {
		// 等待5分钟后首次检查
		time.Sleep(5 * time.Minute)

		// 首次检查
		checkForUpdatesAndShow()

		// 之后每24小时检查一次
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			checkForUpdatesAndShow()
		}
	}()
}

func tunMenu() {
	// 获取当前 TUN 状态
	isTunEnabled := false
	if mcfg != nil && mcfg.General != nil {
		isTunEnabled = mcfg.General.Tun.Enable
	}

	tunProxyCheckbox = menu.AddCheckbox("虚拟网卡", isTunEnabled).OnClick(func(_ *application.Context) {
		// 动态检查权限（不要使用闭包捕获的变量）
		if !IsRunningAsRoot() {
			MLog.Warn("TUN 模式需要管理员权限")
			// 请求权限并重启(RequestAdminPrivilege 内部会处理退出)
			// 传递 true 表示需要在获得权限后启用 TUN
			if err := RequestAdminPrivilege(true); err != nil {
				errorDialog := application.InfoDialog()
				errorDialog.SetMessage(fmt.Sprintf("请求权限失败: %v", err))
				errorDialog.Show()
			}
			return
		}

		// 已有权限，切换 TUN 状态
		// 动态获取当前状态，不使用闭包捕获的变量
		currentTunState := false
		if mcfg != nil && mcfg.General != nil {
			currentTunState = mcfg.General.Tun.Enable
		}
		newTunState := !currentTunState
		MLog.Info("切换 TUN 模式", "当前状态", currentTunState, "目标状态", newTunState)

		// 修改配置并重新加载
		if err := toggleTunMode(newTunState); err != nil {
			MLog.Error("切换 TUN 模式失败", "error", err)
			errorDialog := application.InfoDialog()
			errorDialog.SetMessage(fmt.Sprintf("切换 TUN 模式失败: %v", err))
			errorDialog.Show()
			return
		}

		// 更新菜单状态
		tunProxyCheckbox.SetChecked(newTunState)
		MLog.Info("TUN 模式已切换", "enable", newTunState)
	})
}

// EnableTunMode 启用 TUN 模式(用于权限提升后自动启用)
func EnableTunMode() error {
	return toggleTunMode(true)
}

// toggleTunMode 切换 TUN 模式
func toggleTunMode(enable bool) error {
	// 1. 读取配置
	vm, err := NewOverwriteVm()
	if err != nil {
		return fmt.Errorf("解析 config.js 失败: %v", err)
	}

	configData := make(map[string]interface{})
	processedConfig, err := vm.Main(configData)
	if err != nil {
		MLog.Warn("执行 config.js main 函数失败，使用默认配置", "error", err)
		processedConfig = configData
	}

	// 2. 修改 TUN 配置
	if processedConfig["tun"] == nil {
		// 如果没有 TUN 配置，创建默认配置
		processedConfig["tun"] = map[string]interface{}{
			"enable":                enable,
			"stack":                 "mixed",
			"auto-route":            true,
			"auto-detect-interface": true,
			"dns-hijack":            []string{"any:53", "tcp://any:53"},
			"strict-route":          true,
		}
	} else {
		// 修改现有配置
		if tunConfig, ok := processedConfig["tun"].(map[string]interface{}); ok {
			tunConfig["enable"] = enable
		}
	}

	// 3. 写入配置文件
	if err := WriteConfigYAML(processedConfig); err != nil {
		return fmt.Errorf("写入配置文件失败: %v", err)
	}

	// 4. 重新加载配置
	apply()

	return nil
}

func refreshMenu() {
	// 构建菜单
	menu.Clear()
	commonMenu()
	tunMenu()
	menu.AddSeparator()
	allProxies := getAllProxy()
	groupAll, renameMap := allProxies.executeScript()
	for _, group := range getProxyGroup() {
		newAll := groupAll[group.Name]
		groupName := group.Name
		sub := menu.AddSubmenu(groupName + allProxies.Delay(groupName))

		finalProxyName := allProxies.FinalProxy(group.Name)
		if finalProxyName != "" && finalProxyName != group.Name {
			if rename, ok := renameMap[finalProxyName]; ok {
				sub.Add(rename).SetEnabled(false)
			}
		}

		sub.Add("重新测试").OnClick(func(_ *application.Context) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			// 解析期望的 HTTP 状态码范围
			expectedStatus, err := utils.NewUnsignedRanges[uint16]("204")
			if err != nil {
				MLog.Error("解析期望状态码失败", "group", group.Name, "error", err)
				return
			}

			MLog.Info("开始测试代理组延迟", "group", group.Name)
			//调用代理组的 URLTest 方法
			delayMap, err := group.URLTest(ctx, "https://cp.cloudflare.com/generate_204", expectedStatus)
			if err != nil {
				MLog.Error("测试失败", "group", group.Name, "error", err)
				return
			}
			MLog.Info("测试完成", "group", group.Name, "result", delayMap)
		})

		sub.AddSeparator()
		for _, newProxy := range newAll {
			displayName := newProxy["name"].(string)
			proxyName := newProxy["_originalName"].(string)
			proxy := allProxies.Get(proxyName)
			sub.AddRadio(displayName+allProxies.Delay(proxyName), proxyName == group.Now).OnClick(func(_ *application.Context) {
				dialog := application.InfoDialog()
				selector, ok := group.ProxyAdapter.(outboundgroup.SelectAble)
				if !ok {
					dialog.SetMessage("Must be a Selector " + proxyName)
					dialog.Show()
					return
				}

				if err := selector.Set(proxyName); err != nil {
					dialog.SetMessage(fmt.Sprintf("切换代理失败: %s", err.Error()))
					dialog.Show()
					return
				}
				cachefile.Cache().SetSelected(proxy.Name(), proxyName)
			})
		}
	}
	quitMenu()
	menu.Update()

	// Windows 下需要重新设置托盘菜单才能生效
	if systemTray != nil {
		systemTray.SetMenu(menu)
	}
}
