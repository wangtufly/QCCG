package main

import (
	stdruntime "runtime"

	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"qoder2api/account"
	"qoder2api/logger"
)

// buildAppMenu 构造 wails 应用菜单（macOS 顶部菜单栏 / Windows&Linux 窗口菜单）。
// 注：macOS 没有真"系统托盘"，因 energye/systray 与 wails v2 的 NSApp delegate
// 冲突无法稳定共存（详见提交记录），这里使用 wails 自带菜单作为操作入口。
func (a *App) buildAppMenu() *menu.Menu {
	m := menu.NewMenu()
	if stdruntime.GOOS == "darwin" {
		m.Append(menu.AppMenu())
	}

	// Qoder2API 主菜单（macOS 上会作为应用菜单出现）
	app := m.AddSubmenu("Qoder2API")
	app.AddText("显示主界面", keys.CmdOrCtrl("0"), func(_ *menu.CallbackData) {
		a.showWindow()
	})
	app.AddSeparator()

	// 启动 / 停止 Bridge —— 用文本切换实现，避免 checkbox 在不同平台行为差异
	status := a.GetStatus()
	if status.Running {
		app.AddText("停止 Bridge", keys.CmdOrCtrl("k"), func(_ *menu.CallbackData) {
			if err := a.StopBridge(); err != nil {
				logger.Error("Menu: stop bridge failed: %v", err)
			}
			a.refreshAppMenu()
		})
	} else {
		app.AddText("启动 Bridge", keys.CmdOrCtrl("k"), func(_ *menu.CallbackData) {
			if err := a.StartBridge(); err != nil {
				logger.Error("Menu: start bridge failed: %v", err)
			}
			a.refreshAppMenu()
		})
	}

	app.AddSeparator()
	app.AddText("退出", keys.CmdOrCtrl("q"), func(_ *menu.CallbackData) {
		a.QuitApp()
	})

	// 账号子菜单
	accountsMenu := m.AddSubmenu("账号")
	accounts, _ := account.List()
	if len(accounts) == 0 {
		empty := accountsMenu.AddText("暂无账号", nil, nil)
		empty.Disabled = true
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
			accountsMenu.AddCheckbox(label, acct.Active, nil, func(_ *menu.CallbackData) {
				if err := a.SetActiveAccount(acct.ID); err != nil {
					logger.Error("Menu: switch account failed: %v", err)
					return
				}
				a.refreshAppMenu()
			})
		}
	}

	if stdruntime.GOOS == "darwin" {
		m.Append(menu.EditMenu())
	}

	return m
}

// refreshAppMenu 重新计算并应用菜单；ctx 未就绪时忽略。
func (a *App) refreshAppMenu() {
	if a.ctx == nil {
		return
	}
	wruntime.MenuSetApplicationMenu(a.ctx, a.buildAppMenu())
	wruntime.MenuUpdateApplicationMenu(a.ctx)
}

// showWindow 唤起主窗口；ctx 尚未就绪时静默忽略。
func (a *App) showWindow() {
	if a.ctx == nil {
		return
	}
	wruntime.Show(a.ctx)
	wruntime.WindowShow(a.ctx)
	wruntime.WindowUnminimise(a.ctx)
}
