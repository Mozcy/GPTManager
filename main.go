package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if err := initAppLogger(); err != nil {
		appLogger.Error("初始化日志失败", "error", err)
	}
	appLogger.Info("应用启动")

	// Create an instance of the app structure
	app := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:             "GPTManager",
		Width:             1560,
		Height:            850,
		HideWindowOnClose: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 31, G: 47, B: 63, A: 1},
		Windows: &windows.Options{
			Theme: windows.Dark,
			CustomTheme: &windows.ThemeSettings{
				DarkModeTitleBar:           windows.RGB(31, 47, 63),
				DarkModeTitleBarInactive:   windows.RGB(31, 47, 63),
				DarkModeTitleText:          windows.RGB(255, 255, 255),
				DarkModeTitleTextInactive:  windows.RGB(182, 195, 209),
				DarkModeBorder:             windows.RGB(50, 71, 91),
				DarkModeBorderInactive:     windows.RGB(50, 71, 91),
				LightModeTitleBar:          windows.RGB(31, 47, 63),
				LightModeTitleBarInactive:  windows.RGB(31, 47, 63),
				LightModeTitleText:         windows.RGB(255, 255, 255),
				LightModeTitleTextInactive: windows.RGB(182, 195, 209),
				LightModeBorder:            windows.RGB(50, 71, 91),
				LightModeBorderInactive:    windows.RGB(50, 71, 91),
			},
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		appLogger.Error("应用运行失败", "error", err)
	}
}
