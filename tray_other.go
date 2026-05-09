//go:build !windows

package main

// startSystemTray 在非 Windows 平台不执行任何操作。
func startSystemTray(app *App) {}

// stopSystemTray 在非 Windows 平台不执行任何操作。
func stopSystemTray() {}
