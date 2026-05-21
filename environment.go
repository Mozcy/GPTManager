package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type codexAuthFile struct {
	Tokens codexAuthTokens `json:"tokens"`
}

type codexAuthTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	AccountID    string `json:"account_id"`
}

// GetEnvironmentConfig 返回已保存的环境配置。
func (a *App) GetEnvironmentConfig() (EnvironmentConfig, error) {
	if err := a.ensureProxyService(); err != nil {
		appLogger.Error("获取环境配置失败: 服务未初始化", "error", err)
		return EnvironmentConfig{}, err
	}
	return a.proxyStore.GetEnvironmentConfig()
}

// GetCodexAuthInfo 返回环境管理展示的 Codex Auth 信息。
func (a *App) GetCodexAuthInfo() (CodexAuthInfo, error) {
	if err := a.ensureProxyService(); err != nil {
		appLogger.Error("获取 Codex Auth 信息失败: 服务未初始化", "error", err)
		return CodexAuthInfo{}, err
	}
	info, err := a.proxyStore.GetCodexAuthInfo()
	if err != nil {
		return CodexAuthInfo{}, err
	}
	if updatedAt, ok := codexAuthFileUpdatedAt(info.Path); ok {
		info.UpdatedAt = updatedAt
	}
	return info, nil
}

// ScanCodexAuth 扫描默认 Codex auth.json，解析认证信息后写入环境配置。
func (a *App) ScanCodexAuth() (CodexAuthInfo, error) {
	if err := a.ensureProxyService(); err != nil {
		appLogger.Error("扫描 Codex auth.json 失败: 服务未初始化", "error", err)
		return CodexAuthInfo{}, err
	}

	path, err := defaultCodexAuthPath()
	if err != nil {
		appLogger.Error("扫描 Codex auth.json 失败: 获取用户目录失败", "error", err)
		return CodexAuthInfo{}, err
	}
	authFile, fileUpdatedAt, err := readCodexAuthFile(path)
	if err != nil {
		return CodexAuthInfo{}, err
	}
	fileUpdatedAtText := formatCodexAuthFileTime(fileUpdatedAt)

	token := oauthTokenResponse{
		AccessToken:  authFile.Tokens.AccessToken,
		RefreshToken: authFile.Tokens.RefreshToken,
		IDToken:      authFile.Tokens.IDToken,
		TokenType:    firstNonEmpty(authFile.Tokens.TokenType, "Bearer"),
		AccountID:    authFile.Tokens.AccountID,
	}
	record, err := buildAccountFromToken(token)
	if err != nil {
		return CodexAuthInfo{}, fmt.Errorf("解析 Codex auth.json 账号信息失败: %w", err)
	}

	workspaceName := ""
	if workspace, ok := a.fetchCodexAuthWorkspace(context.Background(), record); ok {
		workspaceName = workspace.Name
	}

	info := codexAuthInfoFromRecord(path, record, workspaceName, fileUpdatedAtText)
	if _, err := a.proxyStore.SaveCodexAuthInfo(info); err != nil {
		appLogger.Error("保存 Codex auth.json 扫描结果失败", "error", err, "path", path, "account_id", record.AccountID)
		return CodexAuthInfo{}, err
	}

	appLogger.Info("Codex auth.json 认证扫描完成", "path", path, "account_id", info.AccountID, "email", info.Email)
	return info, nil
}

// ScanCodexAuthPath 保留旧绑定兼容，实际执行完整认证扫描。
func (a *App) ScanCodexAuthPath() (CodexAuthInfo, error) {
	return a.ScanCodexAuth()
}

func readCodexAuthFile(path string) (codexAuthFile, time.Time, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return codexAuthFile{}, time.Time{}, errors.New("未找到 Codex auth.json: " + path)
	}
	if err != nil {
		return codexAuthFile{}, time.Time{}, err
	}
	if info.IsDir() {
		return codexAuthFile{}, time.Time{}, errors.New("Codex auth.json 路径是目录: " + path)
	}

	file, err := os.Open(path)
	if err != nil {
		return codexAuthFile{}, time.Time{}, fmt.Errorf("打开 Codex auth.json 失败: %w", err)
	}
	defer file.Close()

	var authFile codexAuthFile
	if err := json.NewDecoder(file).Decode(&authFile); err != nil {
		return codexAuthFile{}, time.Time{}, fmt.Errorf("解析 Codex auth.json 失败: %w", err)
	}
	if authFile.Tokens.AccessToken == "" {
		return codexAuthFile{}, time.Time{}, errors.New("Codex auth.json 缺少 access_token")
	}
	return authFile, info.ModTime(), nil
}

func (a *App) fetchCodexAuthWorkspace(ctx context.Context, record accountRecord) (accountWorkspaceInfo, bool) {
	client, closeIdle, err := newAccountUsageClient(a.proxyManager.GetUpstreamConfig())
	if err != nil {
		appLogger.Warn("认证扫描查询工作空间失败: 创建 HTTP 客户端失败", "error", err, "account_id", record.AccountID)
		return accountWorkspaceInfo{}, false
	}
	defer closeIdle()

	workspace, ok, err := fetchAccountWorkspace(ctx, client, record, accountUsageUserAgentForAccount(record))
	if err != nil {
		appLogger.Warn("认证扫描查询工作空间失败", "error", err, "account_id", record.AccountID, "email", record.Email)
		return accountWorkspaceInfo{}, false
	}
	if !ok {
		appLogger.Warn("认证扫描未匹配到账号工作空间", "account_id", record.AccountID, "email", record.Email)
		return accountWorkspaceInfo{}, false
	}
	return workspace, true
}

func codexAuthInfoFromRecord(path string, record accountRecord, workspaceName string, fileUpdatedAt string) CodexAuthInfo {
	return CodexAuthInfo{
		Path:          path,
		AccountID:     record.AccountID,
		Email:         record.Email,
		Subscription:  record.Subscription,
		WorkspaceName: workspaceName,
		UpdatedAt:     fileUpdatedAt,
	}
}

func codexAuthFileUpdatedAt(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", false
	}
	return formatCodexAuthFileTime(info.ModTime()), true
}

func formatCodexAuthFileTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Local().Format("2006-01-02 15:04:05")
}

func defaultCodexAuthPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".codex", "auth.json"), nil
}
