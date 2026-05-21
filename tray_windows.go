//go:build windows

package main

import (
	"os"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	trayWindowClass = "GPTManagerTrayWindow"
	trayTitle       = "GPTManager"

	wmClose         = 0x0010
	wmDestroy       = 0x0002
	wmNull          = 0x0000
	wmUser          = 0x0400
	wmTrayIcon      = wmUser + 1
	wmRButtonUp     = 0x0205
	wmLButtonUp     = 0x0202
	wmLButtonDblClk = 0x0203

	nimAdd    = 0x00000000
	nimDelete = 0x00000002

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	idiApplication = 32512

	mfString = 0x00000000

	tpmRightButton = 0x0002
	tpmNonotify    = 0x0080
	tpmReturnCmd   = 0x0100

	menuShowID = 1001
	menuQuitID = 1002
)

type notifyIconData struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         windows.GUID
	HBalloonIcon     uintptr
}

type wndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type point struct {
	X int32
	Y int32
}

type msg struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type windowsTray struct {
	app      *App
	hwnd     uintptr
	hicon    uintptr
	ownsIcon bool
	added    bool

	mu       sync.Mutex
	stopOnce sync.Once
}

var (
	user32  = windows.NewLazySystemDLL("user32.dll")
	shell32 = windows.NewLazySystemDLL("shell32.dll")
	kernel  = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
	procLoadIconW        = user32.NewProc("LoadIconW")
	procDestroyIcon      = user32.NewProc("DestroyIcon")
	procCreatePopupMenu  = user32.NewProc("CreatePopupMenu")
	procAppendMenuW      = user32.NewProc("AppendMenuW")
	procTrackPopupMenu   = user32.NewProc("TrackPopupMenu")
	procDestroyMenu      = user32.NewProc("DestroyMenu")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
	procSetForegroundWnd = user32.NewProc("SetForegroundWindow")
	procPostMessageW     = user32.NewProc("PostMessageW")

	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
	procExtractIconW     = shell32.NewProc("ExtractIconW")

	procGetModuleHandleW = kernel.NewProc("GetModuleHandleW")

	trayWndProcCallback = windows.NewCallback(trayWindowProc)

	systemTrayMu sync.Mutex
	systemTray   *windowsTray
)

// startSystemTray 启动 Windows 系统托盘图标和托盘消息循环。
func startSystemTray(app *App) {
	systemTrayMu.Lock()
	if systemTray != nil {
		systemTrayMu.Unlock()
		appLogger.Info("系统托盘已运行，跳过启动")
		return
	}

	tray := &windowsTray{app: app}
	systemTray = tray
	systemTrayMu.Unlock()

	appLogger.Info("启动系统托盘")
	go tray.run()
}

// stopSystemTray 停止系统托盘并移除托盘图标。
func stopSystemTray() {
	tray := currentSystemTray()
	if tray != nil {
		appLogger.Info("停止系统托盘")
		tray.stop()
		return
	}
	appLogger.Info("系统托盘未运行，跳过停止")
}

// currentSystemTray 返回当前正在运行的系统托盘实例。
func currentSystemTray() *windowsTray {
	systemTrayMu.Lock()
	defer systemTrayMu.Unlock()
	return systemTray
}

// clearSystemTray 在托盘消息循环退出后清空全局托盘实例。
func clearSystemTray(tray *windowsTray) {
	systemTrayMu.Lock()
	if systemTray == tray {
		systemTray = nil
	}
	systemTrayMu.Unlock()
}

// run 在独立线程中创建隐藏窗口、添加托盘图标并处理托盘消息。
func (t *windowsTray) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer clearSystemTray(t)

	hwnd, ok := t.createWindow()
	if !ok {
		appLogger.Error("创建系统托盘隐藏窗口失败")
		return
	}

	t.mu.Lock()
	t.hwnd = hwnd
	t.mu.Unlock()

	t.hicon, t.ownsIcon = loadTrayIcon()
	if t.hicon == 0 {
		appLogger.Error("加载系统托盘图标失败")
		return
	}

	if !t.addIcon() {
		appLogger.Error("添加系统托盘图标失败")
		t.cleanup()
		return
	}
	appLogger.Info("系统托盘图标已添加")
	defer t.cleanup()

	var message msg
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(ret) <= 0 {
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&message)))
	}
}

// createWindow 创建用于接收托盘消息的隐藏 Win32 窗口。
func (t *windowsTray) createWindow() (uintptr, bool) {
	hinstance, _, _ := procGetModuleHandleW.Call(0)
	className := windows.StringToUTF16(trayWindowClass)

	wc := wndClassEx{
		CbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		LpfnWndProc:   trayWndProcCallback,
		HInstance:     hinstance,
		LpszClassName: &className[0],
	}

	atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 && err != windows.ERROR_CLASS_ALREADY_EXISTS {
		appLogger.Error("注册系统托盘窗口类失败", "error", err)
		return 0, false
	}

	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(&className[0])),
		uintptr(unsafe.Pointer(&className[0])),
		0,
		0,
		0,
		0,
		0,
		0,
		0,
		hinstance,
		0,
	)

	if hwnd == 0 {
		appLogger.Error("创建系统托盘隐藏窗口失败")
		return 0, false
	}
	appLogger.Info("系统托盘隐藏窗口已创建")
	return hwnd, true
}

// loadTrayIcon 优先从当前可执行文件提取图标，失败时使用系统默认图标。
func loadTrayIcon() (uintptr, bool) {
	if exe, err := os.Executable(); err == nil {
		exePath := windows.StringToUTF16(exe)
		icon, _, _ := procExtractIconW.Call(0, uintptr(unsafe.Pointer(&exePath[0])), 0)
		if icon > 1 {
			appLogger.Info("从可执行文件加载托盘图标", "path", exe)
			return icon, true
		}
	}

	icon, _, _ := procLoadIconW.Call(0, idiApplication)
	appLogger.Info("使用系统默认托盘图标")
	return icon, false
}

// addIcon 将应用图标添加到 Windows 系统托盘。
func (t *windowsTray) addIcon() bool {
	var data notifyIconData
	data.CbSize = uint32(unsafe.Sizeof(data))
	data.HWnd = t.hwnd
	data.UID = 1
	data.UFlags = nifMessage | nifIcon | nifTip
	data.UCallbackMessage = wmTrayIcon
	data.HIcon = t.hicon
	copy(data.SzTip[:], windows.StringToUTF16(trayTitle))

	ret, _, _ := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&data)))
	t.added = ret != 0
	if !t.added {
		appLogger.Error("调用 Shell_NotifyIcon 添加托盘图标失败")
	}
	return t.added
}

// stop 移除托盘图标并通知隐藏窗口退出消息循环。
func (t *windowsTray) stop() {
	t.stopOnce.Do(func() {
		t.mu.Lock()
		hwnd := t.hwnd
		t.mu.Unlock()
		t.cleanup()
		if hwnd != 0 {
			procPostMessageW.Call(hwnd, wmClose, 0, 0)
		}
		appLogger.Info("系统托盘停止消息已发送")
	})
}

// cleanup 删除托盘图标并释放当前实例持有的图标资源。
func (t *windowsTray) cleanup() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.added {
		var data notifyIconData
		data.CbSize = uint32(unsafe.Sizeof(data))
		data.HWnd = t.hwnd
		data.UID = 1
		procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&data)))
		t.added = false
		appLogger.Info("系统托盘图标已删除")
	}

	if t.ownsIcon && t.hicon != 0 {
		procDestroyIcon.Call(t.hicon)
		t.hicon = 0
		appLogger.Info("系统托盘图标资源已释放")
	}
}

// trayWindowProc 处理隐藏窗口收到的托盘和窗口消息。
func trayWindowProc(hwnd uintptr, message uint32, wparam uintptr, lparam uintptr) uintptr {
	tray := currentSystemTray()

	switch message {
	case wmTrayIcon:
		if tray != nil {
			tray.handleTrayEvent(uint32(lparam))
		}
		return 0
	case wmClose:
		if tray != nil {
			tray.cleanup()
		}
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}

	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wparam, lparam)
	return ret
}

// handleTrayEvent 根据托盘鼠标事件显示菜单或恢复主窗口。
func (t *windowsTray) handleTrayEvent(event uint32) {
	switch event {
	case wmRButtonUp:
		appLogger.Info("打开系统托盘右键菜单")
		t.showContextMenu()
	case wmLButtonUp, wmLButtonDblClk:
		appLogger.Info("点击系统托盘图标，显示窗口")
		go t.app.ShowWindow()
	}
}

// showContextMenu 在托盘图标位置弹出“显示”和“关闭”菜单。
func (t *windowsTray) showContextMenu() {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)

	showText := windows.StringToUTF16("显示")
	quitText := windows.StringToUTF16("关闭")
	procAppendMenuW.Call(menu, mfString, menuShowID, uintptr(unsafe.Pointer(&showText[0])))
	procAppendMenuW.Call(menu, mfString, menuQuitID, uintptr(unsafe.Pointer(&quitText[0])))

	var cursor point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor)))
	procSetForegroundWnd.Call(t.hwnd)

	cmd, _, _ := procTrackPopupMenu.Call(
		menu,
		tpmRightButton|tpmNonotify|tpmReturnCmd,
		uintptr(cursor.X),
		uintptr(cursor.Y),
		0,
		t.hwnd,
		0,
	)
	procPostMessageW.Call(t.hwnd, wmNull, 0, 0)

	switch cmd {
	case menuShowID:
		appLogger.Info("系统托盘菜单选择显示")
		go t.app.ShowWindow()
	case menuQuitID:
		appLogger.Info("系统托盘菜单选择关闭")
		go t.app.QuitApplication()
	}
}
