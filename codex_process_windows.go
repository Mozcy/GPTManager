//go:build windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"

	gnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

func scanCodexProcessesByName(processName string) ([]CodexProcessInfo, error) {
	procs, err := process.Processes()
	if err != nil {
		return nil, err
	}

	procMap := make(map[int32]*process.Process, len(procs))
	for _, p := range procs {
		if p != nil && p.Pid > 0 {
			procMap[p.Pid] = p
		}
	}

	rows := make([]CodexProcessInfo, 0)
	for _, p := range procs {
		name, err := p.Name()
		if err != nil || !strings.EqualFold(name, processName) {
			continue
		}
		rows = append(rows, collectCodexProcessInfo(p, name, procMap))
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].ProcessID < rows[j].ProcessID
	})
	return rows, nil
}

func scanCodexProcessIDsByName(processName string) ([]int32, error) {
	procs, err := process.Processes()
	if err != nil {
		return nil, err
	}

	pids := make([]int32, 0)
	for _, p := range procs {
		name, err := p.Name()
		if err != nil || !strings.EqualFold(name, processName) {
			continue
		}
		pids = append(pids, p.Pid)
	}
	return normalizeProcessIDs(pids), nil
}

func collectCodexProcessInfo(p *process.Process, name string, procMap map[int32]*process.Process) CodexProcessInfo {
	info := CodexProcessInfo{
		ProcessID: p.Pid,
		Name:      name,
	}

	info.CommandLine = getProcessString(p.Cmdline)
	info.ExecutablePath = getProcessString(p.Exe)
	info.Owner = getProcessString(p.Username)
	info.ParentProcessID = getProcessInt32(p.Ppid)
	info.Status = strings.Join(getProcessStringSlice(p.Status), ", ")
	info.ThreadCount = getProcessInt32(p.NumThreads)
	info.HandleCount = uint32(getProcessInt32(p.NumFDs))
	info.IsRunning = getProcessBoolPtr(p.IsRunning)
	info.Foreground = getProcessBoolPtr(p.Foreground)

	if ms, err := p.CreateTime(); err == nil && ms > 0 {
		info.CreationDate = time.Unix(0, ms*int64(time.Millisecond)).Local().Format("2006-01-02 15:04:05")
	}

	if parent, err := p.Parent(); err == nil && parent != nil {
		info.ParentName = getProcessString(parent.Name)
		info.ParentCommandLine = getProcessString(parent.Cmdline)
	}
	enrichCodexProcessLauncher(&info, p, procMap)

	if children, err := p.Children(); err == nil && len(children) > 0 {
		parts := make([]string, 0, len(children))
		for _, child := range children {
			childName := getProcessString(child.Name)
			if childName == "" {
				childName = "unknown"
			}
			parts = append(parts, fmt.Sprintf("%s(%d)", childName, child.Pid))
		}
		info.ChildProcesses = strings.Join(parts, "; ")
	}

	if mem, err := p.MemoryInfo(); err == nil && mem != nil {
		info.WorkingSetMB = processMBPtr(mem.RSS)
		info.VirtualSizeMB = processMBPtr(mem.VMS)
		info.PeakWorkingSetMB = processMBPtr(mem.HWM)
		info.DataMemoryMB = processMBPtr(mem.Data)
	}

	if ioStat, err := p.IOCounters(); err == nil && ioStat != nil {
		info.ReadCount = ioStat.ReadCount
		info.WriteCount = ioStat.WriteCount
		info.ReadBytesMB = processMBPtr(ioStat.ReadBytes)
		info.WriteBytesMB = processMBPtr(ioStat.WriteBytes)
	}

	if cpuPercent, err := p.CPUPercent(); err == nil {
		info.CPUPercent = processRoundPtr(cpuPercent)
	}

	if times, err := p.Times(); err == nil && times != nil {
		info.TotalCPUSeconds = processRoundPtr(times.User + times.System)
		info.UserModeTimeSec = processRoundPtr(times.User)
		info.KernelModeTimeSec = processRoundPtr(times.System)
	}

	if info.ExecutablePath != "" {
		enrichCodexProcessFileInfo(&info)
	}

	if conns, err := p.Connections(); err == nil {
		info.TCPConnections = formatCodexTCPConnections(conns)
	}

	return info
}

type codexProcessAncestor struct {
	pid            int32
	name           string
	executablePath string
	commandLine    string
}

type codexProcessLauncherMatch struct {
	displayName string
	confidence  string
}

var codexProcessLauncherNames = map[string]codexProcessLauncherMatch{
	"idea.exe":            {displayName: "IntelliJ IDEA", confidence: "high"},
	"idea64.exe":          {displayName: "IntelliJ IDEA", confidence: "high"},
	"goland.exe":          {displayName: "GoLand", confidence: "high"},
	"goland64.exe":        {displayName: "GoLand", confidence: "high"},
	"code.exe":            {displayName: "VS Code", confidence: "high"},
	"code - insiders.exe": {displayName: "VS Code Insiders", confidence: "high"},
	"codium.exe":          {displayName: "VSCodium", confidence: "high"},
	"vscodium.exe":        {displayName: "VSCodium", confidence: "high"},
	"cursor.exe":          {displayName: "Cursor", confidence: "high"},
	"windsurf.exe":        {displayName: "Windsurf", confidence: "high"},
	"trae.exe":            {displayName: "Trae", confidence: "high"},
	"pycharm.exe":         {displayName: "PyCharm", confidence: "high"},
	"pycharm64.exe":       {displayName: "PyCharm", confidence: "high"},
	"webstorm.exe":        {displayName: "WebStorm", confidence: "high"},
	"webstorm64.exe":      {displayName: "WebStorm", confidence: "high"},
	"rider.exe":           {displayName: "Rider", confidence: "high"},
	"rider64.exe":         {displayName: "Rider", confidence: "high"},
	"clion.exe":           {displayName: "CLion", confidence: "high"},
	"clion64.exe":         {displayName: "CLion", confidence: "high"},
	"phpstorm.exe":        {displayName: "PhpStorm", confidence: "high"},
	"phpstorm64.exe":      {displayName: "PhpStorm", confidence: "high"},
	"rubymine.exe":        {displayName: "RubyMine", confidence: "high"},
	"rubymine64.exe":      {displayName: "RubyMine", confidence: "high"},
}

var codexProcessFallbackLauncherNames = map[string]codexProcessLauncherMatch{
	"windowsterminal.exe":      {displayName: "Windows Terminal", confidence: "medium"},
	"openterminal.exe":         {displayName: "Windows Terminal", confidence: "medium"},
	"openconsole.exe":          {displayName: "Console Host", confidence: "medium"},
	"conhost.exe":              {displayName: "Console Host", confidence: "medium"},
	"powershell.exe":           {displayName: "PowerShell", confidence: "medium"},
	"pwsh.exe":                 {displayName: "PowerShell", confidence: "medium"},
	"cmd.exe":                  {displayName: "Command Prompt", confidence: "medium"},
	"explorer.exe":             {displayName: "Windows Shell", confidence: "medium"},
	"applicationframehost.exe": {displayName: "Windows App Host", confidence: "medium"},
}

func enrichCodexProcessLauncher(info *CodexProcessInfo, p *process.Process, procMap map[int32]*process.Process) {
	ancestors := collectCodexProcessAncestors(p, procMap, 12)
	info.ProcessTree = formatCodexProcessTree(info.ProcessID, info.Name, ancestors)

	for _, ancestor := range ancestors {
		match, ok := matchCodexProcessLauncher(ancestor.name, ancestor.executablePath)
		if !ok {
			continue
		}
		info.LauncherName = match.displayName
		info.LauncherPID = ancestor.pid
		info.LauncherPath = ancestor.executablePath
		info.LauncherCommandLine = ancestor.commandLine
		info.LauncherConfidence = match.confidence
		return
	}
	if match, launcher, evidence, ok := matchCodexProcessLauncherFromEnv(p); ok {
		info.LauncherName = match.displayName
		info.LauncherPID = launcher.pid
		info.LauncherPath = launcher.executablePath
		info.LauncherCommandLine = firstNonEmpty(launcher.commandLine, evidence)
		info.LauncherConfidence = match.confidence
		return
	}
	if match, launcher, ok := matchCodexProcessFallbackLauncher(info, ancestors); ok {
		info.LauncherName = match.displayName
		info.LauncherPID = launcher.pid
		info.LauncherPath = launcher.executablePath
		info.LauncherCommandLine = launcher.commandLine
		info.LauncherConfidence = match.confidence
		return
	}
	info.LauncherConfidence = "low"
}

func collectCodexProcessAncestors(p *process.Process, procMap map[int32]*process.Process, maxDepth int) []codexProcessAncestor {
	ancestors := make([]codexProcessAncestor, 0, maxDepth)
	seen := map[int32]struct{}{
		p.Pid: {},
	}
	current := p
	for depth := 0; depth < maxDepth; depth++ {
		ppid := getProcessInt32(current.Ppid)
		if ppid <= 0 {
			break
		}
		if _, ok := seen[ppid]; ok {
			break
		}
		seen[ppid] = struct{}{}

		parent := procMap[ppid]
		if parent == nil {
			var err error
			parent, err = process.NewProcess(ppid)
			if err != nil || parent == nil {
				break
			}
		}

		ancestors = append(ancestors, codexProcessAncestor{
			pid:            parent.Pid,
			name:           getProcessString(parent.Name),
			executablePath: getProcessString(parent.Exe),
			commandLine:    getProcessString(parent.Cmdline),
		})
		current = parent
	}
	return ancestors
}

func matchCodexProcessLauncher(name string, path string) (codexProcessLauncherMatch, bool) {
	candidates := []string{name}
	if path != "" {
		parts := strings.FieldsFunc(path, func(r rune) bool {
			return r == '\\' || r == '/'
		})
		if len(parts) > 0 {
			candidates = append(candidates, parts[len(parts)-1])
		}
	}
	for _, candidate := range candidates {
		if match, ok := codexProcessLauncherNames[strings.ToLower(strings.TrimSpace(candidate))]; ok {
			return match, true
		}
	}
	return codexProcessLauncherMatch{}, false
}

func matchCodexProcessFallbackLauncher(info *CodexProcessInfo, ancestors []codexProcessAncestor) (codexProcessLauncherMatch, codexProcessAncestor, bool) {
	if isCodexProcessMicrosoftStorePath(info.ExecutablePath) || isCodexProcessMicrosoftStorePath(info.CommandLine) {
		return codexProcessLauncherMatch{displayName: "Microsoft Store Codex", confidence: "medium"}, codexProcessAncestor{
			pid:            info.ProcessID,
			name:           info.Name,
			executablePath: info.ExecutablePath,
			commandLine:    firstNonEmpty(info.CommandLine, info.ExecutablePath),
		}, true
	}
	for _, ancestor := range ancestors {
		normalized := strings.ToLower(strings.TrimSpace(ancestor.name))
		if match, ok := codexProcessFallbackLauncherNames[normalized]; ok {
			return match, ancestor, true
		}
		if isCodexProcessMicrosoftStorePath(ancestor.executablePath) || isCodexProcessMicrosoftStorePath(ancestor.commandLine) {
			return codexProcessLauncherMatch{displayName: "Microsoft Store / WindowsApps", confidence: "medium"}, ancestor, true
		}
	}
	return codexProcessLauncherMatch{}, codexProcessAncestor{}, false
}

func isCodexProcessMicrosoftStorePath(value string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(value, "/", "\\"))
	return strings.Contains(normalized, "\\windowsapps\\") ||
		strings.Contains(normalized, "\\microsoft\\windowsapps\\") ||
		strings.Contains(normalized, "microsoft.windowsapps")
}

func matchCodexProcessLauncherFromEnv(p *process.Process) (codexProcessLauncherMatch, codexProcessAncestor, string, bool) {
	env, err := p.Environ()
	if err != nil || len(env) == 0 {
		return codexProcessLauncherMatch{}, codexProcessAncestor{}, "", false
	}

	envMap := make(map[string]string, len(env))
	for _, item := range env {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		envMap[strings.ToUpper(strings.TrimSpace(key))] = value
	}

	if key, ok := findEnvKeyWithPrefix(envMap, "CURSOR_"); ok {
		return codexProcessLauncherMatch{displayName: "Cursor", confidence: "medium"}, codexProcessAncestor{}, "环境变量: " + key, true
	}
	if strings.EqualFold(envMap["TERM_PROGRAM"], "vscode") {
		return codexProcessLauncherMatch{displayName: "VS Code", confidence: "medium"}, codexProcessAncestor{}, "环境变量: TERM_PROGRAM=vscode", true
	}
	if key, ok := findEnvKeyWithPrefix(envMap, "VSCODE_"); ok {
		return codexProcessLauncherMatch{displayName: "VS Code", confidence: "medium"}, codexProcessAncestor{}, "环境变量: " + key, true
	}
	if strings.Contains(strings.ToLower(envMap["TERMINAL_EMULATOR"]), "jetbrains") {
		return codexProcessLauncherMatch{displayName: "JetBrains Terminal", confidence: "medium"}, codexProcessAncestor{}, "环境变量: TERMINAL_EMULATOR=" + envMap["TERMINAL_EMULATOR"], true
	}
	if key, ok := findEnvKeyWithPrefix(envMap, "__INTELLIJ_"); ok {
		return codexProcessLauncherMatch{displayName: "JetBrains Terminal", confidence: "medium"}, codexProcessAncestor{}, "环境变量: " + key, true
	}
	if _, ok := envMap["WT_SESSION"]; ok {
		return codexProcessLauncherMatch{displayName: "Windows Terminal", confidence: "medium"}, codexProcessAncestor{}, "环境变量: WT_SESSION", true
	}
	return codexProcessLauncherMatch{}, codexProcessAncestor{}, "", false
}

func findEnvKeyWithPrefix(envMap map[string]string, prefix string) (string, bool) {
	keys := make([]string, 0, len(envMap))
	for key := range envMap {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return "", false
	}
	sort.Strings(keys)
	return keys[0], true
}

func formatCodexProcessTree(pid int32, name string, ancestors []codexProcessAncestor) string {
	parts := []string{formatCodexProcessNode(name, pid)}
	for _, ancestor := range ancestors {
		parts = append(parts, formatCodexProcessNode(ancestor.name, ancestor.pid))
	}
	return strings.Join(parts, " <- ")
}

func formatCodexProcessNode(name string, pid int32) string {
	if name == "" {
		name = "unknown"
	}
	return fmt.Sprintf("%s(%d)", name, pid)
}

func enrichCodexProcessFileInfo(info *CodexProcessInfo) {
	file, err := os.Open(info.ExecutablePath)
	if err == nil {
		defer file.Close()
		hash := sha256.New()
		if _, err := io.Copy(hash, file); err == nil {
			info.SHA256 = strings.ToUpper(hex.EncodeToString(hash.Sum(nil)))
		}
	}

	if stat, err := os.Stat(info.ExecutablePath); err == nil {
		info.FileSizeMB = processMBPtr(uint64(stat.Size()))
		info.FileModified = stat.ModTime().Format("2006-01-02 15:04:05")
	}

	if created, ok := codexProcessFileCreationTime(info.ExecutablePath); ok {
		info.FileCreated = created.Format("2006-01-02 15:04:05")
	}

	version := readCodexProcessFileVersionInfo(info.ExecutablePath)
	info.FileProductName = version.ProductName
	info.FileProductVersion = version.ProductVersion
	info.FileVersion = version.FileVersion
	info.FileCompany = version.CompanyName
	info.FileDescription = version.FileDescription
}

func formatCodexTCPConnections(conns []gnet.ConnectionStat) string {
	parts := make([]string, 0, len(conns))
	for _, c := range conns {
		if c.Type != syscall.SOCK_STREAM {
			continue
		}
		remote := "-"
		if c.Raddr.IP != "" || c.Raddr.Port != 0 {
			remote = fmt.Sprintf("%s:%d", c.Raddr.IP, c.Raddr.Port)
		}
		parts = append(parts, fmt.Sprintf("%s %s:%d -> %s", c.Status, c.Laddr.IP, c.Laddr.Port, remote))
	}
	return strings.Join(parts, "; ")
}

func getProcessString(fn func() (string, error)) string {
	v, err := fn()
	if err != nil {
		return ""
	}
	return v
}

func getProcessStringSlice(fn func() ([]string, error)) []string {
	v, err := fn()
	if err != nil {
		return nil
	}
	return v
}

func getProcessInt32(fn func() (int32, error)) int32 {
	v, err := fn()
	if err != nil {
		return 0
	}
	return v
}

func getProcessBoolPtr(fn func() (bool, error)) *bool {
	v, err := fn()
	if err != nil {
		return nil
	}
	return &v
}

func processMBPtr(bytes uint64) *float64 {
	return processRoundPtr(float64(bytes) / 1024 / 1024)
}

func processRoundPtr(v float64) *float64 {
	rounded := float64(int64(v*1000+0.5)) / 1000
	return &rounded
}

var (
	codexProcessKernel32            = syscall.NewLazyDLL("kernel32.dll")
	codexProcessVersionDLL          = syscall.NewLazyDLL("version.dll")
	codexProcessCloseHandle         = codexProcessKernel32.NewProc("CloseHandle")
	codexProcessCreateFileW         = codexProcessKernel32.NewProc("CreateFileW")
	codexProcessGetFileTime         = codexProcessKernel32.NewProc("GetFileTime")
	codexProcessGetFileVersionSizeW = codexProcessVersionDLL.NewProc("GetFileVersionInfoSizeW")
	codexProcessGetFileVersionInfoW = codexProcessVersionDLL.NewProc("GetFileVersionInfoW")
	codexProcessVerQueryValueW      = codexProcessVersionDLL.NewProc("VerQueryValueW")
)

func codexProcessFileCreationTime(path string) (time.Time, bool) {
	const (
		genericRead       = 0x80000000
		fileShareRead     = 0x00000001
		fileShareWrite    = 0x00000002
		fileShareDelete   = 0x00000004
		openExisting      = 3
		fileAttributeNorm = 0x00000080
	)

	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return time.Time{}, false
	}

	handle, _, _ := codexProcessCreateFileW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		genericRead,
		fileShareRead|fileShareWrite|fileShareDelete,
		0,
		openExisting,
		fileAttributeNorm,
		0,
	)
	if handle == 0 || handle == ^uintptr(0) {
		return time.Time{}, false
	}
	defer codexProcessCloseHandle.Call(handle)

	var created syscall.Filetime
	ok, _, _ := codexProcessGetFileTime.Call(handle, uintptr(unsafe.Pointer(&created)), 0, 0)
	if ok == 0 {
		return time.Time{}, false
	}
	return time.Unix(0, created.Nanoseconds()).Local(), true
}

type codexProcessVersionInfo struct {
	ProductName     string
	ProductVersion  string
	FileVersion     string
	CompanyName     string
	FileDescription string
}

type codexProcessFixedFileInfo struct {
	Signature        uint32
	StrucVersion     uint32
	FileVersionMS    uint32
	FileVersionLS    uint32
	ProductVersionMS uint32
	ProductVersionLS uint32
	FileFlagsMask    uint32
	FileFlags        uint32
	FileOS           uint32
	FileType         uint32
	FileSubtype      uint32
	FileDateMS       uint32
	FileDateLS       uint32
}

type codexProcessTranslation struct {
	Language uint16
	CodePage uint16
}

func readCodexProcessFileVersionInfo(path string) codexProcessVersionInfo {
	var info codexProcessVersionInfo

	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return info
	}

	size, _, _ := codexProcessGetFileVersionSizeW.Call(uintptr(unsafe.Pointer(pathPtr)), 0)
	if size == 0 {
		return info
	}

	data := make([]byte, int(size))
	ok, _, _ := codexProcessGetFileVersionInfoW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		0,
		size,
		uintptr(unsafe.Pointer(&data[0])),
	)
	if ok == 0 {
		return info
	}

	if fixed, ok := queryCodexProcessFixedFileInfo(data); ok {
		info.FileVersion = codexProcessVersionFromParts(fixed.FileVersionMS, fixed.FileVersionLS)
		info.ProductVersion = codexProcessVersionFromParts(fixed.ProductVersionMS, fixed.ProductVersionLS)
	}

	lang, codePage := queryCodexProcessTranslation(data)
	for _, key := range []struct {
		name string
		dest *string
	}{
		{"ProductName", &info.ProductName},
		{"ProductVersion", &info.ProductVersion},
		{"FileVersion", &info.FileVersion},
		{"CompanyName", &info.CompanyName},
		{"FileDescription", &info.FileDescription},
	} {
		if v := queryCodexProcessVersionString(data, lang, codePage, key.name); v != "" {
			*key.dest = v
		}
	}
	return info
}

func queryCodexProcessFixedFileInfo(data []byte) (*codexProcessFixedFileInfo, bool) {
	var ptr uintptr
	var size uint32
	subBlock, _ := syscall.UTF16PtrFromString(`\`)
	ok, _, _ := codexProcessVerQueryValueW.Call(
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(unsafe.Pointer(subBlock)),
		uintptr(unsafe.Pointer(&ptr)),
		uintptr(unsafe.Pointer(&size)),
	)
	if ok == 0 || ptr == 0 || size < uint32(unsafe.Sizeof(codexProcessFixedFileInfo{})) {
		return nil, false
	}
	return (*codexProcessFixedFileInfo)(unsafe.Pointer(ptr)), true
}

func queryCodexProcessTranslation(data []byte) (uint16, uint16) {
	var ptr uintptr
	var size uint32
	subBlock, _ := syscall.UTF16PtrFromString(`\VarFileInfo\Translation`)
	ok, _, _ := codexProcessVerQueryValueW.Call(
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(unsafe.Pointer(subBlock)),
		uintptr(unsafe.Pointer(&ptr)),
		uintptr(unsafe.Pointer(&size)),
	)
	if ok == 0 || ptr == 0 || size < uint32(unsafe.Sizeof(codexProcessTranslation{})) {
		return 0x0409, 0x04b0
	}
	t := (*codexProcessTranslation)(unsafe.Pointer(ptr))
	return t.Language, t.CodePage
}

func queryCodexProcessVersionString(data []byte, lang uint16, codePage uint16, name string) string {
	subBlock := fmt.Sprintf(`\StringFileInfo\%04x%04x\%s`, lang, codePage, name)
	subBlockPtr, err := syscall.UTF16PtrFromString(subBlock)
	if err != nil {
		return ""
	}

	var ptr uintptr
	var length uint32
	ok, _, _ := codexProcessVerQueryValueW.Call(
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(unsafe.Pointer(subBlockPtr)),
		uintptr(unsafe.Pointer(&ptr)),
		uintptr(unsafe.Pointer(&length)),
	)
	if ok == 0 || ptr == 0 || length == 0 {
		return ""
	}

	chars := unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), length)
	return syscall.UTF16ToString(chars)
}

func codexProcessVersionFromParts(ms uint32, ls uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d", ms>>16, ms&0xffff, ls>>16, ls&0xffff)
}
