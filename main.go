package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"

	"qccg/logger"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed baseprompt.json
var basePromptRaw []byte

func main() {
	appService := NewApp(nil, nil)

	app := application.New(application.Options{
		Name: "QCCG",
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false, // 关窗不退出，留在托盘
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Services: []application.Service{
			application.NewService(appService),
		},
		OnShutdown: func() {
			logger.Info("Application shutting down")
			logger.Close()
		},
	})

	mainWindow := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "QCCG",
		Width:            900,
		Height:           650,
		MinWidth:         720,
		MinHeight:        500,
		Hidden:           false, // 启动时显示主窗口；用户关闭窗口后由托盘控制再次显示
		BackgroundColour: application.NewRGBA(246, 245, 242, 1),
		Mac: application.MacWindow{
			TitleBar:   application.MacTitleBarHiddenInset,
			Appearance: application.NSAppearanceNameVibrantLight,
			Backdrop:   application.MacBackdropTranslucent,
		},
	})

	// 绑定 app 和 window 引用
	appService.app = app
	appService.window = mainWindow

	setupTray(app, mainWindow, appService)

	// 启动后把主窗口抢到最前，避免被其他应用遮挡
	mainWindow.Focus()

	err := app.Run()
	if err != nil {
		log.Fatal(err)
	}
}
