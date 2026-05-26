package main

import (
	"errors"
	"fmt"
	"sort"
)

// CodexProcessInfo 表示 Codex 进程扫描结果。
type CodexProcessInfo struct {
	ProcessID          int32    `json:"pid"`
	Name               string   `json:"name"`
	CommandLine        string   `json:"commandLine"`
	ExecutablePath     string   `json:"executablePath"`
	Owner              string   `json:"owner"`
	CreationDate       string   `json:"creationDate"`
	ParentProcessID    int32    `json:"parentPid"`
	ParentName         string   `json:"parentName"`
	ParentCommandLine  string   `json:"parentCommandLine"`
	ChildProcesses     string   `json:"childProcesses"`
	Status             string   `json:"status"`
	ThreadCount        int32    `json:"threadCount"`
	HandleCount        uint32   `json:"handleCount"`
	WorkingSetMB       *float64 `json:"workingSetMB"`
	VirtualSizeMB      *float64 `json:"virtualSizeMB"`
	PeakWorkingSetMB   *float64 `json:"peakWorkingSetMB"`
	SharedMemoryMB     *float64 `json:"sharedMemoryMB"`
	DataMemoryMB       *float64 `json:"dataMemoryMB"`
	ReadCount          uint64   `json:"readCount"`
	WriteCount         uint64   `json:"writeCount"`
	ReadBytesMB        *float64 `json:"readBytesMB"`
	WriteBytesMB       *float64 `json:"writeBytesMB"`
	CPUPercent         *float64 `json:"cpuPercent"`
	TotalCPUSeconds    *float64 `json:"totalCPUSeconds"`
	UserModeTimeSec    *float64 `json:"userModeTimeSec"`
	KernelModeTimeSec  *float64 `json:"kernelModeTimeSec"`
	IsRunning          *bool    `json:"isRunning"`
	Foreground         *bool    `json:"foreground"`
	FileSizeMB         *float64 `json:"fileSizeMB"`
	FileCreated        string   `json:"fileCreated"`
	FileModified       string   `json:"fileModified"`
	FileProductName    string   `json:"fileProductName"`
	FileProductVersion string   `json:"fileProductVersion"`
	FileVersion        string   `json:"fileVersion"`
	FileCompany        string   `json:"fileCompany"`
	FileDescription    string   `json:"fileDescription"`
	SHA256             string   `json:"sha256"`
	TCPConnections     string   `json:"tcpConnections"`
}

// ScanCodexProcesses 扫描正在运行的 codex.exe 进程。
func (a *App) ScanCodexProcesses() ([]CodexProcessInfo, error) {
	a.clearSelectedCodexProcessPIDs()

	rows, err := scanCodexProcessesByName("codex.exe")
	if err != nil {
		appLogger.Error("扫描 Codex 进程失败", "error", err)
		return nil, err
	}
	appLogger.Info("扫描 Codex 进程完成", "count", len(rows))
	return rows, nil
}

// SetSelectedCodexProcessPIDs 保存当前 Codex Process 表格勾选的 PID 集合。
func (a *App) SetSelectedCodexProcessPIDs(pids []int32) {
	a.processMu.Lock()
	defer a.processMu.Unlock()

	a.selectedPIDs = make(map[int32]struct{}, len(pids))
	for _, pid := range pids {
		if pid > 0 {
			a.selectedPIDs[pid] = struct{}{}
		}
	}
	appLogger.Info("Codex 进程选择状态已更新", "count", len(a.selectedPIDs))
}

// GetSelectedCodexProcessPIDs 返回当前内存中保存的 Codex Process 勾选 PID。
func (a *App) GetSelectedCodexProcessPIDs() []int32 {
	a.processMu.RLock()
	defer a.processMu.RUnlock()

	pids := make([]int32, 0, len(a.selectedPIDs))
	for pid := range a.selectedPIDs {
		pids = append(pids, pid)
	}
	sort.Slice(pids, func(i, j int) bool {
		return pids[i] < pids[j]
	})
	return pids
}

// InjectActiveAccountToCodexProcess 将当前激活账号写入指定 Codex 进程内存。
func (a *App) InjectActiveAccountToCodexProcess(pid int32) error {
	if err := a.ensureProxyService(); err != nil {
		appLogger.Error("注入 Codex 进程失败: 服务未初始化", "error", err, "pid", pid)
		return err
	}
	if pid <= 0 {
		return errors.New("Codex 进程 PID 无效")
	}

	record, ok, err := a.proxyStore.GetActiveAccountRecord()
	if err != nil {
		appLogger.Error("注入 Codex 进程失败: 查询激活账号失败", "error", err, "pid", pid)
		return err
	}
	if !ok {
		return errors.New("未找到当前激活账号")
	}

	if err := patchCodexProcessMemory(pid, record); err != nil {
		appLogger.Error("注入 Codex 进程失败", "error", err, "pid", pid, "account_id", record.AccountID, "email", record.Email)
		return err
	}
	appLogger.Info("注入 Codex 进程成功", "pid", pid, "account_id", record.AccountID, "email", record.Email)
	return nil
}

func (a *App) patchSelectedCodexProcesses(record accountRecord) error {
	pids := a.GetSelectedCodexProcessPIDs()
	if len(pids) == 0 {
		return nil
	}
	return patchCodexProcesses(pids, record)
}

func patchCodexProcesses(pids []int32, record accountRecord) error {
	var errs []error
	for _, pid := range pids {
		if pid <= 0 {
			continue
		}
		if err := patchCodexProcessMemory(pid, record); err != nil {
			errs = append(errs, fmt.Errorf("PID %d: %w", pid, err))
			continue
		}
		appLogger.Info("Codex 进程内存替换成功", "pid", pid, "account_id", record.AccountID, "email", record.Email)
	}
	if len(errs) > 0 {
		return fmt.Errorf("Codex 进程内存替换失败: %w", errors.Join(errs...))
	}
	return nil
}

func (a *App) clearSelectedCodexProcessPIDs() {
	a.processMu.Lock()
	defer a.processMu.Unlock()

	a.selectedPIDs = make(map[int32]struct{})
}
