package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	accountUsageEndpoint        = "https://chatgpt.com/backend-api/wham/usage"
	accountWorkspaceEndpoint    = "https://chatgpt.com/backend-api/accounts"
	accountUsageUpdatedEvent    = "account:usage-updated"
	accountUsageErrorEvent      = "account:usage-error"
	accountUsageUserAgent       = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36"
	accountUsageRefreshInterval = 15 * time.Minute
	accountUsageRefreshCooldown = 2 * time.Minute
)

// AccountUsageError 表示单个账号额度刷新失败事件。
type AccountUsageError struct {
	AccountID string `json:"accountId"`
	Message   string `json:"message"`
}

type accountUsageResponse struct {
	UserID    string `json:"user_id"`
	AccountID string `json:"account_id"`
	Email     string `json:"email"`
	PlanType  string `json:"plan_type"`
	RateLimit struct {
		Allowed         bool                `json:"allowed"`
		LimitReached    bool                `json:"limit_reached"`
		PrimaryWindow   usageWindowResponse `json:"primary_window"`
		SecondaryWindow usageWindowResponse `json:"secondary_window"`
	} `json:"rate_limit"`
}

type accountWorkspaceResponse struct {
	Items []accountWorkspaceInfo `json:"items"`
}

type accountWorkspaceInfo struct {
	ID                              string `json:"id"`
	Name                            string `json:"name"`
	ProfilePictureID                string `json:"profile_picture_id"`
	ProfilePictureURL               string `json:"profile_picture_url"`
	Structure                       string `json:"structure"`
	CreatedTime                     string `json:"created_time"`
	Processor                       string `json:"processor"`
	CurrentUserRole                 string `json:"current_user_role"`
	EligibleForAutoReactivation     bool   `json:"eligible_for_auto_reactivation"`
}

type usageWindowResponse struct {
	UsedPercent        int   `json:"used_percent"`
	LimitWindowSeconds int64 `json:"limit_window_seconds"`
	ResetAfterSeconds  int64 `json:"reset_after_seconds"`
	ResetAt            int64 `json:"reset_at"`
}

// startAccountUsageRefresher 启动账号额度后台刷新任务。
func (a *App) startAccountUsageRefresher() {
	if a.usageCancel != nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.usageCancel = cancel

	a.usageWG.Add(1)
	go func() {
		defer a.usageWG.Done()
		appLogger.Info("账号额度后台刷新任务已启动")
		if err := a.refreshAllAccountUsage(ctx); err != nil {
			appLogger.Info("账号额度后台刷新跳过", "error", err)
		}

		ticker := time.NewTicker(accountUsageRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				appLogger.Info("账号额度后台刷新任务已停止")
				return
			case <-ticker.C:
				if err := a.refreshAllAccountUsage(ctx); err != nil {
					appLogger.Info("账号额度后台刷新跳过", "error", err)
				}
			}
		}
	}()
}

// stopAccountUsageRefresher 停止账号额度后台刷新任务。
func (a *App) stopAccountUsageRefresher() {
	if a.usageCancel == nil {
		return
	}
	a.usageCancel()
	a.usageCancel = nil
	a.usageWG.Wait()
}

// refreshAllAccountUsage 刷新所有账号额度，逐个账号更新后推送事件给前端。
func (a *App) refreshAllAccountUsage(ctx context.Context) error {
	if err := a.ensureProxyService(); err != nil {
		appLogger.Warn("刷新账号额度失败: 服务未初始化", "error", err)
		return err
	}

	a.usageMu.Lock()
	if a.usageRunning {
		a.usageMu.Unlock()
		appLogger.Info("账号额度刷新正在进行，跳过本次触发")
		return errors.New("账号额度刷新正在进行")
	}
	a.usageRunning = true
	a.usageMu.Unlock()
	defer func() {
		a.usageMu.Lock()
		a.usageRunning = false
		a.usageMu.Unlock()
	}()

	records, err := a.proxyStore.ListAccountRecords()
	if err != nil {
		appLogger.Error("刷新账号额度失败: 查询账号失败", "error", err)
		return err
	}
	if len(records) == 0 {
		appLogger.Info("暂无账号需要刷新额度")
		return nil
	}
	if err := a.reserveAccountUsageRefresh(); err != nil {
		appLogger.Info("账号额度刷新过于频繁，跳过本次触发", "error", err)
		return err
	}

	upstreamConfig := a.proxyManager.GetUpstreamConfig()
	client, closeIdle, err := newAccountUsageClient(upstreamConfig)
	if err != nil {
		appLogger.Error("刷新账号额度失败: 创建二次代理 HTTP 客户端失败", "error", err)
		return err
	}
	defer closeIdle()

	appLogger.Info("开始刷新账号额度", "count", len(records), "upstream", upstreamConfig.Type+"://"+upstreamConfig.IP+":"+upstreamConfig.Port)
	for index, record := range records {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if index > 0 {
			if !sleepAccountUsageJitter(ctx) {
				return ctx.Err()
			}
		}

		updated, err := a.refreshAccountUsage(ctx, client, record)
		if err != nil {
			appLogger.Error("刷新账号额度失败", "error", err, "account_id", record.AccountID, "email", record.Email)
			a.emitAccountUsageError(record.AccountID, err)
			continue
		}
		appLogger.Info("账号额度刷新完成",
			"account_id", updated.AccountID,
			"email", updated.Email,
			"primary_used_percent", updated.PrimaryWindow.UsedPercent,
			"secondary_used_percent", updated.SecondaryWindow.UsedPercent,
		)
		if a.ctx != nil {
			wailsRuntime.EventsEmit(a.ctx, accountUsageUpdatedEvent, updated)
		}
	}
	return nil
}

// reserveAccountUsageRefresh 记录本次刷新时间，避免短时间重复触发导致 403。
func (a *App) reserveAccountUsageRefresh() error {
	a.usageMu.Lock()
	defer a.usageMu.Unlock()

	now := time.Now()
	if !a.usageLastRun.IsZero() {
		elapsed := now.Sub(a.usageLastRun)
		if elapsed < accountUsageRefreshCooldown {
			return fmt.Errorf("请 %d 秒后再刷新", int((accountUsageRefreshCooldown-elapsed).Seconds())+1)
		}
	}
	a.usageLastRun = now
	return nil
}

// newAccountUsageClient 创建额度刷新专用 HTTP Client，和正常代理流量隔离。
func newAccountUsageClient(config UpstreamConfig) (*http.Client, func(), error) {
	transport, err := newUpstreamTransport(config)
	if err != nil {
		return nil, nil, err
	}
	transport.DisableKeepAlives = true
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: false}

	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}
	return client, transport.CloseIdleConnections, nil
}

// sleepAccountUsageJitter 在账号之间加入短暂错峰，降低连续请求特征。
func sleepAccountUsageJitter(ctx context.Context) bool {
	delay := time.Duration(2+time.Now().UnixNano()%5) * time.Second
	select {
	case <-ctx.Done():
		return false
	case <-time.After(delay):
		return true
	}
}

// refreshAccountUsage 拉取单个账号额度并更新数据库。
func (a *App) refreshAccountUsage(ctx context.Context, client *http.Client, record accountRecord) (AccountInfo, error) {
	if strings.TrimSpace(record.AccountID) == "" {
		return AccountInfo{}, errors.New("账号 account_id 为空")
	}
	if strings.TrimSpace(record.AccessToken) == "" {
		return AccountInfo{}, errors.New("账号 access_token 为空")
	}

	usage, err := fetchAccountUsage(ctx, client, record)
	if err != nil {
		return AccountInfo{}, err
	}
	if record.UserID != "" && usage.UserID != "" && usage.UserID != record.UserID {
		return AccountInfo{}, fmt.Errorf("额度接口返回的 user_id 与本地账号不一致: local=%s remote=%s", record.UserID, usage.UserID)
	}

	updated, err := a.proxyStore.UpdateAccountUsage(
		record.AccountID,
		usage.UserID,
		usage.Email,
		strings.ToLower(usage.PlanType),
		usage.RateLimit.PrimaryWindow.toUsageWindowInfo(),
		usage.RateLimit.SecondaryWindow.toUsageWindowInfo(),
	)
	if err != nil {
		return AccountInfo{}, err
	}

	workspace, ok, err := fetchAccountWorkspace(ctx, client, record)
	if err != nil {
		appLogger.Warn("刷新账号工作空间失败", "error", err, "account_id", record.AccountID, "email", record.Email)
		return updated, nil
	}
	if !ok {
		appLogger.Warn("未匹配到账号工作空间", "account_id", record.AccountID, "email", record.Email)
		return updated, nil
	}

	workspaceUpdated, err := a.proxyStore.UpdateAccountWorkspace(record.AccountID, workspace)
	if err != nil {
		appLogger.Warn("保存账号工作空间失败", "error", err, "account_id", record.AccountID, "workspace", workspace.Name)
		return updated, nil
	}
	return workspaceUpdated, nil
}

// toUsageWindowInfo 将接口 snake_case 响应转换为前端使用的 camelCase 结构。
func (w usageWindowResponse) toUsageWindowInfo() UsageWindowInfo {
	return UsageWindowInfo{
		UsedPercent:        w.UsedPercent,
		LimitWindowSeconds: w.LimitWindowSeconds,
		ResetAfterSeconds:  w.ResetAfterSeconds,
		ResetAt:            w.ResetAt,
	}
}

// fetchAccountUsage 调用 ChatGPT 额度接口获取单个账号额度信息。
func fetchAccountUsage(ctx context.Context, client *http.Client, record accountRecord) (accountUsageResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, accountUsageEndpoint, nil)
	if err != nil {
		return accountUsageResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+record.AccessToken)
	req.Header.Set("ChatGPT-Account-Id", record.AccountID)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", accountUsageUserAgent)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Origin", "https://chatgpt.com")
	req.Header.Set("Referer", "https://chatgpt.com/")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")

	resp, err := client.Do(req)
	if err != nil {
		return accountUsageResponse{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return accountUsageResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return accountUsageResponse{}, fmt.Errorf("额度接口返回失败: %s %s", resp.Status, string(body))
	}

	var usage accountUsageResponse
	if err := json.Unmarshal(body, &usage); err != nil {
		return accountUsageResponse{}, err
	}
	return usage, nil
}

// fetchAccountWorkspace 调用 ChatGPT accounts 接口并按本地 account_id 匹配工作空间。
func fetchAccountWorkspace(ctx context.Context, client *http.Client, record accountRecord) (accountWorkspaceInfo, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, accountWorkspaceEndpoint, nil)
	if err != nil {
		return accountWorkspaceInfo{}, false, err
	}
	req.Header.Set("Authorization", "Bearer "+record.AccessToken)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Origin", "https://chatgpt.com")
	req.Header.Set("Referer", "https://chatgpt.com/")
	req.Header.Set("User-Agent", accountUsageUserAgent)
	req.Header.Set("Sec-CH-UA", `"Chromium";v="136", "Google Chrome";v="136"`)

	resp, err := client.Do(req)
	if err != nil {
		return accountWorkspaceInfo{}, false, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return accountWorkspaceInfo{}, false, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return accountWorkspaceInfo{}, false, fmt.Errorf("工作空间接口返回失败: %s %s", resp.Status, string(body))
	}

	var result accountWorkspaceResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return accountWorkspaceInfo{}, false, err
	}
	for _, item := range result.Items {
		if item.ID == record.AccountID {
			return item, true, nil
		}
	}
	return accountWorkspaceInfo{}, false, nil
}

// emitAccountUsageError 推送单个账号额度刷新失败事件。
func (a *App) emitAccountUsageError(accountID string, err error) {
	if a.ctx == nil {
		return
	}
	wailsRuntime.EventsEmit(a.ctx, accountUsageErrorEvent, AccountUsageError{
		AccountID: accountID,
		Message:   err.Error(),
	})
}
