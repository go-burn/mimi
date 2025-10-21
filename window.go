package main

import (
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

var window *application.WebviewWindow
var windowURL = ""

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
		Width:  1000,
		Height: 750,
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
