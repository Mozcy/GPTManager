package main

import (
	"context"
	"sort"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const codexProcessChangedEvent = "codex-process:changed"

type codexProcessChangedPayload struct {
	PIDs []int32 `json:"pids"`
}

func (a *App) startCodexProcessWatcher() {
	a.codexWatchMu.Lock()
	defer a.codexWatchMu.Unlock()

	if a.codexWatchCancel != nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.codexWatchCancel = cancel
	a.codexWatchWG.Add(1)
	go func() {
		defer a.codexWatchWG.Done()
		a.watchCodexProcessPIDs(ctx)
	}()
	appLogger.Info("Codex 进程 PID 监听已启动")
}

func (a *App) stopCodexProcessWatcher() {
	a.codexWatchMu.Lock()
	cancel := a.codexWatchCancel
	a.codexWatchCancel = nil
	a.codexWatchMu.Unlock()

	if cancel == nil {
		return
	}
	cancel()
	a.codexWatchWG.Wait()
	appLogger.Info("Codex 进程 PID 监听已停止")
}

func (a *App) watchCodexProcessPIDs(ctx context.Context) {
	previous, err := scanCodexProcessIDsByName("codex.exe")
	if err != nil {
		appLogger.Warn("Codex 进程 PID 初始扫描失败", "error", err)
		previous = nil
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			current, err := scanCodexProcessIDsByName("codex.exe")
			if err != nil {
				appLogger.Warn("Codex 进程 PID 扫描失败", "error", err)
				continue
			}
			if sameProcessIDs(previous, current) {
				continue
			}
			previous = current
			a.emitCodexProcessChanged(current)
		}
	}
}

func (a *App) emitCodexProcessChanged(pids []int32) {
	appLogger.Info("Codex 进程 PID 集合变化", "count", len(pids), "pids", pids)
	if a.ctx != nil {
		wailsRuntime.EventsEmit(a.ctx, codexProcessChangedEvent, codexProcessChangedPayload{
			PIDs: append([]int32(nil), pids...),
		})
	}
}

func sameProcessIDs(left []int32, right []int32) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func normalizeProcessIDs(pids []int32) []int32 {
	normalized := make([]int32, 0, len(pids))
	seen := make(map[int32]struct{}, len(pids))
	for _, pid := range pids {
		if pid <= 0 {
			continue
		}
		if _, ok := seen[pid]; ok {
			continue
		}
		seen[pid] = struct{}{}
		normalized = append(normalized, pid)
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i] < normalized[j]
	})
	return normalized
}
