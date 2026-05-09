package main

import (
	"context"
	"fmt"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App 保存应用运行时上下文。
type App struct {
	ctx context.Context
}

// NewApp 创建一个新的应用实例。
func NewApp() *App {
	return &App{}
}

// startup 在应用启动时调用，保存上下文并启动系统托盘。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	startSystemTray(a)
}

// shutdown 在应用退出前调用，用于清理系统托盘图标。
func (a *App) shutdown(ctx context.Context) {
	stopSystemTray()
}

// ShowWindow 从系统托盘恢复并显示主窗口。
func (a *App) ShowWindow() {
	if a.ctx == nil {
		return
	}
	wailsRuntime.WindowShow(a.ctx)
	wailsRuntime.WindowUnminimise(a.ctx)
}

// QuitApplication 清理系统托盘并真正退出应用。
func (a *App) QuitApplication() {
	stopSystemTray()
	if a.ctx == nil {
		return
	}
	wailsRuntime.Quit(a.ctx)
}

// Greet 返回指定名称的问候语。
func (a *App) Greet(name string) string {
	return fmt.Sprintf("Hello %s, It's show time!", name)
}
