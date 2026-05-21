package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

// ProxyManager 保存代理配置和账号管理需要的运行时状态。
type ProxyManager struct {
	upstreamMu     sync.RWMutex
	upstreamConfig UpstreamConfig
	activeMu       sync.RWMutex
	activeAccount  *activeProxyAccount
}

type activeProxyAccount struct {
	ID          int64
	AccountID   string
	Email       string
	AccessToken string
}

// NewProxyManager 创建代理服务管理器。
func NewProxyManager(store *ProxyStore) *ProxyManager {
	appLogger.Info("创建代理服务管理器")
	upstreamConfig, err := store.GetUpstreamConfig()
	if err != nil {
		appLogger.Error("加载代理缓存失败，使用默认值", "error", err)
		upstreamConfig = defaultUpstreamConfig()
	}
	appLogger.Info("代理配置已加载到内存", "type", upstreamConfig.Type, "address", upstreamConfig.IP+":"+upstreamConfig.Port)
	manager := &ProxyManager{
		upstreamConfig: upstreamConfig,
	}
	if record, ok, err := store.GetActiveAccountRecord(); err != nil {
		appLogger.Error("加载激活账号缓存失败", "error", err)
	} else if ok {
		manager.SetActiveAccount(record)
		appLogger.Info("激活账号已从数据库加载到内存", "id", record.ID, "account_id", record.AccountID, "email", record.Email)
	} else {
		appLogger.Info("未找到已保存的激活账号")
	}
	return manager
}

// Close 清理运行时状态。
func (m *ProxyManager) Close() {
	if m == nil {
		return
	}
	m.activeMu.Lock()
	m.activeAccount = nil
	m.activeMu.Unlock()
	appLogger.Info("代理运行时状态已清理")
}

// GetUpstreamConfig 返回当前内存中的代理配置快照。
func (m *ProxyManager) GetUpstreamConfig() UpstreamConfig {
	m.upstreamMu.RLock()
	defer m.upstreamMu.RUnlock()
	return m.upstreamConfig
}

// SetUpstreamConfig 更新内存中的代理配置。
func (m *ProxyManager) SetUpstreamConfig(config UpstreamConfig) {
	m.upstreamMu.Lock()
	m.upstreamConfig = config
	m.upstreamMu.Unlock()
	appLogger.Info("代理内存缓存已更新", "type", config.Type, "address", config.IP+":"+config.Port)
}

// SetActiveAccount 设置当前激活账号。
func (m *ProxyManager) SetActiveAccount(record accountRecord) {
	m.activeMu.Lock()
	m.activeAccount = &activeProxyAccount{
		ID:          record.ID,
		AccountID:   record.AccountID,
		Email:       record.Email,
		AccessToken: record.AccessToken,
	}
	m.activeMu.Unlock()
	appLogger.Info("激活账号缓存已更新", "id", record.ID, "account_id", record.AccountID, "email", record.Email)
}

// ClearActiveAccount 清除指定账号的激活状态，避免删除后继续使用旧 token。
func (m *ProxyManager) ClearActiveAccount(id int64) {
	m.activeMu.Lock()
	if m.activeAccount != nil && m.activeAccount.ID == id {
		m.activeAccount = nil
		appLogger.Info("激活账号缓存已清除", "id", id)
	}
	m.activeMu.Unlock()
}

// newUpstreamTransport 根据全局代理配置创建 HTTP 转发器。
func newUpstreamTransport(config UpstreamConfig) (*http.Transport, error) {
	transport := &http.Transport{
		Proxy:                 nil,
		ResponseHeaderTimeout: 60 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	}

	upstreamAddr := net.JoinHostPort(config.IP, config.Port)
	switch config.Type {
	case "http":
		upstreamURL := &url.URL{Scheme: "http", Host: upstreamAddr}
		transport.Proxy = http.ProxyURL(upstreamURL)
	case "socks5":
		dialer, err := proxy.SOCKS5("tcp", upstreamAddr, nil, proxy.Direct)
		if err != nil {
			appLogger.Error("创建 SOCKS5 代理失败", "error", err, "address", upstreamAddr)
			return nil, fmt.Errorf("创建 SOCKS5 代理失败: %w", err)
		}
		transport.DialContext = func(ctx context.Context, network string, address string) (net.Conn, error) {
			type result struct {
				conn net.Conn
				err  error
			}
			ch := make(chan result, 1)
			go func() {
				conn, err := dialer.Dial(network, address)
				ch <- result{conn: conn, err: err}
			}()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case result := <-ch:
				return result.conn, result.err
			}
		}
	default:
		return nil, errors.New("代理协议仅支持 http 或 socks5")
	}

	return transport, nil
}

// CheckUpstreamStatus 检查全局代理是否可以建立本地 TCP 连接。
func CheckUpstreamStatus(config UpstreamConfig) UpstreamStatus {
	config, err := normalizeUpstreamConfig(config)
	if err != nil {
		return UpstreamStatus{Connected: false, Message: err.Error()}
	}

	address := net.JoinHostPort(config.IP, config.Port)
	appLogger.Info("检查代理 TCP 连通性", "type", config.Type, "address", address)
	conn, err := net.DialTimeout("tcp", address, 3*time.Second)
	if err != nil {
		appLogger.Warn("代理 TCP 连通性检查失败", "error", err, "type", config.Type, "address", address)
		return UpstreamStatus{Connected: false, Message: err.Error()}
	}
	_ = conn.Close()
	appLogger.Info("代理 TCP 连通性检查成功", "type", config.Type, "address", address)
	return UpstreamStatus{Connected: true, Message: "已连接"}
}
