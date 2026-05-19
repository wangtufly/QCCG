package main

import (
	_ "embed"

	"github.com/wailsapp/wails/v3/pkg/application"

	"qccg/account"
	"qccg/logger"
)

//go:embed build/trayicon.png
var trayIcon []byte

func setupTray(app *application.App, window *application.WebviewWindow, a *App) {
	tray := app.SystemTray.New()
	tray.SetIcon(trayIcon)
	tray.SetLabel("")

	// 左键单击切换窗口
	tray.OnClick(func() {
		if window.IsVisible() {
			window.Hide()
		} else {
			window.Show()
			window.UnMinimise()
		}
	})

	// 设置 a.refreshTray 以便 app.go 中状态变化后重建菜单
	a.refreshTray = func() {
		tray.SetMenu(buildTrayMenu(app, window, a))
	}

	// 初始菜单
	tray.SetMenu(buildTrayMenu(app, window, a))
}

func buildTrayMenu(app *application.App, window *application.WebviewWindow, a *App) *application.Menu {
	m := app.NewMenu()

	// 显示主界面
	m.Add("显示主界面").SetAccelerator("CmdOrCtrl+0").OnClick(func(_ *application.Context) {
		window.Show()
		window.UnMinimise()
	})
	m.AddSeparator()

	// Bridge 状态动态构建
	status := a.GetStatus()
	if status.Running {
		m.Add("停止 Bridge").SetAccelerator("CmdOrCtrl+K").OnClick(func(_ *application.Context) {
			if err := a.StopBridge(); err != nil {
				logger.Error("Tray: stop bridge failed: %v", err)
			}
			a.refreshTrayMenu()
		})
	} else {
		m.Add("启动 Bridge").SetAccelerator("CmdOrCtrl+K").OnClick(func(_ *application.Context) {
			if err := a.StartBridge(); err != nil {
				logger.Error("Tray: start bridge failed: %v", err)
			}
			a.refreshTrayMenu()
		})
	}
	m.AddSeparator()

	// 账号子菜单
	accounts, _ := account.List()
	accountsMenu := m.AddSubmenu("账号")
	if len(accounts) == 0 {
		accountsMenu.Add("暂无账号").SetEnabled(false)
	} else {
		for _, acct := range accounts {
			acct := acct
			label := acct.Name
			if label == "" {
				label = acct.Email
			}
			if label == "" && len(acct.ID) >= 8 {
				label = acct.ID[:8]
			}
			accountsMenu.AddCheckbox(label, acct.Active).OnClick(func(_ *application.Context) {
				if err := a.SetActiveAccount(acct.ID); err != nil {
					logger.Error("Tray: switch account failed: %v", err)
					return
				}
				a.refreshTrayMenu()
			})
		}
	}
	m.AddSeparator()

	// 退出
	m.Add("退出").SetAccelerator("CmdOrCtrl+Q").OnClick(func(_ *application.Context) {
		app.Quit()
	})

	return m
}
