package main

import (
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

var window *application.WebviewWindow
var windowURL = ""
var trafficWindow *application.WebviewWindow

const (
	panelWindowWidth  = 1000
	panelWindowHeight = 750
)

func createWindow(app *application.App) {
	if windowURL == "" {
		return
	}

	// 如果窗口已存在,显示并聚焦窗口
	if window != nil {
		window.Show()
		window.Focus()
		return
	}

	// 创建新窗口
	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Width:  panelWindowWidth,
		Height: panelWindowHeight,
		URL:    windowURL,
	})

	window.OnWindowEvent(events.Common.WindowClosing, func(e *application.WindowEvent) {
		window = nil
	})

	window.Show()
	window.Focus()
}

func setWindowHost(host string) {
	if host == "" {
		windowURL = ""
		return
	}
	windowURL = fmt.Sprintf("http://%s/ui", host)
}

func createTrafficWindow(app *application.App) {
	monitor := trafficMonitor.Load()
	if monitor == nil || monitor.DashboardURL() == "" {
		return
	}
	if trafficWindow != nil {
		trafficWindow.Show()
		trafficWindow.Focus()
		return
	}
	trafficWindow = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "Mimi 历史流量分析",
		Width:  panelWindowWidth,
		Height: panelWindowHeight,
		URL:    monitor.DashboardURL(),
	})
	trafficWindow.OnWindowEvent(events.Common.WindowClosing, func(e *application.WindowEvent) {
		trafficWindow = nil
	})
	trafficWindow.Show()
	trafficWindow.Focus()
}
