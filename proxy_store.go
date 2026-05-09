package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

// ProxyConfig 表示一条本地代理监听配置。
type ProxyConfig struct {
	ID      int64  `json:"id"`
	IP      string `json:"ip"`
	Port    string `json:"port"`
	Enabled bool   `json:"enabled"`
}

// UpstreamConfig 表示全局二次代理配置，所有本地代理都会使用它作为出口。
type UpstreamConfig struct {
	Type string `json:"type"`
	IP   string `json:"ip"`
	Port string `json:"port"`
}

// UpstreamStatus 表示二次代理连接检查结果。
type UpstreamStatus struct {
	Connected bool   `json:"connected"`
	Message   string `json:"message"`
}

// ProxyStore 负责代理配置的 SQLite 持久化。
type ProxyStore struct {
	db *sql.DB
}

// NewProxyStore 创建代理配置存储，数据库文件位于用户 Local AppData 目录。
func NewProxyStore() (*ProxyStore, error) {
	dataDir, err := appDataDir()
	if err != nil {
		return nil, fmt.Errorf("获取用户 Local AppData 目录失败: %w", err)
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}

	dbPath := filepath.Join(dataDir, "gptproxy.db")
	appLogger.Info("打开 SQLite 数据库", "path", dbPath)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite 数据库失败: %w", err)
	}

	store := &ProxyStore{db: db}
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, err
	}

	appLogger.Info("SQLite 数据库初始化完成", "path", dbPath)
	return store, nil
}

// Close 关闭 SQLite 连接。
func (s *ProxyStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	err := s.db.Close()
	if err != nil {
		appLogger.Error("关闭 SQLite 数据库失败", "error", err)
		return err
	}
	appLogger.Info("SQLite 数据库已关闭")
	return nil
}

// initSchema 初始化代理配置表和全局二次代理配置表。
func (s *ProxyStore) initSchema() error {
	const schema = `
CREATE TABLE IF NOT EXISTS proxies (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	ip TEXT NOT NULL,
	port TEXT NOT NULL,
	upstream_type TEXT NOT NULL DEFAULT '',
	upstream_ip TEXT NOT NULL DEFAULT '',
	upstream_port TEXT NOT NULL DEFAULT '',
	enabled INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS upstream_config (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	type TEXT NOT NULL,
	ip TEXT NOT NULL,
	port TEXT NOT NULL,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("初始化代理配置表失败: %w", err)
	}
	appLogger.Info("SQLite 表结构检查完成")
	return nil
}

// ListProxies 返回全部代理配置。
func (s *ProxyStore) ListProxies() ([]ProxyConfig, error) {
	rows, err := s.db.Query(`
SELECT id, ip, port, enabled
FROM proxies
ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("查询代理配置失败: %w", err)
	}
	defer rows.Close()

	var result []ProxyConfig
	for rows.Next() {
		var item ProxyConfig
		var enabled int
		if err := rows.Scan(&item.ID, &item.IP, &item.Port, &enabled); err != nil {
			return nil, fmt.Errorf("读取代理配置失败: %w", err)
		}
		item.Enabled = enabled == 1
		result = append(result, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取代理配置失败: %w", err)
	}

	appLogger.Info("查询代理配置完成", "count", len(result))
	return result, nil
}

// CreateProxy 新增一条代理配置。
func (s *ProxyStore) CreateProxy(input ProxyConfig) (ProxyConfig, error) {
	item, err := normalizeProxyConfig(input)
	if err != nil {
		return ProxyConfig{}, err
	}

	result, err := s.db.Exec(`
INSERT INTO proxies (ip, port, upstream_type, upstream_ip, upstream_port, enabled, updated_at)
VALUES (?, ?, '', '', '', 0, CURRENT_TIMESTAMP)`,
		item.IP, item.Port)
	if err != nil {
		return ProxyConfig{}, fmt.Errorf("创建代理配置失败: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return ProxyConfig{}, fmt.Errorf("获取代理配置 ID 失败: %w", err)
	}
	item.ID = id
	appLogger.Info("代理配置已写入数据库", "id", item.ID, "listen", item.IP+":"+item.Port)
	return item, nil
}

// UpdateProxy 更新一条未启用的代理配置。
func (s *ProxyStore) UpdateProxy(input ProxyConfig) (ProxyConfig, error) {
	if input.ID <= 0 {
		return ProxyConfig{}, errors.New("代理 ID 无效")
	}

	current, err := s.GetProxy(input.ID)
	if err != nil {
		return ProxyConfig{}, err
	}
	if current.Enabled {
		return ProxyConfig{}, errors.New("代理启用中，不能编辑")
	}

	item, err := normalizeProxyConfig(input)
	if err != nil {
		return ProxyConfig{}, err
	}
	item.ID = input.ID

	result, err := s.db.Exec(`
UPDATE proxies
SET ip = ?, port = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND enabled = 0`,
		item.IP, item.Port, item.ID)
	if err != nil {
		return ProxyConfig{}, fmt.Errorf("更新代理配置失败: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return ProxyConfig{}, fmt.Errorf("确认更新结果失败: %w", err)
	}
	if affected == 0 {
		return ProxyConfig{}, errors.New("代理配置不存在或正在启用")
	}

	appLogger.Info("代理配置已更新数据库", "id", item.ID, "listen", item.IP+":"+item.Port)
	return item, nil
}

// DeleteProxy 删除一条未启用的代理配置。
func (s *ProxyStore) DeleteProxy(id int64) error {
	result, err := s.db.Exec("DELETE FROM proxies WHERE id = ? AND enabled = 0", id)
	if err != nil {
		return fmt.Errorf("删除代理配置失败: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("确认删除结果失败: %w", err)
	}
	if affected == 0 {
		return errors.New("代理配置不存在或正在启用")
	}

	appLogger.Info("代理配置已从数据库删除", "id", id)
	return nil
}

// GetProxy 根据 ID 查询代理配置。
func (s *ProxyStore) GetProxy(id int64) (ProxyConfig, error) {
	var item ProxyConfig
	var enabled int
	err := s.db.QueryRow(`
SELECT id, ip, port, enabled
FROM proxies
WHERE id = ?`, id).
		Scan(&item.ID, &item.IP, &item.Port, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return ProxyConfig{}, errors.New("代理配置不存在")
	}
	if err != nil {
		return ProxyConfig{}, fmt.Errorf("查询代理配置失败: %w", err)
	}

	item.Enabled = enabled == 1
	appLogger.Info("查询代理配置完成", "id", item.ID, "listen", item.IP+":"+item.Port, "enabled", item.Enabled)
	return item, nil
}

// SetProxyEnabled 更新代理启用状态。
func (s *ProxyStore) SetProxyEnabled(id int64, enabled bool) (ProxyConfig, error) {
	value := 0
	if enabled {
		value = 1
	}

	result, err := s.db.Exec(`
UPDATE proxies
SET enabled = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?`, value, id)
	if err != nil {
		return ProxyConfig{}, fmt.Errorf("更新代理状态失败: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return ProxyConfig{}, fmt.Errorf("确认代理状态失败: %w", err)
	}
	if affected == 0 {
		return ProxyConfig{}, errors.New("代理配置不存在")
	}

	appLogger.Info("代理启用状态已更新数据库", "id", id, "enabled", enabled)
	return s.GetProxy(id)
}

// GetUpstreamConfig 返回全局二次代理配置，未配置时返回默认值。
func (s *ProxyStore) GetUpstreamConfig() (UpstreamConfig, error) {
	var config UpstreamConfig
	err := s.db.QueryRow("SELECT type, ip, port FROM upstream_config WHERE id = 1").
		Scan(&config.Type, &config.IP, &config.Port)
	if errors.Is(err, sql.ErrNoRows) {
		config = defaultUpstreamConfig()
		appLogger.Info("未找到二次代理配置，使用默认值", "type", config.Type, "address", config.IP+":"+config.Port)
		return config, nil
	}
	if err != nil {
		return UpstreamConfig{}, fmt.Errorf("查询二次代理配置失败: %w", err)
	}
	config, err = normalizeUpstreamConfig(config)
	if err != nil {
		return UpstreamConfig{}, err
	}
	appLogger.Info("读取二次代理配置完成", "type", config.Type, "address", config.IP+":"+config.Port)
	return config, nil
}

// SaveUpstreamConfig 保存全局二次代理配置。
func (s *ProxyStore) SaveUpstreamConfig(input UpstreamConfig) (UpstreamConfig, error) {
	config, err := normalizeUpstreamConfig(input)
	if err != nil {
		return UpstreamConfig{}, err
	}

	_, err = s.db.Exec(`
INSERT INTO upstream_config (id, type, ip, port, updated_at)
VALUES (1, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(id) DO UPDATE SET
	type = excluded.type,
	ip = excluded.ip,
	port = excluded.port,
	updated_at = CURRENT_TIMESTAMP`,
		config.Type, config.IP, config.Port)
	if err != nil {
		return UpstreamConfig{}, fmt.Errorf("保存二次代理配置失败: %w", err)
	}
	appLogger.Info("二次代理配置已保存数据库", "type", config.Type, "address", config.IP+":"+config.Port)
	return config, nil
}

// defaultUpstreamConfig 返回默认二次代理配置。
func defaultUpstreamConfig() UpstreamConfig {
	return UpstreamConfig{
		Type: "http",
		IP:   "127.0.0.1",
		Port: "1080",
	}
}

// normalizeProxyConfig 清理并校验本地代理配置输入。
func normalizeProxyConfig(input ProxyConfig) (ProxyConfig, error) {
	item := ProxyConfig{
		ID:      input.ID,
		IP:      strings.TrimSpace(input.IP),
		Port:    strings.TrimSpace(input.Port),
		Enabled: input.Enabled,
	}

	if item.IP == "" {
		return ProxyConfig{}, errors.New("监听 IP 不能为空")
	}
	if err := validatePort(item.Port, "监听端口"); err != nil {
		return ProxyConfig{}, err
	}

	item.IP = normalizeHost(item.IP)
	return item, nil
}

// normalizeUpstreamConfig 清理并校验全局二次代理配置输入。
func normalizeUpstreamConfig(input UpstreamConfig) (UpstreamConfig, error) {
	config := UpstreamConfig{
		Type: strings.ToLower(strings.TrimSpace(input.Type)),
		IP:   strings.TrimSpace(input.IP),
		Port: strings.TrimSpace(input.Port),
	}
	if config.Type == "" {
		config.Type = "http"
	}

	if config.IP == "" {
		return UpstreamConfig{}, errors.New("二次代理 IP 不能为空")
	}
	if config.Type != "http" && config.Type != "socks5" {
		return UpstreamConfig{}, errors.New("二次代理协议仅支持 http 或 socks5")
	}
	if err := validatePort(config.Port, "二次代理端口"); err != nil {
		return UpstreamConfig{}, err
	}

	config.IP = normalizeHost(config.IP)
	return config, nil
}

// validatePort 校验端口是否为 1 到 65535 的数字。
func validatePort(port string, label string) error {
	value, err := strconv.Atoi(port)
	if err != nil || value < 1 || value > 65535 {
		return fmt.Errorf("%s必须是 1-65535 的数字", label)
	}
	return nil
}

// normalizeHost 去掉用户输入中可能包含的端口，只保留主机名或 IP。
func normalizeHost(value string) string {
	host, _, err := net.SplitHostPort(value)
	if err == nil {
		return host
	}
	return strings.Trim(value, "[]")
}
