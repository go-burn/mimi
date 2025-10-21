package main

import (
	_ "embed"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var systemTray *application.SystemTray

//go:embed build/appicon.png
var Icon []byte

//go:embed build/darwin/icons.icns
var macTemplateIcon []byte

//go:embed build/icon-dark.png
var iconDark []byte

//go:embed build/icon-light.png
var iconLight []byte

func newTray() *application.SystemTray {
	if systemTray == nil {
		systemTray = app.SystemTray.New()
		if runtime.GOOS == "windows" {
			systemTray.OnClick(func() {
				createWindow(app)
			})
		} else {
			systemTray.OnRightClick(func() {
				createWindow(app)
			})
		}

		if runtime.GOOS == "darwin" {
			systemTray.SetTemplateIcon(macTemplateIcon)
		} else {
			systemTray.SetDarkModeIcon(iconDark)
			systemTray.SetIcon(Icon)
		}

		systemTray.SetMenu(menu)

		var open = func() {
			refreshMenu()
			systemTray.OpenMenu()
		}
		if runtime.GOOS == "windows" {
			systemTray.OnRightClick(open)
		} else {
			systemTray.OnClick(open)
		}

	}

	return systemTray
}
