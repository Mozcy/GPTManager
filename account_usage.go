package main

import (
	"context"
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
	accountUsageEndpoint     = "https://chatgpt.com/backend-api/wham/usage"
	accountUsageUpdatedEvent = "account:usage-updated"
	accountUsageErrorEvent   = "account:usage-error"
	accountUsageUserAgent    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
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
		a.refreshAllAccountUsage(ctx)

		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				appLogger.Info("账号额度后台刷新任务已停止")
				return
			case <-ticker.C:
				a.refreshAllAccountUsage(ctx)
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
func (a *App) refreshAllAccountUsage(ctx context.Context) {
	if err := a.ensureProxyService(); err != nil {
		appLogger.Warn("刷新账号额度失败: 服务未初始化", "error", err)
		return
	}

	a.usageMu.Lock()
	if a.usageRunning {
		a.usageMu.Unlock()
		appLogger.Info("账号额度刷新正在进行，跳过本次触发")
		return
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
		return
	}
	if len(records) == 0 {
		appLogger.Info("暂无账号需要刷新额度")
		return
	}

	upstreamConfig := a.proxyManager.GetUpstreamConfig()
	transport, err := newUpstreamTransport(upstreamConfig)
	if err != nil {
		appLogger.Error("刷新账号额度失败: 创建二次代理 HTTP 客户端失败", "error", err)
		return
	}
	defer transport.CloseIdleConnections()

	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}

	appLogger.Info("开始刷新账号额度", "count", len(records), "upstream", upstreamConfig.Type+"://"+upstreamConfig.IP+":"+upstreamConfig.Port)
	for _, record := range records {
		select {
		case <-ctx.Done():
			return
		default:
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

	return a.proxyStore.UpdateAccountUsage(
		record.AccountID,
		usage.UserID,
		usage.Email,
		strings.ToLower(usage.PlanType),
		usage.RateLimit.PrimaryWindow.toUsageWindowInfo(),
		usage.RateLimit.SecondaryWindow.toUsageWindowInfo(),
	)
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
	req.Header.Set("Accept", "application/json")

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
