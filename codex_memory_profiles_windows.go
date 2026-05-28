//go:build windows

package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/shirou/gopsutil/v4/process"
)

type codexMemoryLauncherKind string

const (
	codexMemoryLauncherVSCode    codexMemoryLauncherKind = "vscode"
	codexMemoryLauncherJetBrains codexMemoryLauncherKind = "jetbrains"
)

type codexMemoryPatchField struct {
	name       string
	baseOffset uintptr
	offsets    []uintptr
	length     int
}

type codexMemoryPatchProfile struct {
	launcher codexMemoryLauncherKind
	version  string
	fields   []codexMemoryPatchField
}

type codexMemoryPatchContext struct {
	launcherName       string
	launcherConfidence string
	launcherKind       codexMemoryLauncherKind
	version            string
	versionCandidates  []string
}

var codexMemoryPatchProfiles = []codexMemoryPatchProfile{
	{
		launcher: codexMemoryLauncherVSCode,
		version:  "0.133.0-alpha.1",
		fields: []codexMemoryPatchField{
			{
				name:       "account_id",
				baseOffset: 0x0E4ED808,
				offsets:    []uintptr{0x20, 0x80, 0x380, 0x58, 0x118, 0xE8, 0x0},
				length:     36,
			},
			{
				name:       "access_token",
				baseOffset: 0x0E4F4000,
				offsets:    []uintptr{0xE0, 0x670, 0x278, 0xA20, 0x118, 0xB8, 0x0},
				length:     2,
			},
		},
	},
	{
		launcher: codexMemoryLauncherJetBrains,
		version:  "0.128.0",
		fields: []codexMemoryPatchField{
			{
				name:       "account_id",
				baseOffset: 0x0E299648,
				offsets:    []uintptr{0xD8, 0x80, 0x7B0, 0xD98, 0x128, 0x150, 0x0},
				length:     36,
			},
			{
				name:       "access_token",
				baseOffset: 0x0E299648,
				offsets:    []uintptr{0x240, 0x1F0, 0x108, 0x58, 0x118, 0xB8, 0x0},
				length:     2,
			},
		},
	},
}

func resolveCodexMemoryPatchProfile(pid int32, modulePath string) (codexMemoryPatchProfile, codexMemoryPatchContext, error) {
	ctx, err := buildCodexMemoryPatchContext(pid, modulePath)
	if err != nil {
		return codexMemoryPatchProfile{}, ctx, err
	}
	if ctx.launcherKind == "" {
		return codexMemoryPatchProfile{}, ctx, fmt.Errorf("不支持的 Codex 启动来源: %s", firstNonEmpty(ctx.launcherName, "未知"))
	}

	for _, profile := range codexMemoryPatchProfiles {
		if profile.launcher != ctx.launcherKind {
			continue
		}
		for _, candidate := range ctx.versionCandidates {
			if strings.EqualFold(profile.version, candidate) {
				ctx.version = candidate
				return profile, ctx, nil
			}
		}
	}
	return codexMemoryPatchProfile{}, ctx, fmt.Errorf(
		"未找到 %s 的 Codex %s 偏移配置，已支持: %s",
		ctx.launcherKind,
		firstNonEmpty(ctx.version, strings.Join(ctx.versionCandidates, "/"), "未知版本"),
		formatCodexMemorySupportedProfiles(ctx.launcherKind),
	)
}

func buildCodexMemoryPatchContext(pid int32, modulePath string) (codexMemoryPatchContext, error) {
	ctx := codexMemoryPatchContext{}

	procMap, proc, err := codexMemoryProcessMap(pid)
	if err != nil {
		return ctx, err
	}

	info := CodexProcessInfo{
		ProcessID:      pid,
		Name:           getProcessString(proc.Name),
		ExecutablePath: modulePath,
	}
	info.CommandLine = getProcessString(proc.Cmdline)
	enrichCodexProcessLauncher(&info, proc, procMap)

	ctx.launcherName = info.LauncherName
	ctx.launcherConfidence = info.LauncherConfidence
	ctx.launcherKind = codexMemoryLauncherKindFromName(info.LauncherName)
	ctx.versionCandidates = codexMemoryVersionCandidates(modulePath)
	if len(ctx.versionCandidates) > 0 {
		ctx.version = ctx.versionCandidates[0]
	}
	if len(ctx.versionCandidates) == 0 {
		return ctx, fmt.Errorf("无法读取 Codex 文件版本: %s", modulePath)
	}
	return ctx, nil
}

func codexMemoryProcessMap(pid int32) (map[int32]*process.Process, *process.Process, error) {
	procs, err := process.Processes()
	if err != nil {
		return nil, nil, err
	}

	procMap := make(map[int32]*process.Process, len(procs))
	for _, p := range procs {
		if p != nil && p.Pid > 0 {
			procMap[p.Pid] = p
		}
	}

	proc := procMap[pid]
	if proc == nil {
		proc, err = process.NewProcess(pid)
		if err != nil {
			return nil, nil, fmt.Errorf("未找到 Codex 进程 %d: %w", pid, err)
		}
		procMap[pid] = proc
	}
	return procMap, proc, nil
}

func codexMemoryLauncherKindFromName(name string) codexMemoryLauncherKind {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "vs code", "vs code insiders", "vscodium":
		return codexMemoryLauncherVSCode
	case "intellij idea", "goland", "pycharm", "webstorm", "rider", "clion", "phpstorm", "rubymine", "jetbrains terminal":
		return codexMemoryLauncherJetBrains
	default:
		return ""
	}
}

func codexMemoryVersionCandidates(path string) []string {
	version := readCodexProcessFileVersionInfo(path)
	values := []string{
		version.ProductVersion,
		version.FileVersion,
	}

	seen := make(map[string]struct{}, len(values))
	candidates := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.TrimSpace(strings.TrimPrefix(value, "v"))
		if normalized == "" {
			continue
		}
		key := strings.ToLower(normalized)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		candidates = append(candidates, normalized)
	}
	return candidates
}

func formatCodexMemorySupportedProfiles(launcher codexMemoryLauncherKind) string {
	versions := make([]string, 0)
	for _, profile := range codexMemoryPatchProfiles {
		if launcher != "" && profile.launcher != launcher {
			continue
		}
		versions = append(versions, fmt.Sprintf("%s/%s", profile.launcher, profile.version))
	}
	sort.Strings(versions)
	if len(versions) == 0 {
		return "无"
	}
	return strings.Join(versions, ", ")
}
