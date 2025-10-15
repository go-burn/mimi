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
)

var menu *application.Menu
var systemProxyService *sysproxy.SystemProxy
var systemProxyCheckbox *application.MenuItem
var tunProxyCheckbox *application.MenuItem
var autoStartService *autostart.AutoStart
var autoStartCheckbox *application.MenuItem

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
	menu.Add("退出应用").
		SetAccelerator("CmdOrCtrl+Q").
		OnClick(func(_ *application.Context) {
			app.Quit()
		})
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
