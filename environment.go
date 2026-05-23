package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const codexAuthUpdatedEvent = "codex-auth:updated"

type codexAuthFile struct {
	AuthMode    string          `json:"auth_mode"`
	LastRefresh string          `json:"last_refresh"`
	Tokens      codexAuthTokens `json:"tokens"`
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
	return a.withCodexAuthFileDetails(info), nil
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
	if isTeamSubscription(record.Subscription) {
		if workspace, ok := a.fetchCodexAuthWorkspace(context.Background(), record); ok {
			workspaceName = workspace.Name
		}
	} else {
		appLogger.Info("认证扫描跳过工作空间查询: 非 Team 订阅", "account_id", record.AccountID, "email", record.Email, "subscription", record.Subscription)
	}

	info := codexAuthInfoFromRecord(path, record, workspaceName, fileUpdatedAtText)
	info = codexAuthInfoWithFileDetails(info, authFile)
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

func (a *App) syncCodexAuthFromAccount(record accountRecord) (CodexAuthInfo, error) {
	path, err := a.codexAuthPathForWrite()
	if err != nil {
		return CodexAuthInfo{}, err
	}

	authFile, fileUpdatedAt, err := writeCodexAuthFile(path, record)
	if err != nil {
		return CodexAuthInfo{}, err
	}

	info := codexAuthInfoFromRecord(path, record, record.WorkspaceName, formatCodexAuthFileTime(fileUpdatedAt))
	info = codexAuthInfoWithFileDetails(info, authFile)
	if _, err := a.proxyStore.SaveCodexAuthInfo(info); err != nil {
		return CodexAuthInfo{}, err
	}

	appLogger.Info("Codex auth.json 已同步为激活账号", "path", path, "account_id", record.AccountID, "email", record.Email)
	return info, nil
}

func (a *App) codexAuthPathForWrite() (string, error) {
	config, err := a.proxyStore.GetEnvironmentConfig()
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(config.CodexAuthPath)
	if path != "" {
		return path, nil
	}
	return defaultCodexAuthPath()
}

func writeCodexAuthFile(path string, record accountRecord) (codexAuthFile, time.Time, error) {
	if strings.TrimSpace(path) == "" {
		return codexAuthFile{}, time.Time{}, errors.New("Codex auth.json 路径不能为空")
	}
	if strings.TrimSpace(record.AccessToken) == "" {
		return codexAuthFile{}, time.Time{}, errors.New("激活账号缺少 access_token")
	}

	var data []byte
	if existing, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(existing))) > 0 {
		updated, err := updateCodexAuthJSON(existing, record)
		if err != nil {
			return codexAuthFile{}, time.Time{}, err
		}
		data = updated
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return codexAuthFile{}, time.Time{}, fmt.Errorf("读取现有 Codex auth.json 失败: %w", err)
	} else {
		data = defaultCodexAuthJSON(record, time.Now().UTC().Format(time.RFC3339))
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return codexAuthFile{}, time.Time{}, fmt.Errorf("创建 Codex 配置目录失败: %w", err)
	}

	mode := os.FileMode(0600)
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		mode = info.Mode().Perm()
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		return codexAuthFile{}, time.Time{}, fmt.Errorf("写入 Codex auth.json 失败: %w", err)
	}

	authFile, fileUpdatedAt, err := readCodexAuthFile(path)
	if err != nil {
		return codexAuthFile{}, time.Time{}, err
	}
	return authFile, fileUpdatedAt, nil
}

func updateCodexAuthJSON(data []byte, record accountRecord) ([]byte, error) {
	rootStart := skipJSONWhitespace(data, 0)
	if rootStart >= len(data) || data[rootStart] != '{' {
		return nil, errors.New("解析现有 Codex auth.json 失败: 根节点不是 JSON 对象")
	}
	if _, err := matchingJSONEnd(data, rootStart); err != nil {
		return nil, fmt.Errorf("解析现有 Codex auth.json 失败: %w", err)
	}

	authModeValue := "chatgpt"
	if property, ok, err := findJSONProperty(data, rootStart, "auth_mode"); err != nil {
		return nil, fmt.Errorf("解析现有 Codex auth.json 失败: %w", err)
	} else if ok {
		var existing string
		if err := json.Unmarshal(data[property.valueStart:property.valueEnd], &existing); err == nil && strings.TrimSpace(existing) != "" {
			authModeValue = existing
		}
	}

	var err error
	data, err = upsertJSONStringProperty(data, rootStart, "auth_mode", authModeValue)
	if err != nil {
		return nil, err
	}
	data, err = upsertJSONStringProperty(data, rootStart, "last_refresh", time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}

	tokenValues := []jsonStringField{
		{key: "access_token", value: record.AccessToken},
		{key: "refresh_token", value: record.RefreshToken},
		{key: "id_token", value: record.IDToken},
		{key: "token_type", value: firstNonEmpty(record.TokenType, "Bearer")},
		{key: "account_id", value: record.AccountID},
	}

	property, ok, err := findJSONProperty(data, rootStart, "tokens")
	if err != nil {
		return nil, fmt.Errorf("解析现有 Codex auth.json 失败: %w", err)
	}
	if !ok {
		return insertJSONRawProperty(data, rootStart, "tokens", codexAuthTokensJSON(tokenValues, objectChildIndent(data, rootStart))), nil
	}
	if property.valueStart >= len(data) || data[property.valueStart] != '{' {
		replacement := codexAuthTokensJSON(tokenValues, propertyIndent(data, property.memberStart))
		return replaceJSONRange(data, property.valueStart, property.valueEnd, []byte(replacement)), nil
	}

	tokensStart := property.valueStart
	for _, field := range tokenValues {
		data, err = upsertJSONStringProperty(data, tokensStart, field.key, field.value)
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

type jsonStringField struct {
	key   string
	value string
}

type jsonPropertyRange struct {
	memberStart int
	valueStart  int
	valueEnd    int
}

func defaultCodexAuthJSON(record accountRecord, lastRefresh string) []byte {
	fields := []jsonStringField{
		{key: "access_token", value: record.AccessToken},
		{key: "refresh_token", value: record.RefreshToken},
		{key: "id_token", value: record.IDToken},
		{key: "token_type", value: firstNonEmpty(record.TokenType, "Bearer")},
		{key: "account_id", value: record.AccountID},
	}
	var builder strings.Builder
	builder.WriteString("{\n")
	builder.WriteString("  \"auth_mode\": \"chatgpt\",\n")
	builder.WriteString("  \"last_refresh\": ")
	builder.WriteString(jsonStringLiteral(lastRefresh))
	builder.WriteString(",\n")
	builder.WriteString("  \"tokens\": ")
	builder.WriteString(codexAuthTokensJSON(fields, "  "))
	builder.WriteString("\n}\n")
	return []byte(builder.String())
}

func codexAuthTokensJSON(fields []jsonStringField, indent string) string {
	childIndent := indent + "  "
	var builder strings.Builder
	builder.WriteString("{\n")
	for index, field := range fields {
		if index > 0 {
			builder.WriteString(",\n")
		}
		builder.WriteString(childIndent)
		builder.WriteString(jsonStringLiteral(field.key))
		builder.WriteString(": ")
		builder.WriteString(jsonStringLiteral(field.value))
	}
	builder.WriteString("\n")
	builder.WriteString(indent)
	builder.WriteString("}")
	return builder.String()
}

func upsertJSONStringProperty(data []byte, objectStart int, key string, value string) ([]byte, error) {
	property, ok, err := findJSONProperty(data, objectStart, key)
	if err != nil {
		return nil, fmt.Errorf("解析现有 Codex auth.json 失败: %w", err)
	}
	if !ok {
		return insertJSONRawProperty(data, objectStart, key, jsonStringLiteral(value)), nil
	}
	return replaceJSONRange(data, property.valueStart, property.valueEnd, []byte(jsonStringLiteral(value))), nil
}

func insertJSONRawProperty(data []byte, objectStart int, key string, rawValue string) []byte {
	objectEnd, err := matchingJSONEnd(data, objectStart)
	if err != nil {
		return data
	}
	memberIndent := objectChildIndent(data, objectStart)
	closeIndent := propertyIndent(data, objectStart)
	insert := strings.Builder{}
	if jsonObjectIsEmpty(data, objectStart, objectEnd) {
		insert.WriteString("\n")
		insert.WriteString(memberIndent)
		insert.WriteString(jsonStringLiteral(key))
		insert.WriteString(": ")
		insert.WriteString(rawValue)
		insert.WriteString("\n")
		insert.WriteString(closeIndent)
	} else {
		insert.WriteString(",\n")
		insert.WriteString(memberIndent)
		insert.WriteString(jsonStringLiteral(key))
		insert.WriteString(": ")
		insert.WriteString(rawValue)
	}
	closeIndex := objectEnd - 1
	return replaceJSONRange(data, closeIndex, closeIndex, []byte(insert.String()))
}

func findJSONProperty(data []byte, objectStart int, key string) (jsonPropertyRange, bool, error) {
	if objectStart >= len(data) || data[objectStart] != '{' {
		return jsonPropertyRange{}, false, errors.New("JSON 对象起始位置无效")
	}
	pos := skipJSONWhitespace(data, objectStart+1)
	for pos < len(data) {
		if data[pos] == '}' {
			return jsonPropertyRange{}, false, nil
		}
		if data[pos] != '"' {
			return jsonPropertyRange{}, false, errors.New("JSON 对象成员缺少字段名")
		}
		memberStart := pos
		keyEnd, err := scanJSONStringEnd(data, pos)
		if err != nil {
			return jsonPropertyRange{}, false, err
		}
		var currentKey string
		if err := json.Unmarshal(data[pos:keyEnd], &currentKey); err != nil {
			return jsonPropertyRange{}, false, err
		}
		pos = skipJSONWhitespace(data, keyEnd)
		if pos >= len(data) || data[pos] != ':' {
			return jsonPropertyRange{}, false, errors.New("JSON 对象成员缺少冒号")
		}
		valueStart := skipJSONWhitespace(data, pos+1)
		valueEnd, err := scanJSONValueEnd(data, valueStart)
		if err != nil {
			return jsonPropertyRange{}, false, err
		}
		if currentKey == key {
			return jsonPropertyRange{
				memberStart: memberStart,
				valueStart:  valueStart,
				valueEnd:    valueEnd,
			}, true, nil
		}
		pos = skipJSONWhitespace(data, valueEnd)
		if pos < len(data) && data[pos] == ',' {
			pos = skipJSONWhitespace(data, pos+1)
			continue
		}
		if pos < len(data) && data[pos] == '}' {
			return jsonPropertyRange{}, false, nil
		}
		return jsonPropertyRange{}, false, errors.New("JSON 对象成员分隔符无效")
	}
	return jsonPropertyRange{}, false, errors.New("JSON 对象未闭合")
}

func scanJSONValueEnd(data []byte, pos int) (int, error) {
	pos = skipJSONWhitespace(data, pos)
	if pos >= len(data) {
		return 0, errors.New("JSON 值为空")
	}
	switch data[pos] {
	case '"':
		return scanJSONStringEnd(data, pos)
	case '{', '[':
		return matchingJSONEnd(data, pos)
	default:
		end := pos
		for end < len(data) && data[end] != ',' && data[end] != '}' && data[end] != ']' {
			end++
		}
		for end > pos && isJSONWhitespace(data[end-1]) {
			end--
		}
		if end == pos {
			return 0, errors.New("JSON 值无效")
		}
		var value any
		if err := json.Unmarshal(data[pos:end], &value); err != nil {
			return 0, err
		}
		return end, nil
	}
}

func matchingJSONEnd(data []byte, start int) (int, error) {
	if start >= len(data) || (data[start] != '{' && data[start] != '[') {
		return 0, errors.New("JSON 复合值起始位置无效")
	}
	stack := []byte{data[start]}
	for pos := start + 1; pos < len(data); pos++ {
		switch data[pos] {
		case '"':
			end, err := scanJSONStringEnd(data, pos)
			if err != nil {
				return 0, err
			}
			pos = end - 1
		case '{', '[':
			stack = append(stack, data[pos])
		case '}', ']':
			if len(stack) == 0 {
				return 0, errors.New("JSON 复合值括号不匹配")
			}
			open := stack[len(stack)-1]
			if (open == '{' && data[pos] != '}') || (open == '[' && data[pos] != ']') {
				return 0, errors.New("JSON 复合值括号不匹配")
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return pos + 1, nil
			}
		}
	}
	return 0, errors.New("JSON 复合值未闭合")
}

func scanJSONStringEnd(data []byte, start int) (int, error) {
	if start >= len(data) || data[start] != '"' {
		return 0, errors.New("JSON 字符串起始位置无效")
	}
	escaped := false
	for pos := start + 1; pos < len(data); pos++ {
		if escaped {
			escaped = false
			continue
		}
		if data[pos] == '\\' {
			escaped = true
			continue
		}
		if data[pos] == '"' {
			return pos + 1, nil
		}
	}
	return 0, errors.New("JSON 字符串未闭合")
}

func skipJSONWhitespace(data []byte, pos int) int {
	for pos < len(data) && isJSONWhitespace(data[pos]) {
		pos++
	}
	return pos
}

func isJSONWhitespace(value byte) bool {
	return value == ' ' || value == '\n' || value == '\r' || value == '\t'
}

func jsonObjectIsEmpty(data []byte, objectStart int, objectEnd int) bool {
	for pos := objectStart + 1; pos < objectEnd-1; pos++ {
		if !isJSONWhitespace(data[pos]) {
			return false
		}
	}
	return true
}

func objectChildIndent(data []byte, objectStart int) string {
	return propertyIndent(data, objectStart) + "  "
}

func propertyIndent(data []byte, pos int) string {
	lineStart := pos
	for lineStart > 0 && data[lineStart-1] != '\n' && data[lineStart-1] != '\r' {
		lineStart--
	}
	indentEnd := lineStart
	for indentEnd < len(data) && (data[indentEnd] == ' ' || data[indentEnd] == '\t') {
		indentEnd++
	}
	return string(data[lineStart:indentEnd])
}

func jsonStringLiteral(value string) string {
	data, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(data)
}

func replaceJSONRange(data []byte, start int, end int, replacement []byte) []byte {
	result := make([]byte, 0, len(data)-(end-start)+len(replacement))
	result = append(result, data[:start]...)
	result = append(result, replacement...)
	result = append(result, data[end:]...)
	return result
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

func (a *App) withCodexAuthFileDetails(info CodexAuthInfo) CodexAuthInfo {
	if strings.TrimSpace(info.Path) == "" {
		return info
	}
	authFile, _, err := readCodexAuthFile(info.Path)
	if err != nil {
		appLogger.Warn("读取 Codex auth.json 详情失败", "error", err, "path", info.Path)
		return info
	}
	return codexAuthInfoWithFileDetails(info, authFile)
}

func codexAuthInfoWithFileDetails(info CodexAuthInfo, authFile codexAuthFile) CodexAuthInfo {
	info.AuthMode = authFile.AuthMode
	info.LastRefresh = authFile.LastRefresh
	info.AccessToken = authFile.Tokens.AccessToken
	info.IDToken = authFile.Tokens.IDToken
	info.RefreshToken = authFile.Tokens.RefreshToken
	info.TokenType = firstNonEmpty(authFile.Tokens.TokenType, "Bearer")
	return info
}

func isTeamSubscription(subscription string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(subscription)), "team")
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
