//go:build windows

package main

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	gwHwndNext                     = 2
	processQueryLimitedInformation = 0x1000
)

var (
	browserUser32                         = windows.NewLazySystemDLL("user32.dll")
	browserKernel32                       = windows.NewLazySystemDLL("kernel32.dll")
	procBrowserGetForegroundWindow        = browserUser32.NewProc("GetForegroundWindow")
	procBrowserGetWindowThreadProcessID   = browserUser32.NewProc("GetWindowThreadProcessId")
	procBrowserGetTopWindow               = browserUser32.NewProc("GetTopWindow")
	procBrowserGetWindow                  = browserUser32.NewProc("GetWindow")
	procBrowserIsWindowVisible            = browserUser32.NewProc("IsWindowVisible")
	procBrowserGetWindowTextLengthW       = browserUser32.NewProc("GetWindowTextLengthW")
	procBrowserQueryFullProcessImageNameW = browserKernel32.NewProc("QueryFullProcessImageNameW")
)

// openURLInActiveBrowser 优先使用当前或最近活动的浏览器打开链接，失败时回退系统默认浏览器。
func openURLInActiveBrowser(ctx context.Context, targetURL string) {
	if err := startActiveBrowser(targetURL); err != nil {
		appLogger.Warn("使用当前活动浏览器打开链接失败，回退到系统默认浏览器", "error", err)
		wailsRuntime.BrowserOpenURL(ctx, targetURL)
		return
	}
	appLogger.Info("已使用当前活动浏览器打开链接")
}

// startActiveBrowser 定位当前或最近活动的浏览器进程，并用它打开目标链接。
func startActiveBrowser(targetURL string) error {
	browserPath, err := findActiveBrowserExecutable()
	if err != nil {
		return err
	}
	return exec.Command(browserPath, targetURL).Start()
}

// findActiveBrowserExecutable 查找前台窗口或窗口层级中最近的浏览器可执行文件。
func findActiveBrowserExecutable() (string, error) {
	if hwnd := getForegroundWindow(); hwnd != 0 {
		if path, ok := browserExecutableFromWindow(hwnd); ok {
			return path, nil
		}
	}

	for hwnd, checked := getTopWindow(), 0; hwnd != 0 && checked < 200; hwnd, checked = getNextWindow(hwnd), checked+1 {
		if !isVisibleWindow(hwnd) || getWindowTextLength(hwnd) == 0 {
			continue
		}
		if path, ok := browserExecutableFromWindow(hwnd); ok {
			return path, nil
		}
	}

	return "", errors.New("未找到当前活动的浏览器窗口")
}

// browserExecutableFromWindow 根据窗口句柄解析所属进程，并判断是否为支持的浏览器。
func browserExecutableFromWindow(hwnd uintptr) (string, bool) {
	processPath, err := processPathFromWindow(hwnd)
	if err != nil {
		return "", false
	}
	if !isBrowserExecutable(processPath) {
		return "", false
	}
	return processPath, true
}

// processPathFromWindow 通过窗口句柄获取所属进程的完整可执行文件路径。
func processPathFromWindow(hwnd uintptr) (string, error) {
	var processID uint32
	procBrowserGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&processID)))
	if processID == 0 {
		return "", errors.New("窗口没有关联进程")
	}

	handle, err := windows.OpenProcess(processQueryLimitedInformation, false, processID)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)

	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	ret, _, callErr := procBrowserQueryFullProcessImageNameW.Call(
		uintptr(handle),
		0,
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if ret == 0 {
		return "", callErr
	}
	return windows.UTF16ToString(buffer[:size]), nil
}

// isBrowserExecutable 判断进程可执行文件是否为常见浏览器。
func isBrowserExecutable(processPath string) bool {
	switch strings.ToLower(filepath.Base(processPath)) {
	case "msedge.exe", "chrome.exe", "brave.exe", "firefox.exe", "opera.exe",
		"opera_gx.exe", "vivaldi.exe", "chromium.exe", "arc.exe", "iexplore.exe":
		return true
	default:
		return false
	}
}

// getForegroundWindow 获取当前前台窗口句柄。
func getForegroundWindow() uintptr {
	hwnd, _, _ := procBrowserGetForegroundWindow.Call()
	return hwnd
}

// getTopWindow 获取桌面窗口层级中的第一个顶级窗口。
func getTopWindow() uintptr {
	hwnd, _, _ := procBrowserGetTopWindow.Call(0)
	return hwnd
}

// getNextWindow 获取窗口层级中的下一个窗口。
func getNextWindow(hwnd uintptr) uintptr {
	next, _, _ := procBrowserGetWindow.Call(hwnd, gwHwndNext)
	return next
}

// isVisibleWindow 判断窗口是否为可见窗口。
func isVisibleWindow(hwnd uintptr) bool {
	visible, _, _ := procBrowserIsWindowVisible.Call(hwnd)
	return visible != 0
}

// getWindowTextLength 获取窗口标题长度，用于过滤无标题的后台窗口。
func getWindowTextLength(hwnd uintptr) int {
	length, _, _ := procBrowserGetWindowTextLengthW.Call(hwnd)
	return int(length)
}
