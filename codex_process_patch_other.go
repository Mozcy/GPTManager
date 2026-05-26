//go:build !windows

package main

import "errors"

func patchCodexProcessMemory(pid int32, record accountRecord) error {
	return errors.New("Codex 进程内存替换仅支持 Windows")
}
