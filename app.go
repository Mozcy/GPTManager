package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App 保存应用运行时上下文。
type App struct {
	ctx          context.Context
	proxyStore   *ProxyStore
	proxyManager *ProxyManager
	proxyInitErr error
	authMu       sync.Mutex
	authRunning  bool
	usageCancel  context.CancelFunc
	usageWG      sync.WaitGroup
	usageMu      sync.Mutex
	usageRunning bool
}

// NewApp 创建一个新的应用实例。
func NewApp() *App {
	return &App{}
}

// startup 在应用启动时调用，保存上下文并启动系统托盘。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	appLogger.Info("Wails 启动回调开始")
	if err := a.initProxyService(); err != nil {
		a.proxyInitErr = err
		appLogger.Error("初始化代理服务失败", "error", err)
	} else {
		a.startAccountUsageRefresher()
	}
	startSystemTray(a)
	appLogger.Info("Wails 启动回调完成")
}

// shutdown 在应用退出前调用，用于清理系统托盘图标。
func (a *App) shutdown(ctx context.Context) {
	appLogger.Info("应用关闭清理开始")
	a.stopAccountUsageRefresher()
	if a.proxyManager != nil {
		a.proxyManager.Close()
	}
	if a.proxyStore != nil {
		if err := a.proxyStore.Close(); err != nil {
			appLogger.Error("关闭数据库失败", "error", err)
		}
	}
	stopSystemTray()
	appLogger.Info("应用关闭清理完成")
}

// ShowWindow 从系统托盘恢复并显示主窗口。
func (a *App) ShowWindow() {
	if a.ctx == nil {
		appLogger.Warn("显示窗口失败: Wails 上下文为空")
		return
	}
	appLogger.Info("从托盘恢复窗口")
	wailsRuntime.WindowShow(a.ctx)
	wailsRuntime.WindowUnminimise(a.ctx)
}

// QuitApplication 清理系统托盘并真正退出应用。
func (a *App) QuitApplication() {
	appLogger.Info("用户请求退出应用")
	a.stopAccountUsageRefresher()
	if a.proxyManager != nil {
		a.proxyManager.Close()
	}
	stopSystemTray()
	if a.ctx == nil {
		return
	}
	wailsRuntime.Quit(a.ctx)
}

// ListProxies 返回全部代理配置。
func (a *App) ListProxies() ([]ProxyConfig, error) {
	if err := a.ensureProxyService(); err != nil {
		appLogger.Error("查询代理列表失败: 服务未初始化", "error", err)
		return nil, err
	}
	return a.proxyStore.ListProxies()
}

// CreateProxy 创建一条新的本地代理配置。
func (a *App) CreateProxy(input ProxyConfig) (ProxyConfig, error) {
	if err := a.ensureProxyService(); err != nil {
		appLogger.Error("创建代理失败: 服务未初始化", "error", err)
		return ProxyConfig{}, err
	}
	item, err := a.proxyStore.CreateProxy(input)
	if err != nil {
		appLogger.Error("创建代理失败", "error", err, "ip", input.IP, "port", input.Port)
		return ProxyConfig{}, err
	}
	appLogger.Info("创建代理成功", "id", item.ID, "listen", item.IP+":"+item.Port)
	return item, nil
}

// UpdateProxy 更新一条未启用的本地代理配置。
func (a *App) UpdateProxy(input ProxyConfig) (ProxyConfig, error) {
	if err := a.ensureProxyService(); err != nil {
		appLogger.Error("更新代理失败: 服务未初始化", "error", err)
		return ProxyConfig{}, err
	}
	item, err := a.proxyStore.UpdateProxy(input)
	if err != nil {
		appLogger.Error("更新代理失败", "error", err, "id", input.ID)
		return ProxyConfig{}, err
	}
	appLogger.Info("更新代理成功", "id", item.ID, "listen", item.IP+":"+item.Port)
	return item, nil
}

// DeleteProxy 删除一条未启用的本地代理配置。
func (a *App) DeleteProxy(id int64) error {
	if err := a.ensureProxyService(); err != nil {
		appLogger.Error("删除代理失败: 服务未初始化", "error", err, "id", id)
		return err
	}
	if err := a.proxyStore.DeleteProxy(id); err != nil {
		appLogger.Error("删除代理失败", "error", err, "id", id)
		return err
	}
	appLogger.Info("删除代理成功", "id", id)
	return nil
}

// SetProxyEnabled 启动或停止指定代理。
func (a *App) SetProxyEnabled(id int64, enabled bool) (ProxyConfig, error) {
	if err := a.ensureProxyService(); err != nil {
		appLogger.Error("切换代理状态失败: 服务未初始化", "error", err, "id", id, "enabled", enabled)
		return ProxyConfig{}, err
	}
	item, err := a.proxyManager.SetProxyEnabled(id, enabled)
	if err != nil {
		appLogger.Error("切换代理状态失败", "error", err, "id", id, "enabled", enabled)
		return ProxyConfig{}, err
	}
	appLogger.Info("切换代理状态成功", "id", id, "enabled", enabled, "listen", item.IP+":"+item.Port)
	return item, nil
}

// GetUpstreamConfig 返回全局二次代理配置。
func (a *App) GetUpstreamConfig() (UpstreamConfig, error) {
	if err := a.ensureProxyService(); err != nil {
		appLogger.Error("获取二次代理配置失败: 服务未初始化", "error", err)
		return UpstreamConfig{}, err
	}
	return a.proxyManager.GetUpstreamConfig(), nil
}

// SaveUpstreamConfig 保存全局二次代理配置。
func (a *App) SaveUpstreamConfig(input UpstreamConfig) (UpstreamConfig, error) {
	if err := a.ensureProxyService(); err != nil {
		appLogger.Error("保存二次代理配置失败: 服务未初始化", "error", err)
		return UpstreamConfig{}, err
	}
	config, err := a.proxyStore.SaveUpstreamConfig(input)
	if err != nil {
		appLogger.Error("保存二次代理配置失败", "error", err, "type", input.Type, "ip", input.IP, "port", input.Port)
		return UpstreamConfig{}, err
	}
	a.proxyManager.SetUpstreamConfig(config)
	appLogger.Info("保存二次代理配置成功", "type", config.Type, "address", config.IP+":"+config.Port)
	return config, nil
}

// CheckUpstreamStatus 检查全局二次代理连接状态。
func (a *App) CheckUpstreamStatus() (UpstreamStatus, error) {
	if err := a.ensureProxyService(); err != nil {
		appLogger.Error("检查二次代理失败: 服务未初始化", "error", err)
		return UpstreamStatus{}, err
	}
	config := a.proxyManager.GetUpstreamConfig()
	status := CheckUpstreamStatus(config)
	level := slog.LevelInfo
	if !status.Connected {
		level = slog.LevelWarn
	}
	appLogger.Log(context.Background(), level, "二次代理状态检查完成",
		"type", config.Type,
		"address", config.IP+":"+config.Port,
		"connected", status.Connected,
		"message", status.Message,
	)
	return status, nil
}

// ListAccounts 返回已保存的账号列表。
func (a *App) ListAccounts() ([]AccountInfo, error) {
	if err := a.ensureProxyService(); err != nil {
		appLogger.Error("查询账号列表失败: 服务未初始化", "error", err)
		return nil, err
	}
	return a.proxyStore.ListAccounts()
}

// RefreshAccountUsage 手动刷新所有账号额度，并通过事件推送单账号结果。
func (a *App) RefreshAccountUsage() error {
	if err := a.ensureProxyService(); err != nil {
		appLogger.Error("手动刷新账号额度失败: 服务未初始化", "error", err)
		return err
	}
	appLogger.Info("手动刷新账号额度开始")
	a.refreshAllAccountUsage(context.Background())
	appLogger.Info("手动刷新账号额度完成")
	return nil
}

// DeleteAccount 删除已保存的账号。
func (a *App) DeleteAccount(id int64) error {
	if err := a.ensureProxyService(); err != nil {
		appLogger.Error("删除账号失败: 服务未初始化", "error", err, "id", id)
		return err
	}
	if err := a.proxyStore.DeleteAccount(id); err != nil {
		appLogger.Error("删除账号失败", "error", err, "id", id)
		return err
	}
	appLogger.Info("删除账号成功", "id", id)
	return nil
}

// Greet 返回指定名称的问候语。
func (a *App) Greet(name string) string {
	return fmt.Sprintf("Hello %s, It's show time!", name)
}

// initProxyService 初始化 SQLite 存储和代理服务管理器。
func (a *App) initProxyService() error {
	appLogger.Info("初始化代理服务开始")
	store, err := NewProxyStore()
	if err != nil {
		return err
	}

	manager := NewProxyManager(store)
	a.proxyStore = store
	a.proxyManager = manager
	a.proxyInitErr = nil
	if err := manager.StartEnabledProxies(); err != nil {
		appLogger.Warn("启动已保存代理失败", "error", err)
	}
	appLogger.Info("初始化代理服务完成")
	return nil
}

// ensureProxyService 确认代理服务已完成初始化。
func (a *App) ensureProxyService() error {
	if a.proxyStore != nil && a.proxyManager != nil {
		return nil
	}
	if a.proxyInitErr != nil {
		return a.proxyInitErr
	}
	return errors.New("代理服务未初始化")
}
