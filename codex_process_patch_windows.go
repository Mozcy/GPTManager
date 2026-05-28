//go:build windows

package main

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

type codexMemoryPatchSpec struct {
	name       string
	baseOffset uintptr
	offsets    []uintptr
	length     int
	value      []byte
}

type codexModuleEntry32 struct {
	Size         uint32
	ModuleID     uint32
	ProcessID    uint32
	GlblcntUsage uint32
	ProccntUsage uint32
	ModBaseAddr  uintptr
	ModBaseSize  uint32
	HModule      syscall.Handle
	SzModule     [256]uint16
	SzExePath    [260]uint16
}

type codexModuleInfo struct {
	name string
	path string
	base uintptr
	size uint32
}

var (
	codexMemoryKernel32                 = syscall.NewLazyDLL("kernel32.dll")
	codexMemoryOpenProcess              = codexMemoryKernel32.NewProc("OpenProcess")
	codexMemoryCloseHandle              = codexMemoryKernel32.NewProc("CloseHandle")
	codexMemoryReadProcessMemory        = codexMemoryKernel32.NewProc("ReadProcessMemory")
	codexMemoryWriteProcessMemory       = codexMemoryKernel32.NewProc("WriteProcessMemory")
	codexMemoryCreateToolhelp32Snapshot = codexMemoryKernel32.NewProc("CreateToolhelp32Snapshot")
	codexMemoryModule32FirstW           = codexMemoryKernel32.NewProc("Module32FirstW")
	codexMemoryModule32NextW            = codexMemoryKernel32.NewProc("Module32NextW")
)

const (
	codexMemoryProcessQueryInformation = 0x0400
	codexMemoryProcessVMOperation      = 0x0008
	codexMemoryProcessVMRead           = 0x0010
	codexMemoryProcessVMWrite          = 0x0020

	codexMemoryTH32CSSnapModule   = 0x00000008
	codexMemoryTH32CSSnapModule32 = 0x00000010
)

func patchCodexProcessMemory(pid int32, record accountRecord) error {
	accountBytes := []byte(record.AccountID)
	if len(accountBytes) != 36 {
		return fmt.Errorf("account_id 长度必须是 36 字节，当前是 %d", len(accountBytes))
	}

	handle, err := openCodexMemoryProcess(uint32(pid))
	if err != nil {
		return err
	}
	defer closeCodexMemoryHandle(handle)

	module, err := findCodexMemoryModuleBase(uint32(pid), "codex.exe")
	if err != nil {
		return err
	}

	profile, ctx, err := resolveCodexMemoryPatchProfile(pid, module.path)
	if err != nil {
		return err
	}

	for _, spec := range buildCodexMemoryPatchSpecs(profile, accountBytes) {
		if err := replaceCodexMemory(pid, handle, module.base, spec); err != nil {
			return err
		}
	}

	appLogger.Info(
		"Codex 进程内存替换完成",
		"pid", pid,
		"module", module.name,
		"module_path", module.path,
		"module_base", fmt.Sprintf("0x%X", module.base),
		"launcher", ctx.launcherName,
		"launcher_confidence", ctx.launcherConfidence,
		"profile", fmt.Sprintf("%s/%s", profile.launcher, profile.version),
	)
	return nil
}

func buildCodexMemoryPatchSpecs(profile codexMemoryPatchProfile, accountBytes []byte) []codexMemoryPatchSpec {
	specs := make([]codexMemoryPatchSpec, 0, len(profile.fields))
	for _, field := range profile.fields {
		spec := codexMemoryPatchSpec{
			name:       field.name,
			baseOffset: field.baseOffset,
			offsets:    field.offsets,
			length:     field.length,
		}
		switch field.name {
		case "account_id":
			spec.value = accountBytes
		case "access_token":
			spec.value = []byte("02")
		}
		specs = append(specs, spec)
	}
	return specs
}

func replaceCodexMemory(pid int32, handle syscall.Handle, moduleBase uintptr, spec codexMemoryPatchSpec) error {
	if spec.length <= 0 {
		return fmt.Errorf("%s 写入长度无效: %d", spec.name, spec.length)
	}
	if len(spec.value) != spec.length {
		return fmt.Errorf("%s 写入长度不匹配: value=%d length=%d", spec.name, len(spec.value), spec.length)
	}

	address, err := resolveCodexPointerChain(handle, moduleBase+spec.baseOffset, spec.offsets)
	if err != nil {
		return fmt.Errorf("%s 解析指针链失败: %w", spec.name, err)
	}

	original, err := readCodexMemory(handle, address, spec.length)
	if err != nil {
		return fmt.Errorf("%s 读取原始数据失败: %w", spec.name, err)
	}
	appLogger.Info(
		"查询进程",
		"pid", pid,
		"field", spec.name,
		"内存地址", fmt.Sprintf("0x%X", address),
		"数据", formatCodexMemoryBytes(original),
	)

	if err := writeCodexMemory(handle, address, spec.value); err != nil {
		return fmt.Errorf("%s 写入失败: %w", spec.name, err)
	}

	changed, err := readCodexMemory(handle, address, spec.length)
	if err != nil {
		return fmt.Errorf("%s 读取替换后数据失败: %w", spec.name, err)
	}
	if !bytes.Equal(changed, spec.value) {
		return fmt.Errorf("%s 写入校验失败", spec.name)
	}
	appLogger.Info(
		"修改进程",
		"pid", pid,
		"field", spec.name,
		"内存地址", fmt.Sprintf("0x%X", address),
		"数据", formatCodexMemoryBytes(changed),
	)
	return nil
}

func resolveCodexPointerChain(handle syscall.Handle, address uintptr, offsets []uintptr) (uintptr, error) {
	if len(offsets) == 0 {
		return address, nil
	}

	current := address
	for i, offset := range offsets {
		ptr, err := readCodexPointer(handle, current)
		if err != nil {
			return 0, fmt.Errorf("第 %d 层读取 0x%X 失败: %w", i+1, current, err)
		}
		current = ptr + offset
	}
	return current, nil
}

func readCodexPointer(handle syscall.Handle, address uintptr) (uintptr, error) {
	buf, err := readCodexMemory(handle, address, int(unsafe.Sizeof(uintptr(0))))
	if err != nil {
		return 0, err
	}

	var value uintptr
	for i, b := range buf {
		value |= uintptr(b) << (8 * i)
	}
	return value, nil
}

func openCodexMemoryProcess(processID uint32) (syscall.Handle, error) {
	access := uintptr(codexMemoryProcessQueryInformation | codexMemoryProcessVMOperation | codexMemoryProcessVMRead | codexMemoryProcessVMWrite)
	handle, _, callErr := codexMemoryOpenProcess.Call(access, 0, uintptr(processID))
	if handle == 0 {
		return 0, fmt.Errorf("OpenProcess(%d) 失败: %w", processID, codexMemorySyscallError(callErr))
	}
	return syscall.Handle(handle), nil
}

func readCodexMemory(handle syscall.Handle, address uintptr, length int) ([]byte, error) {
	if length <= 0 {
		return nil, fmt.Errorf("读取长度无效: %d", length)
	}

	buf := make([]byte, length)
	var read uintptr
	ok, _, callErr := codexMemoryReadProcessMemory.Call(
		uintptr(handle),
		address,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		uintptr(unsafe.Pointer(&read)),
	)
	if ok == 0 {
		return nil, codexMemorySyscallError(callErr)
	}
	if read != uintptr(length) {
		return nil, fmt.Errorf("只读取到 %d/%d 字节", read, length)
	}
	return buf, nil
}

func writeCodexMemory(handle syscall.Handle, address uintptr, data []byte) error {
	if len(data) == 0 {
		return errors.New("写入数据为空")
	}

	var written uintptr
	ok, _, callErr := codexMemoryWriteProcessMemory.Call(
		uintptr(handle),
		address,
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(len(data)),
		uintptr(unsafe.Pointer(&written)),
	)
	if ok == 0 {
		return codexMemorySyscallError(callErr)
	}
	if written != uintptr(len(data)) {
		return fmt.Errorf("只写入 %d/%d 字节", written, len(data))
	}
	return nil
}

func findCodexMemoryModuleBase(processID uint32, wantedName string) (codexModuleInfo, error) {
	snapshot, _, callErr := codexMemoryCreateToolhelp32Snapshot.Call(codexMemoryTH32CSSnapModule|codexMemoryTH32CSSnapModule32, uintptr(processID))
	if syscall.Handle(snapshot) == ^syscall.Handle(0) {
		return codexModuleInfo{}, fmt.Errorf("CreateToolhelp32Snapshot(%d) 失败: %w", processID, codexMemorySyscallError(callErr))
	}
	defer closeCodexMemoryHandle(syscall.Handle(snapshot))

	var entry codexModuleEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	ok, _, callErr := codexMemoryModule32FirstW.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
	if ok == 0 {
		return codexModuleInfo{}, fmt.Errorf("Module32FirstW 失败: %w", codexMemorySyscallError(callErr))
	}

	wantedName = strings.TrimSpace(wantedName)
	for {
		info := codexModuleInfo{
			name: syscall.UTF16ToString(entry.SzModule[:]),
			path: syscall.UTF16ToString(entry.SzExePath[:]),
			base: entry.ModBaseAddr,
			size: entry.ModBaseSize,
		}

		if wantedName == "" || strings.EqualFold(info.name, wantedName) {
			return info, nil
		}

		ok, _, callErr = codexMemoryModule32NextW.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
		if ok == 0 {
			break
		}
	}

	return codexModuleInfo{}, fmt.Errorf("未找到模块 %q", wantedName)
}

func closeCodexMemoryHandle(handle syscall.Handle) {
	if handle != 0 && handle != ^syscall.Handle(0) {
		codexMemoryCloseHandle.Call(uintptr(handle))
	}
}

func codexMemorySyscallError(err error) error {
	if err == nil || errors.Is(err, syscall.Errno(0)) {
		return syscall.EINVAL
	}
	return err
}

func formatCodexMemoryBytes(data []byte) string {
	return string(bytes.TrimRight(data, "\x00"))
}
