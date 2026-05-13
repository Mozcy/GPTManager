package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	openAIAuthEndpoint  = "https://auth.openai.com/oauth/authorize"
	openAITokenEndpoint = "https://auth.openai.com/oauth/token"
	openAIClientID      = "app_EMoamEEZ73f0CkXaXp7hrann"
	openAIRedirectURI   = "http://localhost:1455/auth/callback"
	openAIScope         = "openid profile email offline_access"
)

// AccountInfo 表示前端展示用的账号信息。
type AccountInfo struct {
	// ID 是本地数据库账号记录 ID。
	ID                    int64           `json:"id"`
	// Provider 是账号来源，目前固定为 openai。
	Provider              string          `json:"provider"`
	// Subject 是 OAuth/JWT 中的 sub，用于唯一标识授权主体。
	Subject               string          `json:"subject"`
	// UserID 是 ChatGPT 用户 ID，来自 token claims 中的 chatgpt_user_id/user_id。
	UserID                string          `json:"userId"`
	// AccountID 是 ChatGPT 账号 ID，代理请求会写入 ChatGPT-Account-Id 请求头。
	AccountID             string          `json:"accountId"`
	// Email 是账号邮箱，仅用于展示。
	Email                 string          `json:"email"`
	// Name 是账号昵称，仅用于展示。
	Name                  string          `json:"name"`
	// WorkspaceName 是 ChatGPT workspace/team 名称，来自 /backend-api/accounts 匹配 account_id 的账号项。
	WorkspaceName         string          `json:"workspaceName"`
	// WorkspaceStructure 是 ChatGPT workspace 结构，例如 workspace/personal。
	WorkspaceStructure    string          `json:"workspaceStructure"`
	// WorkspaceCreatedTime 是 ChatGPT workspace 创建时间。
	WorkspaceCreatedTime  string          `json:"workspaceCreatedTime"`
	// WorkspaceProcessor 是 ChatGPT workspace 账单处理器。
	WorkspaceProcessor    string          `json:"workspaceProcessor"`
	// WorkspaceRole 是当前用户在 ChatGPT workspace 中的角色。
	WorkspaceRole         string          `json:"workspaceRole"`
	// WorkspaceProfilePictureID 是 ChatGPT workspace 头像 ID。
	WorkspaceProfilePictureID string      `json:"workspaceProfilePictureId"`
	// WorkspaceProfilePictureURL 是 ChatGPT workspace 头像 URL。
	WorkspaceProfilePictureURL string     `json:"workspaceProfilePictureUrl"`
	// WorkspaceEligibleForAutoReactivation 表示 workspace 是否支持自动恢复。
	WorkspaceEligibleForAutoReactivation bool `json:"workspaceEligibleForAutoReactivation"`
	// Subscription 是 ChatGPT 订阅类型，例如 free/plus/team。
	Subscription          string          `json:"subscription"`
	// SubscriptionExpiresAt 是 ChatGPT 订阅有效期，来自 token claims 的 chatgpt_subscription_active_until，不等同于 access_token 过期时间。
	SubscriptionExpiresAt string          `json:"subscriptionExpiresAt"`
	// PrimaryWindow 是短周期额度窗口信息，当前对应 ChatGPT 5 小时额度。
	PrimaryWindow         UsageWindowInfo `json:"primaryWindow"`
	// SecondaryWindow 是长周期额度窗口信息，当前对应 ChatGPT 7 天额度。
	SecondaryWindow       UsageWindowInfo `json:"secondaryWindow"`
	// Active 表示该账号是否为当前持久化的活动账号。
	Active                bool            `json:"active"`
	// ExpiresAt 是 access_token JWT payload 中 exp 换算出的 Token 过期时间，缺失时才回退到 OAuth 响应 expires_in。
	ExpiresAt             string          `json:"expiresAt"`
	// UpdatedAt 是本地数据库记录最后更新时间。
	UpdatedAt             string          `json:"updatedAt"`
}

// UsageWindowInfo 表示账号额度窗口的使用情况。
type UsageWindowInfo struct {
	UsedPercent        int   `json:"usedPercent"`
	LimitWindowSeconds int64 `json:"limitWindowSeconds"`
	ResetAfterSeconds  int64 `json:"resetAfterSeconds"`
	ResetAt            int64 `json:"resetAt"`
}

type accountRecord struct {
	AccountInfo
	AccessToken  string
	RefreshToken string
	IDToken      string
	TokenType    string
}

type oauthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	AccountID    string `json:"account_id"`
}

type idTokenClaims struct {
	Subject       string             `json:"sub"`
	Email         string             `json:"email"`
	Name          string             `json:"name"`
	ExpiresAt     int64              `json:"exp"`
	OpenAIAuth    openAIAuthClaims   `json:"https://api.openai.com/auth"`
	OpenAIProfile openAIProfileClaim `json:"https://api.openai.com/profile"`
}

type openAIAuthClaims struct {
	ChatGPTAccountID               string `json:"chatgpt_account_id"`
	ChatGPTPlanType                string `json:"chatgpt_plan_type"`
	ChatGPTSubscriptionActiveUntil string `json:"chatgpt_subscription_active_until"`
	ChatGPTUserID                  string `json:"chatgpt_user_id"`
	UserID                         string `json:"user_id"`
}

type openAIProfileClaim struct {
	Email string `json:"email"`
}

type oauthCallbackResult struct {
	Code  string
	State string
	Err   error
}

const (
	accountAuthSuccessEvent = "account:auth-success"
	accountAuthErrorEvent   = "account:auth-error"
)

// StartOpenAIAuth 启动 OpenAI OAuth 授权流程，真正结果会通过 Wails 事件通知前端。
func (a *App) StartOpenAIAuth() error {
	if err := a.ensureProxyService(); err != nil {
		appLogger.Error("启动 OpenAI OAuth 失败: 服务未初始化", "error", err)
		return err
	}
	if a.ctx == nil {
		return errors.New("Wails 上下文为空")
	}

	a.authMu.Lock()
	if a.authRunning {
		a.authMu.Unlock()
		return errors.New("已有 OpenAI 授权流程正在进行")
	}
	a.authRunning = true
	a.authMu.Unlock()

	codeVerifier, err := randomURLString(64)
	if err != nil {
		a.finishOpenAIAuth()
		return fmt.Errorf("生成 PKCE code_verifier 失败: %w", err)
	}
	state, err := randomURLString(32)
	if err != nil {
		a.finishOpenAIAuth()
		return fmt.Errorf("生成 OAuth state 失败: %w", err)
	}

	callbackCh := make(chan oauthCallbackResult, 1)
	server, err := startOAuthCallbackServer(state, callbackCh)
	if err != nil {
		a.finishOpenAIAuth()
		appLogger.Error("启动 OAuth 本地回调服务失败", "error", err)
		return err
	}

	authURL := buildOpenAIAuthURL(codeVerifier, state)
	appLogger.Info("打开 OpenAI OAuth 授权链接")
	openURLInActiveBrowser(a.ctx, authURL)
	go a.waitOpenAIAuthCallback(server, callbackCh, codeVerifier)
	return nil
}

// waitOpenAIAuthCallback 在后台等待 OAuth 回调并通过事件通知前端。
func (a *App) waitOpenAIAuthCallback(server *http.Server, callbackCh <-chan oauthCallbackResult, codeVerifier string) {
	defer a.finishOpenAIAuth()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	var account AccountInfo
	var err error
	select {
	case result := <-callbackCh:
		if result.Err != nil {
			err = result.Err
			break
		}
		upstreamConfig := a.proxyManager.GetUpstreamConfig()
		token, err := exchangeOpenAIToken(result.Code, codeVerifier, upstreamConfig)
		if err != nil {
			break
		}
		appLogger.Info("OpenAI token 交换完成", "has_access_token", token.AccessToken != "", "has_refresh_token", token.RefreshToken != "", "has_id_token", token.IDToken != "")
		record, err := buildAccountFromToken(token)
		if err != nil {
			break
		}
		appLogger.Info("OpenAI 账号信息解析完成", "provider", record.Provider, "subject", record.Subject, "user_id", record.UserID, "account_id", record.AccountID, "email", record.Email, "subscription", record.Subscription)
		account, err = a.proxyStore.SaveAccount(record)
		if err != nil {
			break
		}
		if account.ID <= 0 || account.Provider == "" || account.Subject == "" || account.UserID == "" || account.AccountID == "" {
			err = fmt.Errorf("账号保存结果无效: id=%d provider=%q subject=%q user_id=%q account_id=%q", account.ID, account.Provider, account.Subject, account.UserID, account.AccountID)
			break
		}
		if account.Active {
			record.ID = account.ID
			record.AccountInfo = account
			a.proxyManager.SetActiveAccount(record)
		}
	case <-time.After(5 * time.Minute):
		err = errors.New("OpenAI OAuth 登录超时")
	}

	if err != nil {
		appLogger.Error("OpenAI OAuth 登录失败", "error", err)
		wailsRuntime.EventsEmit(a.ctx, accountAuthErrorEvent, err.Error())
		return
	}
	appLogger.Info("OpenAI 账号登录成功", "id", account.ID, "user_id", account.UserID, "account_id", account.AccountID, "email", account.Email, "subject", account.Subject, "subscription", account.Subscription)
	wailsRuntime.EventsEmit(a.ctx, accountAuthSuccessEvent, account)
	go a.refreshAllAccountUsage(context.Background())
}

// finishOpenAIAuth 标记当前 OpenAI OAuth 流程结束。
func (a *App) finishOpenAIAuth() {
	a.authMu.Lock()
	a.authRunning = false
	a.authMu.Unlock()
}

// buildOpenAIAuthURL 生成 OpenAI OAuth 授权链接。
func buildOpenAIAuthURL(codeVerifier string, state string) string {
	hash := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(hash[:])

	values := url.Values{}
	values.Set("client_id", openAIClientID)
	values.Set("redirect_uri", openAIRedirectURI)
	values.Set("response_type", "code")
	values.Set("scope", openAIScope)
	values.Set("state", state)
	values.Set("code_challenge", codeChallenge)
	values.Set("code_challenge_method", "S256")
	return openAIAuthEndpoint + "?" + values.Encode()
}

// startOAuthCallbackServer 启动临时本地 HTTP 服务捕获 OAuth 回调。
func startOAuthCallbackServer(expectedState string, callbackCh chan<- oauthCallbackResult) (*http.Server, error) {
	mux := http.NewServeMux()
	server := &http.Server{
		Addr:              "localhost:1455",
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if errValue := query.Get("error"); errValue != "" {
			description := query.Get("error_description")
			writeOAuthCallbackPage(w, "登录失败", description)
			callbackCh <- oauthCallbackResult{Err: fmt.Errorf("%s: %s", errValue, description)}
			return
		}

		state := query.Get("state")
		if state != expectedState {
			writeOAuthCallbackPage(w, "登录失败", "state 校验失败")
			callbackCh <- oauthCallbackResult{Err: errors.New("OAuth state 校验失败")}
			return
		}

		code := query.Get("code")
		if code == "" {
			writeOAuthCallbackPage(w, "登录失败", "未收到授权 code")
			callbackCh <- oauthCallbackResult{Err: errors.New("未收到 OAuth 授权 code")}
			return
		}

		writeOAuthCallbackPage(w, "登录成功", "授权已完成，可以回到 GPTProxy。")
		callbackCh <- oauthCallbackResult{Code: code, State: state}
	})

	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return nil, fmt.Errorf("监听 OAuth 回调端口失败: %w", err)
	}

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			appLogger.Error("OAuth 回调服务异常退出", "error", err)
		}
	}()
	appLogger.Info("OAuth 本地回调服务已启动", "address", server.Addr)
	return server, nil
}

// exchangeOpenAIToken 用授权 code 和 PKCE verifier 通过二次代理换取 token。
func exchangeOpenAIToken(code string, codeVerifier string, upstreamConfig UpstreamConfig) (oauthTokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", openAIClientID)
	form.Set("redirect_uri", openAIRedirectURI)
	form.Set("code", code)
	form.Set("code_verifier", codeVerifier)

	req, err := http.NewRequest(http.MethodPost, openAITokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return oauthTokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	upstreamConfig, err = normalizeUpstreamConfig(upstreamConfig)
	if err != nil {
		return oauthTokenResponse{}, fmt.Errorf("二次代理配置无效: %w", err)
	}
	transport, err := newUpstreamTransport(upstreamConfig)
	if err != nil {
		return oauthTokenResponse{}, fmt.Errorf("创建二次代理 HTTP 客户端失败: %w", err)
	}
	defer transport.CloseIdleConnections()

	appLogger.Info("通过二次代理交换 OpenAI token", "type", upstreamConfig.Type, "address", upstreamConfig.IP+":"+upstreamConfig.Port)
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}
	resp, err := client.Do(req)
	if err != nil {
		return oauthTokenResponse{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return oauthTokenResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return oauthTokenResponse{}, fmt.Errorf("token 交换失败: %s %s", resp.Status, string(body))
	}

	var token oauthTokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return oauthTokenResponse{}, err
	}
	if token.AccessToken == "" {
		return oauthTokenResponse{}, errors.New("token 响应缺少 access_token")
	}
	return token, nil
}

// buildAccountFromToken 从 token 响应中提取账号信息。
func buildAccountFromToken(token oauthTokenResponse) (accountRecord, error) {
	claims, err := parseIDTokenClaims(token.IDToken)
	if err != nil {
		if token.AccessToken == "" {
			return accountRecord{}, err
		}
		claims, err = parseIDTokenClaims(token.AccessToken)
		if err != nil {
			return accountRecord{}, err
		}
	}
	var accessClaims idTokenClaims
	if token.AccessToken != "" {
		accessClaims, _ = parseIDTokenClaims(token.AccessToken)
	}

	expiresAt := tokenExpiresAt(accessClaims, claims, token.ExpiresIn)
	email := claims.Email
	if email == "" {
		email = claims.OpenAIProfile.Email
	}
	if email == "" {
		email = accessClaims.Email
	}
	if email == "" {
		email = accessClaims.OpenAIProfile.Email
	}
	accountID := firstNonEmpty(token.AccountID, claims.OpenAIAuth.ChatGPTAccountID, accessClaims.OpenAIAuth.ChatGPTAccountID)
	userID := firstNonEmpty(claims.OpenAIAuth.ChatGPTUserID, claims.OpenAIAuth.UserID, accessClaims.OpenAIAuth.ChatGPTUserID, accessClaims.OpenAIAuth.UserID)
	subscription := strings.ToLower(firstNonEmpty(claims.OpenAIAuth.ChatGPTPlanType, accessClaims.OpenAIAuth.ChatGPTPlanType))
	subscriptionExpiresAt := firstNonEmpty(claims.OpenAIAuth.ChatGPTSubscriptionActiveUntil, accessClaims.OpenAIAuth.ChatGPTSubscriptionActiveUntil)

	return accountRecord{
		AccountInfo: AccountInfo{
			Provider:              "openai",
			Subject:               claims.Subject,
			UserID:                userID,
			AccountID:             accountID,
			Email:                 email,
			Name:                  claims.Name,
			Subscription:          subscription,
			SubscriptionExpiresAt: subscriptionExpiresAt,
			ExpiresAt:             expiresAt,
		},
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		IDToken:      token.IDToken,
		TokenType:    token.TokenType,
	}, nil
}

// firstNonEmpty 返回第一个非空字符串。
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// tokenExpiresAt 优先使用 access_token 的 JWT exp，缺失时回退到 id_token exp，再缺失才使用 expires_in 推算。
func tokenExpiresAt(accessClaims idTokenClaims, idClaims idTokenClaims, expiresIn int64) string {
	if accessClaims.ExpiresAt > 0 {
		return time.Unix(accessClaims.ExpiresAt, 0).UTC().Format(time.RFC3339)
	}
	if idClaims.ExpiresAt > 0 {
		return time.Unix(idClaims.ExpiresAt, 0).UTC().Format(time.RFC3339)
	}
	if expiresIn > 0 {
		return time.Now().Add(time.Duration(expiresIn) * time.Second).UTC().Format(time.RFC3339)
	}
	return ""
}

// parseIDTokenClaims 从 ID Token 载荷中解析用户关键信息。
func parseIDTokenClaims(idToken string) (idTokenClaims, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return idTokenClaims{}, errors.New("id_token 格式无效")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return idTokenClaims{}, err
	}

	var claims idTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return idTokenClaims{}, err
	}
	if claims.Subject == "" {
		return idTokenClaims{}, errors.New("id_token 缺少 sub")
	}
	return claims, nil
}

// randomURLString 生成适用于 OAuth PKCE/state 的随机字符串。
func randomURLString(byteCount int) (string, error) {
	data := make([]byte, byteCount)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

// writeOAuthCallbackPage 输出浏览器回调结果页面。
func writeOAuthCallbackPage(w http.ResponseWriter, title string, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	statusClass := "success"
	statusLabel := "Authentication complete"
	leadText := "You are now signed in to GPTProxy."
	if strings.Contains(title, "失败") {
		statusClass = "error"
		statusLabel = "Authentication failed"
		leadText = "GPTProxy could not complete sign in."
	}

	_ = oauthCallbackPageTemplate.Execute(w, oauthCallbackPageData{
		Title:       title,
		Message:     message,
		StatusClass: statusClass,
		StatusLabel: statusLabel,
		LeadText:    leadText,
	})
}

type oauthCallbackPageData struct {
	Title       string
	Message     string
	StatusClass string
	StatusLabel string
	LeadText    string
}

var oauthCallbackPageTemplate = template.Must(template.New("oauth-callback").Parse(oauthCallbackPageHTML))

const oauthCallbackPageHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>

<style>
:root {
	color-scheme: dark;
	--bg: #0f172a;
	--panel: rgba(15, 23, 42, .78);
	--panel-border: rgba(148, 163, 184, .22);
	--text: #f8fafc;
	--muted: #94a3b8;
	--soft: #1e293b;
	--success: #22c55e;
	--success-bg: rgba(34, 197, 94, .14);
	--error: #ef4444;
	--error-bg: rgba(239, 68, 68, .14);
	--accent: #38bdf8;
}

* {
	box-sizing: border-box;
}

body {
	min-height: 100vh;
	margin: 0;
	display: grid;
	place-items: center;
	padding: 24px;
	background:
		radial-gradient(circle at 20% 20%, rgba(56, 189, 248, .18), transparent 32%),
		radial-gradient(circle at 80% 10%, rgba(34, 197, 94, .12), transparent 28%),
		linear-gradient(135deg, #020617, #0f172a 48%, #111827);
	color: var(--text);
	font-family: Inter, -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;
}

.shell {
	width: min(520px, 100%);
	padding: 34px;
	border: 1px solid var(--panel-border);
	border-radius: 24px;
	background: var(--panel);
	box-shadow:
		0 30px 90px rgba(0, 0, 0, .45),
		inset 0 1px 0 rgba(255, 255, 255, .06);
	backdrop-filter: blur(18px);
}

.brand {
	display: flex;
	align-items: center;
	gap: 12px;
	margin-bottom: 30px;
}

.mark {
	width: 42px;
	height: 42px;
	display: grid;
	place-items: center;
	border-radius: 14px;
	background: linear-gradient(135deg, #38bdf8, #2563eb);
	color: white;
	font-weight: 800;
	letter-spacing: -.04em;
	box-shadow: 0 14px 28px rgba(37, 99, 235, .28);
}

.brand-name {
	font-size: 16px;
	font-weight: 700;
	letter-spacing: -.01em;
}

.brand-sub {
	margin-top: 2px;
	color: var(--muted);
	font-size: 13px;
}

.status {
	display: inline-flex;
	align-items: center;
	gap: 8px;
	margin-bottom: 18px;
	padding: 7px 11px;
	border-radius: 999px;
	background: var(--success-bg);
	color: var(--success);
	font-size: 13px;
	font-weight: 700;
}

.error .status {
	background: var(--error-bg);
	color: var(--error);
}

.dot {
	width: 8px;
	height: 8px;
	border-radius: 50%;
	background: currentColor;
	box-shadow: 0 0 0 5px currentColor;
	opacity: .85;
}

h1 {
	margin: 0;
	font-size: clamp(26px, 5vw, 36px);
	line-height: 1.16;
	font-weight: 800;
	letter-spacing: -.04em;
}

.lead {
	margin-top: 14px;
	color: #cbd5e1;
	font-size: 16px;
	line-height: 1.7;
}

.detail {
	margin-top: 22px;
	padding: 16px 18px;
	border: 1px solid rgba(148, 163, 184, .18);
	border-radius: 16px;
	background: rgba(15, 23, 42, .72);
	color: #dbeafe;
	font-size: 14px;
	line-height: 1.7;
	word-break: break-word;
}

.footer {
	margin-top: 28px;
	padding-top: 22px;
	border-top: 1px solid rgba(148, 163, 184, .16);
	color: var(--muted);
	font-size: 13px;
}

@media (max-width: 520px) {
	.shell {
		padding: 26px;
		border-radius: 20px;
	}
}
</style>
</head>

<body>
<main class="shell {{.StatusClass}}">
	<div class="brand">
		<div class="mark">GP</div>
		<div>
			<div class="brand-name">GPTProxy</div>
			<div class="brand-sub">OAuth Callback</div>
		</div>
	</div>

	<div class="status">
		<span class="dot"></span>
		<span>{{.StatusLabel}}</span>
	</div>

	<h1>{{.Title}}</h1>

	<p class="lead">{{.LeadText}}</p>

	<p class="detail">{{.Message}}</p>

	<div class="footer">
		<span>授权流程已结束，可以关闭此页面并回到 GPTProxy。</span>
	</div>
</main>
</body>
</html>`
