package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

// ProxyManager 管理所有已启用的本地代理服务。
type ProxyManager struct {
	store   *ProxyStore
	mu      sync.Mutex
	running map[int64]*runningProxy
}

type runningProxy struct {
	server   *http.Server
	listener net.Listener
}

// NewProxyManager 创建代理服务管理器。
func NewProxyManager(store *ProxyStore) *ProxyManager {
	appLogger.Info("创建代理服务管理器")
	return &ProxyManager{
		store:   store,
		running: make(map[int64]*runningProxy),
	}
}

// StartEnabledProxies 启动数据库中已启用的代理。
func (m *ProxyManager) StartEnabledProxies() error {
	appLogger.Info("开始恢复已启用代理")
	items, err := m.store.ListProxies()
	if err != nil {
		appLogger.Error("恢复已启用代理失败: 查询配置失败", "error", err)
		return err
	}

	var failures []string
	started := 0
	for _, item := range items {
		if !item.Enabled {
			continue
		}
		appLogger.Info("恢复已启用代理", "id", item.ID, "listen", item.IP+":"+item.Port)
		if err := m.StartProxy(item); err != nil {
			_, _ = m.store.SetProxyEnabled(item.ID, false)
			appLogger.Error("恢复代理失败，已自动停用", "error", err, "id", item.ID, "listen", item.IP+":"+item.Port)
			failures = append(failures, fmt.Sprintf("%s:%s %s", item.IP, item.Port, err.Error()))
			continue
		}
		started++
	}
	if len(failures) > 0 {
		return fmt.Errorf("部分代理启动失败，已自动停用: %s", strings.Join(failures, "; "))
	}
	appLogger.Info("已启用代理恢复完成", "started", started)
	return nil
}

// StartProxy 启动一条本地代理监听。
func (m *ProxyManager) StartProxy(item ProxyConfig) error {
	item, err := normalizeProxyConfig(item)
	if err != nil {
		appLogger.Error("启动代理失败: 配置校验失败", "error", err, "id", item.ID)
		return err
	}

	m.mu.Lock()
	if _, ok := m.running[item.ID]; ok {
		m.mu.Unlock()
		appLogger.Info("代理已在运行，跳过启动", "id", item.ID, "listen", item.IP+":"+item.Port)
		return nil
	}
	m.mu.Unlock()

	listenAddr := net.JoinHostPort(item.IP, item.Port)
	appLogger.Info("启动本地代理监听", "id", item.ID, "listen", listenAddr)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		appLogger.Error("启动本地代理监听失败", "error", err, "id", item.ID, "listen", listenAddr)
		return fmt.Errorf("启动本地代理 %s 失败: %w", listenAddr, err)
	}

	handler := newForwardProxyHandler(m.store)

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 15 * time.Second,
	}

	m.mu.Lock()
	m.running[item.ID] = &runningProxy{server: server, listener: listener}
	m.mu.Unlock()

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			appLogger.Error("代理服务异常退出", "error", err, "id", item.ID, "listen", listenAddr)
		}
	}()

	appLogger.Info("本地代理已启动", "id", item.ID, "listen", listenAddr)
	return nil
}

// StopProxy 停止一条本地代理监听。
func (m *ProxyManager) StopProxy(id int64) error {
	m.mu.Lock()
	running := m.running[id]
	delete(m.running, id)
	m.mu.Unlock()
	if running == nil {
		appLogger.Info("代理未运行，跳过停止", "id", id)
		return nil
	}

	appLogger.Info("停止本地代理", "id", id)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := running.server.Shutdown(ctx); err != nil {
		appLogger.Warn("代理优雅停止失败，强制关闭监听", "error", err, "id", id)
		running.listener.Close()
	}
	appLogger.Info("本地代理已停止", "id", id)
	return nil
}

// Close 停止所有正在运行的本地代理。
func (m *ProxyManager) Close() {
	if m == nil {
		return
	}

	appLogger.Info("停止所有本地代理开始")
	m.mu.Lock()
	ids := make([]int64, 0, len(m.running))
	for id := range m.running {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	for _, id := range ids {
		_ = m.StopProxy(id)
	}
	appLogger.Info("停止所有本地代理完成", "count", len(ids))
}

// SetProxyEnabled 启动或停止代理，并同步数据库状态。
func (m *ProxyManager) SetProxyEnabled(id int64, enabled bool) (ProxyConfig, error) {
	item, err := m.store.GetProxy(id)
	if err != nil {
		appLogger.Error("切换代理状态失败: 查询配置失败", "error", err, "id", id, "enabled", enabled)
		return ProxyConfig{}, err
	}

	if enabled {
		if err := m.StartProxy(item); err != nil {
			appLogger.Error("启用代理失败", "error", err, "id", id, "listen", item.IP+":"+item.Port)
			return ProxyConfig{}, err
		}
		return m.store.SetProxyEnabled(id, true)
	}

	if err := m.StopProxy(id); err != nil {
		appLogger.Error("停用代理失败", "error", err, "id", id)
		return ProxyConfig{}, err
	}
	return m.store.SetProxyEnabled(id, false)
}

type forwardProxyHandler struct {
	store *ProxyStore
}

// newForwardProxyHandler 创建转发到二次代理的 HTTP 代理处理器。
func newForwardProxyHandler(store *ProxyStore) *forwardProxyHandler {
	return &forwardProxyHandler{store: store}
}

// newUpstreamTransport 根据全局二次代理配置创建 HTTP 转发器。
func newUpstreamTransport(config UpstreamConfig) (*http.Transport, error) {
	transport := &http.Transport{
		Proxy:                 nil,
		ResponseHeaderTimeout: 60 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	}

	upstreamAddr := net.JoinHostPort(config.IP, config.Port)
	appLogger.Info("创建二次代理转发器", "type", config.Type, "address", upstreamAddr)
	switch config.Type {
	case "http":
		upstreamURL := &url.URL{Scheme: "http", Host: upstreamAddr}
		transport.Proxy = http.ProxyURL(upstreamURL)
	case "socks5":
		dialer, err := proxy.SOCKS5("tcp", upstreamAddr, nil, proxy.Direct)
		if err != nil {
			appLogger.Error("创建 SOCKS5 二次代理失败", "error", err, "address", upstreamAddr)
			return nil, fmt.Errorf("创建 SOCKS5 二次代理失败: %w", err)
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
		return nil, errors.New("二次代理协议仅支持 http 或 socks5")
	}

	return transport, nil
}

// ServeHTTP 处理 HTTP 代理请求。
func (h *forwardProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.EqualFold(r.Method, http.MethodConnect) {
		h.handleConnect(w, r)
		return
	}
	h.handleHTTP(w, r)
}

// handleHTTP 处理普通 HTTP 请求并通过二次代理转发。
func (h *forwardProxyHandler) handleHTTP(w http.ResponseWriter, r *http.Request) {
	config, err := h.store.GetUpstreamConfig()
	if err != nil {
		appLogger.Error("HTTP 转发失败: 读取二次代理配置失败", "error", err, "method", r.Method, "host", r.Host, "url", r.URL.String())
		http.Error(w, "读取二次代理配置失败: "+err.Error(), http.StatusBadGateway)
		return
	}
	transport, err := newUpstreamTransport(config)
	if err != nil {
		appLogger.Error("HTTP 转发失败: 创建二次代理转发器失败", "error", err, "method", r.Method, "host", r.Host, "url", r.URL.String())
		http.Error(w, "创建二次代理转发器失败: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer transport.CloseIdleConnections()

	outReq := r.Clone(r.Context())
	outReq.RequestURI = ""
	removeHopHeaders(outReq.Header)
	if outReq.URL.Scheme == "" {
		outReq.URL.Scheme = "http"
	}
	if outReq.URL.Host == "" {
		outReq.URL.Host = r.Host
	}

	resp, err := transport.RoundTrip(outReq)
	if err != nil {
		appLogger.Error("HTTP 代理转发失败", "error", err, "method", r.Method, "host", r.Host, "url", r.URL.String(), "upstream", config.Type+"://"+config.IP+":"+config.Port)
		http.Error(w, "代理转发失败: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	removeHopHeaders(resp.Header)
	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// handleConnect 处理 HTTPS CONNECT 隧道并通过二次代理建立出口连接。
func (h *forwardProxyHandler) handleConnect(w http.ResponseWriter, r *http.Request) {
	targetConn, err := h.dialConnectTarget(r.Host)
	if err != nil {
		appLogger.Error("CONNECT 建立失败", "error", err, "target", r.Host)
		http.Error(w, "CONNECT 建立失败: "+err.Error(), http.StatusBadGateway)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		targetConn.Close()
		appLogger.Error("CONNECT 建立失败: 当前连接不支持 Hijacker", "target", r.Host)
		http.Error(w, "当前连接不支持隧道", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		targetConn.Close()
		appLogger.Error("CONNECT 建立失败: 接管客户端连接失败", "error", err, "target", r.Host)
		return
	}

	_, _ = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	relayConnections(clientConn, targetConn)
}

// dialConnectTarget 通过二次代理连接 CONNECT 请求目标。
func (h *forwardProxyHandler) dialConnectTarget(target string) (net.Conn, error) {
	config, err := h.store.GetUpstreamConfig()
	if err != nil {
		appLogger.Error("CONNECT 读取二次代理配置失败", "error", err, "target", target)
		return nil, err
	}

	appLogger.Info("通过二次代理建立 CONNECT", "type", config.Type, "upstream", config.IP+":"+config.Port, "target", target)
	switch config.Type {
	case "http":
		return dialHTTPProxyTunnel(net.JoinHostPort(config.IP, config.Port), target)
	case "socks5":
		dialer, err := proxy.SOCKS5("tcp", net.JoinHostPort(config.IP, config.Port), nil, proxy.Direct)
		if err != nil {
			appLogger.Error("创建 SOCKS5 CONNECT 拨号器失败", "error", err, "upstream", config.IP+":"+config.Port, "target", target)
			return nil, err
		}
		return dialer.Dial("tcp", target)
	default:
		return nil, errors.New("二次代理协议仅支持 http 或 socks5")
	}
}

// CheckUpstreamStatus 检查全局二次代理是否可以建立本地 TCP 连接。
func CheckUpstreamStatus(config UpstreamConfig) UpstreamStatus {
	config, err := normalizeUpstreamConfig(config)
	if err != nil {
		return UpstreamStatus{Connected: false, Message: err.Error()}
	}

	address := net.JoinHostPort(config.IP, config.Port)
	appLogger.Info("检查二次代理 TCP 连通性", "type", config.Type, "address", address)
	conn, err := net.DialTimeout("tcp", address, 3*time.Second)
	if err != nil {
		appLogger.Warn("二次代理 TCP 连通性检查失败", "error", err, "type", config.Type, "address", address)
		return UpstreamStatus{Connected: false, Message: err.Error()}
	}
	_ = conn.Close()
	appLogger.Info("二次代理 TCP 连通性检查成功", "type", config.Type, "address", address)
	return UpstreamStatus{Connected: true, Message: "已连接"}
}

// dialHTTPProxyTunnel 通过 HTTP 二次代理建立 CONNECT 隧道。
func dialHTTPProxyTunnel(upstreamAddr string, target string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", upstreamAddr, 15*time.Second)
	if err != nil {
		appLogger.Error("连接 HTTP 二次代理失败", "error", err, "upstream", upstreamAddr, "target", target)
		return nil, err
	}

	request := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Connection: Keep-Alive\r\n\r\n", target, target)
	if _, err := conn.Write([]byte(request)); err != nil {
		conn.Close()
		appLogger.Error("发送 HTTP CONNECT 请求到二次代理失败", "error", err, "upstream", upstreamAddr, "target", target)
		return nil, err
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodConnect})
	if err != nil {
		conn.Close()
		appLogger.Error("读取 HTTP 二次代理 CONNECT 响应失败", "error", err, "upstream", upstreamAddr, "target", target)
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		conn.Close()
		appLogger.Error("HTTP 二次代理 CONNECT 返回非成功状态", "status", resp.Status, "upstream", upstreamAddr, "target", target)
		return nil, fmt.Errorf("二次代理返回 %s", resp.Status)
	}

	return conn, nil
}

// relayConnections 在客户端和目标连接之间双向转发数据。
func relayConnections(left net.Conn, right net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	copyConn := func(dst net.Conn, src net.Conn) {
		defer wg.Done()
		_, _ = io.Copy(dst, src)
		if tcp, ok := dst.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		} else {
			_ = dst.Close()
		}
	}

	go copyConn(left, right)
	go copyConn(right, left)
	wg.Wait()
	_ = left.Close()
	_ = right.Close()
}

// removeHopHeaders 删除不应该被代理转发的逐跳请求头。
func removeHopHeaders(header http.Header) {
	for _, key := range []string{
		"Connection",
		"Proxy-Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailer",
		"Transfer-Encoding",
		"Upgrade",
	} {
		header.Del(key)
	}
}

// copyHeader 复制 HTTP 响应头。
func copyHeader(dst http.Header, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}
