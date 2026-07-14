package main

import "github.com/wailsapp/wails/v3/pkg/application"

func addTrafficMenu(parent *application.Menu) {
	trafficMenuItem := parent.Add("历史流量")
	monitor := trafficMonitor.Load()
	if monitor == nil || monitor.DashboardURL() == "" {
		trafficMenuItem.SetEnabled(false)
		return
	}
	trafficMenuItem.OnClick(func(_ *application.Context) {
		createTrafficWindow(app)
	})
}
