package main

import (
	"context"
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"qccg/logger"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "QCCG",
		Width:     900,
		Height:    650,
		MinWidth:  720,
		MinHeight: 500,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour:  &options.RGBA{R: 246, G: 245, B: 242, A: 1},
		Menu:              app.buildAppMenu(),
		HideWindowOnClose: true,
		OnStartup: func(ctx context.Context) {
			app.startup(ctx)
			// startup 拿到 ctx 后再刷一次菜单，确保账号 / Bridge 状态准确
			app.refreshAppMenu()
		},
		OnShutdown: func(ctx context.Context) {
			logger.Info("Application shutting down")
			logger.Close()
		},
		Bind: []interface{}{
			app,
		},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			Appearance:           mac.NSAppearanceNameVibrantLight,
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
