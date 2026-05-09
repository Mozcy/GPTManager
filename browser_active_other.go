//go:build !windows

package main

import (
	"context"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// openURLInActiveBrowser 在非 Windows 系统上使用系统默认浏览器打开链接。
func openURLInActiveBrowser(ctx context.Context, targetURL string) {
	appLogger.Info("当前系统未实现活动浏览器检测，使用系统默认浏览器打开链接")
	wailsRuntime.BrowserOpenURL(ctx, targetURL)
}
