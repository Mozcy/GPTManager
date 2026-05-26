package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

// UpstreamConfig 表示全局代理配置，账号授权和额度刷新都会使用它作为出口。
type UpstreamConfig struct {
	Type string `json:"type"`
	IP   string `json:"ip"`
	Port string `json:"port"`
}

// UpstreamStatus 表示代理连接检查结果。
type UpstreamStatus struct {
	Connected bool   `json:"connected"`
	Message   string `json:"message"`
}

// EnvironmentConfig 表示本机环境配置。
type EnvironmentConfig struct {
	CodexAuthPath      string `json:"codexAuthPath"`
	CodexAccountID     string `json:"codexAccountId"`
	CodexEmail         string `json:"codexEmail"`
	CodexSubscription  string `json:"codexSubscription"`
	CodexWorkspaceName string `json:"codexWorkspaceName"`
	UpdatedAt          string `json:"updatedAt"`
}

// CodexAuthInfo 表示环境管理中展示的 Codex Auth 信息。
type CodexAuthInfo struct {
	Path          string `json:"path"`
	AccountID     string `json:"accountId"`
	Email         string `json:"email"`
	Subscription  string `json:"subscription"`
	WorkspaceName string `json:"workspaceName"`
	UpdatedAt     string `json:"updatedAt"`
	AuthMode      string `json:"authMode"`
	LastRefresh   string `json:"lastRefresh"`
	AccessToken   string `json:"accessToken"`
	IDToken       string `json:"idToken"`
	RefreshToken  string `json:"refreshToken"`
	TokenType     string `json:"tokenType"`
}

// ProxyStore 负责代理配置和账号信息的 SQLite 持久化。
type ProxyStore struct {
	db *sql.DB
}

// NewProxyStore 创建配置存储，数据库文件位于用户 Local AppData 目录。
func NewProxyStore() (*ProxyStore, error) {
	dataDir, err := appDataDir()
	if err != nil {
		return nil, fmt.Errorf("获取用户 Local AppData 目录失败: %w", err)
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}

	dbPath := filepath.Join(dataDir, "gptmanager.db")
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

// initSchema 初始化代理和账号表结构。
func (s *ProxyStore) initSchema() error {
	const schema = `
CREATE TABLE IF NOT EXISTS upstream_config (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	type TEXT NOT NULL,
	ip TEXT NOT NULL,
	port TEXT NOT NULL,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS environment_config (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	codex_auth_path TEXT NOT NULL DEFAULT '',
	codex_account_id TEXT NOT NULL DEFAULT '',
	codex_email TEXT NOT NULL DEFAULT '',
	codex_subscription TEXT NOT NULL DEFAULT '',
	codex_workspace_name TEXT NOT NULL DEFAULT '',
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS accounts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	provider TEXT NOT NULL,
	subject TEXT NOT NULL,
	user_id TEXT NOT NULL DEFAULT '',
	account_id TEXT NOT NULL DEFAULT '',
	email TEXT NOT NULL DEFAULT '',
	name TEXT NOT NULL DEFAULT '',
	workspace_name TEXT NOT NULL DEFAULT '',
	workspace_structure TEXT NOT NULL DEFAULT '',
	workspace_created_time TEXT NOT NULL DEFAULT '',
	workspace_processor TEXT NOT NULL DEFAULT '',
	workspace_role TEXT NOT NULL DEFAULT '',
	workspace_profile_picture_id TEXT NOT NULL DEFAULT '',
	workspace_profile_picture_url TEXT NOT NULL DEFAULT '',
	workspace_eligible_for_auto_reactivation INTEGER NOT NULL DEFAULT 0,
	subscription TEXT NOT NULL DEFAULT '',
	subscription_expires_at TEXT NOT NULL DEFAULT '',
	primary_window TEXT NOT NULL DEFAULT '',
	secondary_window TEXT NOT NULL DEFAULT '',
	active INTEGER NOT NULL DEFAULT 0,
	access_token TEXT NOT NULL,
	refresh_token TEXT NOT NULL DEFAULT '',
	id_token TEXT NOT NULL DEFAULT '',
	token_type TEXT NOT NULL DEFAULT '',
	expires_at TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(provider, account_id)
);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("初始化配置表失败: %w", err)
	}
	if err := s.migrateAccountColumns(); err != nil {
		return err
	}
	if err := s.migrateEnvironmentColumns(); err != nil {
		return err
	}
	appLogger.Info("SQLite 表结构检查完成")
	return nil
}

// migrateEnvironmentColumns 为旧版本环境表补充新增字段。
func (s *ProxyStore) migrateEnvironmentColumns() error {
	migrations := []string{
		"ALTER TABLE environment_config ADD COLUMN codex_account_id TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE environment_config ADD COLUMN codex_email TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE environment_config ADD COLUMN codex_subscription TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE environment_config ADD COLUMN codex_workspace_name TEXT NOT NULL DEFAULT ''",
	}
	for _, migration := range migrations {
		if _, err := s.db.Exec(migration); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			return fmt.Errorf("迁移环境配置表结构失败: %w", err)
		}
	}
	return nil
}

// migrateAccountColumns 为旧版本账号表补充新增字段。
func (s *ProxyStore) migrateAccountColumns() error {
	migrations := []string{
		"ALTER TABLE accounts ADD COLUMN account_id TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE accounts ADD COLUMN user_id TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE accounts ADD COLUMN subscription TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE accounts ADD COLUMN subscription_expires_at TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE accounts ADD COLUMN primary_window TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE accounts ADD COLUMN secondary_window TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE accounts ADD COLUMN active INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE accounts ADD COLUMN workspace_name TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE accounts ADD COLUMN workspace_structure TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE accounts ADD COLUMN workspace_created_time TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE accounts ADD COLUMN workspace_processor TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE accounts ADD COLUMN workspace_role TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE accounts ADD COLUMN workspace_profile_picture_id TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE accounts ADD COLUMN workspace_profile_picture_url TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE accounts ADD COLUMN workspace_eligible_for_auto_reactivation INTEGER NOT NULL DEFAULT 0",
	}
	for _, migration := range migrations {
		if _, err := s.db.Exec(migration); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			return fmt.Errorf("迁移账号表结构失败: %w", err)
		}
	}
	if err := s.migrateAccountUniqueKey(); err != nil {
		return err
	}
	return nil
}

// migrateAccountUniqueKey 将账号唯一键从 provider+subject 迁移为 provider+account_id，支持同一用户加入多个 Team。
func (s *ProxyStore) migrateAccountUniqueKey() error {
	var tableSQL string
	err := s.db.QueryRow("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'accounts'").Scan(&tableSQL)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("检查账号表唯一键失败: %w", err)
	}
	normalized := strings.ToLower(strings.NewReplacer(" ", "", "\n", "", "\r", "", "\t", "").Replace(tableSQL))
	if !strings.Contains(normalized, "unique(provider,subject)") {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("开始迁移账号唯一键失败: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.Exec("DROP TABLE IF EXISTS accounts_new"); err != nil {
		return fmt.Errorf("清理账号迁移临时表失败: %w", err)
	}
	if _, err := tx.Exec(`
CREATE TABLE accounts_new (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	provider TEXT NOT NULL,
	subject TEXT NOT NULL,
	user_id TEXT NOT NULL DEFAULT '',
	account_id TEXT NOT NULL DEFAULT '',
	email TEXT NOT NULL DEFAULT '',
	name TEXT NOT NULL DEFAULT '',
	workspace_name TEXT NOT NULL DEFAULT '',
	workspace_structure TEXT NOT NULL DEFAULT '',
	workspace_created_time TEXT NOT NULL DEFAULT '',
	workspace_processor TEXT NOT NULL DEFAULT '',
	workspace_role TEXT NOT NULL DEFAULT '',
	workspace_profile_picture_id TEXT NOT NULL DEFAULT '',
	workspace_profile_picture_url TEXT NOT NULL DEFAULT '',
	workspace_eligible_for_auto_reactivation INTEGER NOT NULL DEFAULT 0,
	subscription TEXT NOT NULL DEFAULT '',
	subscription_expires_at TEXT NOT NULL DEFAULT '',
	primary_window TEXT NOT NULL DEFAULT '',
	secondary_window TEXT NOT NULL DEFAULT '',
	active INTEGER NOT NULL DEFAULT 0,
	access_token TEXT NOT NULL,
	refresh_token TEXT NOT NULL DEFAULT '',
	id_token TEXT NOT NULL DEFAULT '',
	token_type TEXT NOT NULL DEFAULT '',
	expires_at TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(provider, account_id)
)`); err != nil {
		return fmt.Errorf("创建新账号表失败: %w", err)
	}

	if _, err := tx.Exec(`
INSERT INTO accounts_new (
	id, provider, subject, user_id, account_id, email, name,
	workspace_name, workspace_structure, workspace_created_time, workspace_processor, workspace_role,
	workspace_profile_picture_id, workspace_profile_picture_url, workspace_eligible_for_auto_reactivation,
	subscription, subscription_expires_at,
	primary_window, secondary_window, active, access_token, refresh_token, id_token, token_type, expires_at, created_at, updated_at
)
SELECT
	id, provider, subject, user_id,
	CASE WHEN account_id <> '' THEN account_id ELSE 'legacy-' || id END,
	email, name,
	workspace_name, workspace_structure, workspace_created_time, workspace_processor, workspace_role,
	workspace_profile_picture_id, workspace_profile_picture_url, workspace_eligible_for_auto_reactivation,
	subscription, subscription_expires_at,
	primary_window, secondary_window, active, access_token, refresh_token, id_token, token_type, expires_at, created_at, updated_at
FROM accounts`); err != nil {
		return fmt.Errorf("迁移账号数据失败: %w", err)
	}

	if _, err := tx.Exec("DROP TABLE accounts"); err != nil {
		return fmt.Errorf("删除旧账号表失败: %w", err)
	}
	if _, err := tx.Exec("ALTER TABLE accounts_new RENAME TO accounts"); err != nil {
		return fmt.Errorf("重命名新账号表失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交账号唯一键迁移失败: %w", err)
	}
	committed = true
	appLogger.Info("账号唯一键已迁移为 provider+account_id")
	return nil
}

// GetUpstreamConfig 返回全局代理配置，未配置时返回默认值。
func (s *ProxyStore) GetUpstreamConfig() (UpstreamConfig, error) {
	var config UpstreamConfig
	err := s.db.QueryRow("SELECT type, ip, port FROM upstream_config WHERE id = 1").
		Scan(&config.Type, &config.IP, &config.Port)
	if errors.Is(err, sql.ErrNoRows) {
		config = defaultUpstreamConfig()
		appLogger.Info("未找到代理配置，使用默认值", "type", config.Type, "address", config.IP+":"+config.Port)
		return config, nil
	}
	if err != nil {
		return UpstreamConfig{}, fmt.Errorf("查询代理配置失败: %w", err)
	}
	config, err = normalizeUpstreamConfig(config)
	if err != nil {
		return UpstreamConfig{}, err
	}
	appLogger.Info("读取代理配置完成", "type", config.Type, "address", config.IP+":"+config.Port)
	return config, nil
}

// SaveUpstreamConfig 保存全局代理配置。
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
		return UpstreamConfig{}, fmt.Errorf("保存代理配置失败: %w", err)
	}
	appLogger.Info("代理配置已保存数据库", "type", config.Type, "address", config.IP+":"+config.Port)
	return config, nil
}

// GetEnvironmentConfig 返回本机环境配置。
func (s *ProxyStore) GetEnvironmentConfig() (EnvironmentConfig, error) {
	var config EnvironmentConfig
	err := s.db.QueryRow(`
SELECT codex_auth_path, codex_account_id, codex_email, codex_subscription, codex_workspace_name, updated_at
FROM environment_config
WHERE id = 1`).
		Scan(
			&config.CodexAuthPath,
			&config.CodexAccountID,
			&config.CodexEmail,
			&config.CodexSubscription,
			&config.CodexWorkspaceName,
			&config.UpdatedAt,
		)
	if errors.Is(err, sql.ErrNoRows) {
		return EnvironmentConfig{}, nil
	}
	if err != nil {
		return EnvironmentConfig{}, fmt.Errorf("查询环境配置失败: %w", err)
	}
	return config, nil
}

// SaveCodexAuthInfo 保存 Codex auth.json 扫描结果。
func (s *ProxyStore) SaveCodexAuthInfo(input CodexAuthInfo) (EnvironmentConfig, error) {
	input.Path = strings.TrimSpace(input.Path)
	if input.Path == "" {
		return EnvironmentConfig{}, errors.New("Codex auth.json 路径不能为空")
	}
	input.AccountID = strings.TrimSpace(input.AccountID)
	input.Email = strings.TrimSpace(input.Email)
	input.Subscription = strings.TrimSpace(input.Subscription)
	input.WorkspaceName = strings.TrimSpace(input.WorkspaceName)
	input.UpdatedAt = strings.TrimSpace(input.UpdatedAt)

	row := s.db.QueryRow(`
INSERT INTO environment_config (
	id, codex_auth_path, codex_account_id, codex_email, codex_subscription, codex_workspace_name, updated_at
)
VALUES (1, ?, ?, ?, ?, ?, CASE WHEN ? <> '' THEN ? ELSE strftime('%Y-%m-%d %H:%M:%S', 'now', 'localtime') END)
ON CONFLICT(id) DO UPDATE SET
	codex_auth_path = excluded.codex_auth_path,
	codex_account_id = excluded.codex_account_id,
	codex_email = excluded.codex_email,
	codex_subscription = excluded.codex_subscription,
	codex_workspace_name = excluded.codex_workspace_name,
	updated_at = excluded.updated_at
RETURNING codex_auth_path, codex_account_id, codex_email, codex_subscription, codex_workspace_name, updated_at`,
		input.Path,
		input.AccountID,
		input.Email,
		input.Subscription,
		input.WorkspaceName,
		input.UpdatedAt,
		input.UpdatedAt,
	)

	var config EnvironmentConfig
	if err := row.Scan(
		&config.CodexAuthPath,
		&config.CodexAccountID,
		&config.CodexEmail,
		&config.CodexSubscription,
		&config.CodexWorkspaceName,
		&config.UpdatedAt,
	); err != nil {
		return EnvironmentConfig{}, fmt.Errorf("保存 Codex auth.json 扫描结果失败: %w", err)
	}
	appLogger.Info("Codex auth.json 扫描结果已保存", "path", config.CodexAuthPath, "account_id", config.CodexAccountID, "email", config.CodexEmail)
	return config, nil
}

// GetCodexAuthInfo 返回环境管理展示的 Codex Auth 信息。
func (s *ProxyStore) GetCodexAuthInfo() (CodexAuthInfo, error) {
	config, err := s.GetEnvironmentConfig()
	if err != nil {
		return CodexAuthInfo{}, err
	}
	info := CodexAuthInfo{
		Path:          config.CodexAuthPath,
		AccountID:     config.CodexAccountID,
		Email:         config.CodexEmail,
		Subscription:  config.CodexSubscription,
		WorkspaceName: config.CodexWorkspaceName,
		UpdatedAt:     config.UpdatedAt,
	}
	return info, nil
}

// ListAccounts 返回已保存的账号信息。
func (s *ProxyStore) ListAccounts() ([]AccountInfo, error) {
	rows, err := s.db.Query(`
SELECT id, provider, subject, user_id, account_id, email, name, workspace_name, workspace_structure, workspace_created_time, workspace_processor, workspace_role, workspace_profile_picture_id, workspace_profile_picture_url, workspace_eligible_for_auto_reactivation, subscription, subscription_expires_at, primary_window, secondary_window, active, expires_at, updated_at,
	access_token, refresh_token, id_token, token_type
FROM accounts
ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("查询账号失败: %w", err)
	}
	defer rows.Close()

	var result []AccountInfo
	for rows.Next() {
		item, _, err := scanAccountRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("读取账号失败: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取账号失败: %w", err)
	}

	appLogger.Info("查询账号列表完成", "count", len(result))
	return result, nil
}

// ListAccountRecords 返回包含 token 的账号记录，仅供后端刷新额度使用。
func (s *ProxyStore) ListAccountRecords() ([]accountRecord, error) {
	rows, err := s.db.Query(`
SELECT id, provider, subject, user_id, account_id, email, name, workspace_name, workspace_structure, workspace_created_time, workspace_processor, workspace_role, workspace_profile_picture_id, workspace_profile_picture_url, workspace_eligible_for_auto_reactivation, subscription, subscription_expires_at, primary_window, secondary_window, active, expires_at, updated_at,
	access_token, refresh_token, id_token, token_type
FROM accounts
WHERE access_token <> ''
ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("查询账号 token 失败: %w", err)
	}
	defer rows.Close()

	var result []accountRecord
	for rows.Next() {
		info, record, err := scanAccountRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("读取账号 token 失败: %w", err)
		}
		record.AccountInfo = info
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取账号 token 失败: %w", err)
	}

	appLogger.Info("查询账号 token 完成", "count", len(result))
	return result, nil
}

// GetAccountRecord 根据账号 ID 查询包含 token 的账号记录，仅供后端内部使用。
func (s *ProxyStore) GetAccountRecord(id int64) (accountRecord, error) {
	if id <= 0 {
		return accountRecord{}, errors.New("账号 ID 无效")
	}

	row := s.db.QueryRow(`
SELECT id, provider, subject, user_id, account_id, email, name, workspace_name, workspace_structure, workspace_created_time, workspace_processor, workspace_role, workspace_profile_picture_id, workspace_profile_picture_url, workspace_eligible_for_auto_reactivation, subscription, subscription_expires_at, primary_window, secondary_window, active, expires_at, updated_at,
	access_token, refresh_token, id_token, token_type
FROM accounts
WHERE id = ?`, id)
	info, record, err := scanAccountRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return accountRecord{}, errors.New("账号不存在")
	}
	if err != nil {
		return accountRecord{}, fmt.Errorf("查询账号 token 失败: %w", err)
	}
	record.AccountInfo = info
	appLogger.Info("查询账号 token 完成", "id", record.ID, "account_id", record.AccountID, "email", record.Email)
	return record, nil
}

// GetActiveAccountRecord 返回数据库中持久化的激活账号。
func (s *ProxyStore) GetActiveAccountRecord() (accountRecord, bool, error) {
	row := s.db.QueryRow(`
SELECT id, provider, subject, user_id, account_id, email, name, workspace_name, workspace_structure, workspace_created_time, workspace_processor, workspace_role, workspace_profile_picture_id, workspace_profile_picture_url, workspace_eligible_for_auto_reactivation, subscription, subscription_expires_at, primary_window, secondary_window, active, expires_at, updated_at,
	access_token, refresh_token, id_token, token_type
FROM accounts
WHERE active = 1 AND access_token <> ''
ORDER BY updated_at DESC, id DESC
LIMIT 1`)
	info, record, err := scanAccountRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return accountRecord{}, false, nil
	}
	if err != nil {
		return accountRecord{}, false, fmt.Errorf("查询激活账号失败: %w", err)
	}
	record.AccountInfo = info
	appLogger.Info("查询激活账号完成", "id", record.ID, "account_id", record.AccountID, "email", record.Email)
	return record, true, nil
}

// SetActiveAccount 将指定账号标记为唯一激活账号，并返回包含 token 的记录。
func (s *ProxyStore) SetActiveAccount(id int64) (accountRecord, error) {
	if id <= 0 {
		return accountRecord{}, errors.New("账号 ID 无效")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return accountRecord{}, fmt.Errorf("开始激活账号事务失败: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	row := tx.QueryRow(`
UPDATE accounts
SET active = 1, updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND access_token <> ''
RETURNING id, provider, subject, user_id, account_id, email, name, workspace_name, workspace_structure, workspace_created_time, workspace_processor, workspace_role, workspace_profile_picture_id, workspace_profile_picture_url, workspace_eligible_for_auto_reactivation, subscription, subscription_expires_at, primary_window, secondary_window, active, expires_at, updated_at,
	access_token, refresh_token, id_token, token_type`, id)
	info, record, err := scanAccountRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return accountRecord{}, errors.New("账号不存在或 access_token 为空")
	}
	if err != nil {
		return accountRecord{}, fmt.Errorf("激活账号失败: %w", err)
	}
	record.AccountInfo = info

	if _, err := tx.Exec("UPDATE accounts SET active = 0 WHERE active = 1 AND id <> ?", id); err != nil {
		return accountRecord{}, fmt.Errorf("清理其他激活账号失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return accountRecord{}, fmt.Errorf("提交激活账号事务失败: %w", err)
	}
	committed = true

	appLogger.Info("账号激活状态已写入数据库", "id", record.ID, "account_id", record.AccountID, "email", record.Email)
	return record, nil
}

type sqlScanner interface {
	Scan(dest ...any) error
}

// scanAccountInfo 扫描账号公开信息并解析额度窗口 JSON。
func scanAccountInfo(scanner sqlScanner) (AccountInfo, error) {
	var item AccountInfo
	var primaryWindow string
	var secondaryWindow string
	var active int
	var workspaceEligible int
	err := scanner.Scan(
		&item.ID,
		&item.Provider,
		&item.Subject,
		&item.UserID,
		&item.AccountID,
		&item.Email,
		&item.Name,
		&item.WorkspaceName,
		&item.WorkspaceStructure,
		&item.WorkspaceCreatedTime,
		&item.WorkspaceProcessor,
		&item.WorkspaceRole,
		&item.WorkspaceProfilePictureID,
		&item.WorkspaceProfilePictureURL,
		&workspaceEligible,
		&item.Subscription,
		&item.SubscriptionExpiresAt,
		&primaryWindow,
		&secondaryWindow,
		&active,
		&item.ExpiresAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return AccountInfo{}, err
	}
	item.PrimaryWindow = decodeUsageWindow(primaryWindow)
	item.SecondaryWindow = decodeUsageWindow(secondaryWindow)
	item.Active = active == 1
	item.WorkspaceEligibleForAutoReactivation = workspaceEligible == 1
	return item, nil
}

// scanAccountRecord 扫描账号公开信息和 token 明文，仅供后端内部使用。
func scanAccountRecord(scanner sqlScanner) (AccountInfo, accountRecord, error) {
	var item AccountInfo
	var record accountRecord
	var primaryWindow string
	var secondaryWindow string
	var active int
	var workspaceEligible int
	err := scanner.Scan(
		&item.ID,
		&item.Provider,
		&item.Subject,
		&item.UserID,
		&item.AccountID,
		&item.Email,
		&item.Name,
		&item.WorkspaceName,
		&item.WorkspaceStructure,
		&item.WorkspaceCreatedTime,
		&item.WorkspaceProcessor,
		&item.WorkspaceRole,
		&item.WorkspaceProfilePictureID,
		&item.WorkspaceProfilePictureURL,
		&workspaceEligible,
		&item.Subscription,
		&item.SubscriptionExpiresAt,
		&primaryWindow,
		&secondaryWindow,
		&active,
		&item.ExpiresAt,
		&item.UpdatedAt,
		&record.AccessToken,
		&record.RefreshToken,
		&record.IDToken,
		&record.TokenType,
	)
	if err != nil {
		return AccountInfo{}, accountRecord{}, err
	}
	item.PrimaryWindow = decodeUsageWindow(primaryWindow)
	item.SecondaryWindow = decodeUsageWindow(secondaryWindow)
	item.Active = active == 1
	item.WorkspaceEligibleForAutoReactivation = workspaceEligible == 1
	item.AccessToken = record.AccessToken
	item.IDToken = record.IDToken
	item.RefreshToken = record.RefreshToken
	return item, record, nil
}

// encodeUsageWindow 将额度窗口序列化为数据库 JSON。
func encodeUsageWindow(window UsageWindowInfo) (string, error) {
	data, err := json.Marshal(usageWindowResponse{
		UsedPercent:        window.UsedPercent,
		LimitWindowSeconds: window.LimitWindowSeconds,
		ResetAfterSeconds:  window.ResetAfterSeconds,
		ResetAt:            window.ResetAt,
	})
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// decodeUsageWindow 从数据库 JSON 解析额度窗口，空值返回零值。
func decodeUsageWindow(value string) UsageWindowInfo {
	if strings.TrimSpace(value) == "" {
		return UsageWindowInfo{}
	}
	var response usageWindowResponse
	if err := json.Unmarshal([]byte(value), &response); err != nil {
		appLogger.Warn("解析账号额度窗口失败", "error", err)
		return UsageWindowInfo{}
	}
	window := response.toUsageWindowInfo()
	if !isZeroUsageWindow(window) || strings.Contains(value, "used_percent") {
		return window
	}

	var legacy UsageWindowInfo
	if err := json.Unmarshal([]byte(value), &legacy); err != nil {
		appLogger.Warn("解析旧版账号额度窗口失败", "error", err)
		return UsageWindowInfo{}
	}
	return legacy
}

// isZeroUsageWindow 判断额度窗口是否为零值。
func isZeroUsageWindow(window UsageWindowInfo) bool {
	return window.UsedPercent == 0 &&
		window.LimitWindowSeconds == 0 &&
		window.ResetAfterSeconds == 0 &&
		window.ResetAt == 0
}

// SaveAccount 保存或更新 OAuth 账号及 token。
func (s *ProxyStore) SaveAccount(input accountRecord) (AccountInfo, error) {
	if input.Provider == "" || input.Subject == "" {
		return AccountInfo{}, errors.New("账号 provider 和 subject 不能为空")
	}
	if input.UserID == "" {
		return AccountInfo{}, errors.New("账号 user_id 不能为空")
	}
	if input.AccountID == "" {
		return AccountInfo{}, errors.New("账号 account_id 不能为空")
	}
	if input.AccessToken == "" {
		return AccountInfo{}, errors.New("账号 access_token 不能为空")
	}

	row := s.db.QueryRow(`
INSERT INTO accounts (
	provider, subject, user_id, account_id, email, name, subscription, subscription_expires_at,
	access_token, refresh_token, id_token, token_type, expires_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(provider, account_id) DO UPDATE SET
	subject = excluded.subject,
	user_id = excluded.user_id,
	email = excluded.email,
	name = excluded.name,
	subscription = excluded.subscription,
	subscription_expires_at = excluded.subscription_expires_at,
	access_token = excluded.access_token,
	refresh_token = excluded.refresh_token,
	id_token = excluded.id_token,
	token_type = excluded.token_type,
	expires_at = excluded.expires_at,
	updated_at = CURRENT_TIMESTAMP
RETURNING id, provider, subject, user_id, account_id, email, name, workspace_name, workspace_structure, workspace_created_time, workspace_processor, workspace_role, workspace_profile_picture_id, workspace_profile_picture_url, workspace_eligible_for_auto_reactivation, subscription, subscription_expires_at, primary_window, secondary_window, active, expires_at, updated_at`,
		input.Provider, input.Subject, input.UserID, input.AccountID, input.Email, input.Name, input.Subscription, input.SubscriptionExpiresAt,
		input.AccessToken, input.RefreshToken, input.IDToken, input.TokenType, input.ExpiresAt)
	item, err := scanAccountInfo(row)
	if err != nil {
		return AccountInfo{}, fmt.Errorf("保存账号失败: %w", err)
	}
	if item.ID <= 0 || item.Provider == "" || item.Subject == "" || item.UserID == "" || item.AccountID == "" {
		return AccountInfo{}, fmt.Errorf("账号保存结果无效: id=%d provider=%q subject=%q user_id=%q account_id=%q", item.ID, item.Provider, item.Subject, item.UserID, item.AccountID)
	}
	appLogger.Info("账号已保存数据库", "id", item.ID, "provider", item.Provider, "user_id", item.UserID, "account_id", item.AccountID, "email", item.Email, "subject", item.Subject, "subscription", item.Subscription)
	return item, nil
}

// GetAccountBySubject 根据 provider 和 subject 查询账号。
func (s *ProxyStore) GetAccountBySubject(provider string, subject string) (AccountInfo, error) {
	row := s.db.QueryRow(`
SELECT id, provider, subject, user_id, account_id, email, name, workspace_name, workspace_structure, workspace_created_time, workspace_processor, workspace_role, workspace_profile_picture_id, workspace_profile_picture_url, workspace_eligible_for_auto_reactivation, subscription, subscription_expires_at, primary_window, secondary_window, active, expires_at, updated_at
FROM accounts
WHERE provider = ? AND subject = ?`, provider, subject)
	item, err := scanAccountInfo(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AccountInfo{}, errors.New("账号不存在")
	}
	if err != nil {
		return AccountInfo{}, fmt.Errorf("查询账号失败: %w", err)
	}
	return item, nil
}

// GetAccountByAccountID 根据 account_id 查询账号。
func (s *ProxyStore) GetAccountByAccountID(accountID string) (AccountInfo, error) {
	row := s.db.QueryRow(`
SELECT id, provider, subject, user_id, account_id, email, name, workspace_name, workspace_structure, workspace_created_time, workspace_processor, workspace_role, workspace_profile_picture_id, workspace_profile_picture_url, workspace_eligible_for_auto_reactivation, subscription, subscription_expires_at, primary_window, secondary_window, active, expires_at, updated_at
FROM accounts
WHERE account_id = ?`, accountID)
	item, err := scanAccountInfo(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AccountInfo{}, errors.New("账号不存在")
	}
	if err != nil {
		return AccountInfo{}, fmt.Errorf("查询账号失败: %w", err)
	}
	return item, nil
}

// UpdateAccountUsage 更新账号额度窗口并返回最新公开账号信息。
func (s *ProxyStore) UpdateAccountUsage(accountID string, userID string, email string, subscription string, primaryWindow UsageWindowInfo, secondaryWindow UsageWindowInfo) (AccountInfo, error) {
	if strings.TrimSpace(accountID) == "" {
		return AccountInfo{}, errors.New("账号 account_id 不能为空")
	}

	primaryValue, err := encodeUsageWindow(primaryWindow)
	if err != nil {
		return AccountInfo{}, fmt.Errorf("序列化 5 小时额度窗口失败: %w", err)
	}
	secondaryValue, err := encodeUsageWindow(secondaryWindow)
	if err != nil {
		return AccountInfo{}, fmt.Errorf("序列化一周额度窗口失败: %w", err)
	}

	row := s.db.QueryRow(`
UPDATE accounts
SET user_id = CASE WHEN ? <> '' THEN ? ELSE user_id END,
	email = CASE WHEN ? <> '' THEN ? ELSE email END,
	subscription = CASE WHEN ? <> '' THEN ? ELSE subscription END,
	primary_window = ?,
	secondary_window = ?,
	updated_at = CURRENT_TIMESTAMP
WHERE account_id = ?
RETURNING id, provider, subject, user_id, account_id, email, name, workspace_name, workspace_structure, workspace_created_time, workspace_processor, workspace_role, workspace_profile_picture_id, workspace_profile_picture_url, workspace_eligible_for_auto_reactivation, subscription, subscription_expires_at, primary_window, secondary_window, active, expires_at, updated_at`,
		userID, userID,
		email, email,
		subscription, subscription,
		primaryValue,
		secondaryValue,
		accountID,
	)
	item, err := scanAccountInfo(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AccountInfo{}, errors.New("账号不存在")
	}
	if err != nil {
		return AccountInfo{}, fmt.Errorf("更新账号额度失败: %w", err)
	}

	appLogger.Info("账号额度已更新数据库",
		"id", item.ID,
		"account_id", item.AccountID,
		"primary_used_percent", item.PrimaryWindow.UsedPercent,
		"secondary_used_percent", item.SecondaryWindow.UsedPercent,
	)
	return item, nil
}

// UpdateAccountWorkspace 更新账号匹配到的 ChatGPT workspace 信息。
func (s *ProxyStore) UpdateAccountWorkspace(accountID string, workspace accountWorkspaceInfo) (AccountInfo, error) {
	if strings.TrimSpace(accountID) == "" {
		return AccountInfo{}, errors.New("账号 account_id 不能为空")
	}
	if strings.TrimSpace(workspace.ID) == "" {
		return AccountInfo{}, errors.New("工作空间 id 不能为空")
	}
	if workspace.ID != accountID {
		return AccountInfo{}, fmt.Errorf("工作空间 id 与账号 account_id 不一致: local=%s remote=%s", accountID, workspace.ID)
	}

	eligible := 0
	if workspace.EligibleForAutoReactivation {
		eligible = 1
	}

	row := s.db.QueryRow(`
UPDATE accounts
SET workspace_name = ?,
	workspace_structure = ?,
	workspace_created_time = ?,
	workspace_processor = ?,
	workspace_role = ?,
	workspace_profile_picture_id = ?,
	workspace_profile_picture_url = ?,
	workspace_eligible_for_auto_reactivation = ?,
	updated_at = CURRENT_TIMESTAMP
WHERE account_id = ?
RETURNING id, provider, subject, user_id, account_id, email, name, workspace_name, workspace_structure, workspace_created_time, workspace_processor, workspace_role, workspace_profile_picture_id, workspace_profile_picture_url, workspace_eligible_for_auto_reactivation, subscription, subscription_expires_at, primary_window, secondary_window, active, expires_at, updated_at`,
		workspace.Name,
		workspace.Structure,
		workspace.CreatedTime,
		workspace.Processor,
		workspace.CurrentUserRole,
		workspace.ProfilePictureID,
		workspace.ProfilePictureURL,
		eligible,
		accountID,
	)
	item, err := scanAccountInfo(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AccountInfo{}, errors.New("账号不存在")
	}
	if err != nil {
		return AccountInfo{}, fmt.Errorf("更新账号工作空间失败: %w", err)
	}

	appLogger.Info("账号工作空间已更新数据库",
		"id", item.ID,
		"account_id", item.AccountID,
		"workspace_name", item.WorkspaceName,
	)
	return item, nil
}

// DeleteAccount 删除已保存账号和 token。
func (s *ProxyStore) DeleteAccount(id int64) error {
	result, err := s.db.Exec("DELETE FROM accounts WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("删除账号失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("确认账号删除结果失败: %w", err)
	}
	if affected == 0 {
		return errors.New("账号不存在")
	}
	appLogger.Info("账号已从数据库删除", "id", id)
	return nil
}

// defaultUpstreamConfig 返回默认代理配置。
func defaultUpstreamConfig() UpstreamConfig {
	return UpstreamConfig{
		Type: "http",
		IP:   "127.0.0.1",
		Port: "1080",
	}
}

// normalizeUpstreamConfig 清理并校验全局代理配置输入。
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
		return UpstreamConfig{}, errors.New("代理 IP 不能为空")
	}
	if config.Type != "http" && config.Type != "socks5" {
		return UpstreamConfig{}, errors.New("代理协议仅支持 http 或 socks5")
	}
	if err := validatePort(config.Port, "代理端口"); err != nil {
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
