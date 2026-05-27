//go:build !windows

package main

import "errors"

func scanCodexProcessesByName(processName string) ([]CodexProcessInfo, error) {
	return nil, errors.New("Codex 进程扫描仅支持 Windows")
}

func scanCodexProcessIDsByName(processName string) ([]int32, error) {
	return nil, nil
}
