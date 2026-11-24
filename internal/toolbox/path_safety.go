package toolbox

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type protectedPath struct{ path, reason string }

// CanonicalResourcePath resolves existing ancestors and Windows path aliases
// for coordinating tasks; it does not create or modify the resource.
func CanonicalResourcePath(path string) (string, error) { return canonicalPath(path) }

var extraProtectedPaths struct {
	sync.RWMutex
	paths []protectedPath
}

// Only fixed protection rules are cached. User input and execution paths are
// always resolved again so a newly introduced junction cannot use this cache.
var protectionPathCache sync.Map

func protectionPath(path string) string {
	if value, ok := protectionPathCache.Load(path); ok {
		return value.(string)
	}
	p, e := canonicalPath(path)
	if e != nil {
		p, _ = filepath.Abs(path)
	}
	protectionPathCache.Store(path, p)
	return p
}

// ProtectDataDirectory registers the running application's private data directory.
func ProtectDataDirectory(path string) {
	p, e := canonicalPath(path)
	if e != nil {
		return
	}
	extraProtectedPaths.Lock()
	defer extraProtectedPaths.Unlock()
	extraProtectedPaths.paths = append(extraProtectedPaths.paths, protectedPath{p, "工具箱数据目录"})
}

func canonicalPath(path string) (string, error) {
	p, e := filepath.Abs(path)
	if e != nil {
		return "", e
	}
	var tail []string
	for {
		if _, e = os.Lstat(p); e == nil {
			break
		}
		if !os.IsNotExist(e) {
			return "", e
		}
		parent := filepath.Dir(p)
		if parent == p {
			return "", e
		}
		tail = append(tail, filepath.Base(p))
		p = parent
	}
	p, e = physicalExistingPath(p)
	if e != nil {
		return "", e
	}
	for i := len(tail) - 1; i >= 0; i-- {
		p = filepath.Join(p, tail[i])
	}
	return filepath.Clean(p), nil
}

func pathContains(root, path string) bool {
	if runtime.GOOS == "windows" {
		root, path = strings.ToLower(root), strings.ToLower(path)
	}
	return within(root, path)
}

func protectedPathReason(path string) string {
	return protectedPathReasonMode(path, true)
}

func protectedPathReasonMode(path string, ancestors bool) string {
	return protectedPathReasonOptions(path, ancestors, true)
}
func protectedPathReasonOptions(path string, ancestors, allowTemporary bool) string {
	if filepath.Dir(path) == path {
		return "盘符根目录或共享根目录"
	}
	var protected []protectedPath
	tempRoot := protectionPath(os.TempDir())
	temporaryChild := allowTemporary && tempRoot != "" && pathContains(tempRoot, path) && !sameExtractPath(tempRoot, path)
	add := func(p, label string) {
		if p == "" {
			return
		}
		if runtime.GOOS == "darwin" && temporaryChild && label == "系统目录" && pathContains(protectionPath(p), tempRoot) {
			return
		}
		// Preserve both the actual location and its logical system path, even
		// when the rule points at an inaccessible directory.
		protected = append(protected, protectedPath{protectionPath(p), label})
		if abs, e := filepath.Abs(p); e == nil {
			protected = append(protected, protectedPath{abs, label})
		}
	}
	if runtime.GOOS == "windows" {
		win := systemDirectory()
		add(win, "Windows 系统目录")
		for _, p := range []string{"Program Files", "Program Files (x86)", "ProgramData", "Recovery", "Boot", "EFI", "Config.Msi", "$WINDOWS.~BT", "$WINDOWS.~WS", "Windows.old"} {
			add(filepath.Join(filepath.VolumeName(win)+`\`, p), "系统或程序目录")
		}
		for _, key := range []string{"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432", "ProgramData"} {
			add(os.Getenv(key), "系统或程序目录")
		}
		for _, p := range []string{"$Recycle.Bin", "System Volume Information", "Recovery", "Boot", "EFI", "Config.Msi", "pagefile.sys", "swapfile.sys", "hiberfil.sys", "bootmgr", "BOOTNXT"} {
			add(filepath.Join(filepath.VolumeName(path)+`\`, p), "磁盘系统文件或回收站")
		}
		volume := filepath.VolumeName(path)
		if strings.HasPrefix(volume, `\\`) && strings.HasSuffix(volume, "$") {
			return "管理共享"
		}
	} else {
		for _, p := range []string{"/etc", "/bin", "/sbin", "/usr", "/var", "/System", "/Library", "/dev", "/proc", "/sys", "/run", "/boot", "/root", "/private", "/Applications", "/opt", "/lost+found"} {
			add(p, "系统目录")
		}
	}
	if home, e := os.UserHomeDir(); e == nil {
		if h := protectionPath(home); h != "" {
			if ancestors && (sameExtractPath(path, h) || sameExtractPath(path, filepath.Dir(h))) {
				return "用户目录根目录"
			}
			if filepath.Dir(h) != filepath.Dir(filepath.Dir(h)) && pathContains(filepath.Dir(h), path) && !pathContains(h, path) && !sameExtractPath(path, filepath.Dir(h)) {
				return "其他用户或公共配置目录"
			}
			// Individual temporary resource folders are usable; the entire Temp is not.
			temp := protectionPath(os.TempDir())
			inTemp := allowTemporary && temp != "" && pathContains(temp, path) && !sameExtractPath(temp, path)
			if sameExtractPath(temp, path) {
				return "临时文件根目录"
			}
			if !inTemp {
				add(filepath.Join(h, "AppData"), "用户应用配置目录")
				add(filepath.Join(h, ".config"), "用户应用配置目录")
				if runtime.GOOS != "windows" {
					add(filepath.Join(h, "Library"), "用户应用配置目录")
					add(filepath.Join(h, ".local"), "用户应用配置目录")
				}
			}
			for _, p := range []string{"NTUSER.DAT", "ntuser.dat.LOG1", "ntuser.dat.LOG2", "ntuser.ini", ".ssh", ".aws", ".azure", ".gnupg", ".kube", ".codex", ".gitconfig"} {
				add(filepath.Join(h, p), "用户配置或凭据")
			}
		}
	}
	if exe, e := os.Executable(); e == nil {
		add(filepath.Dir(exe), "工具箱程序目录")
	}
	extraProtectedPaths.RLock()
	protected = append(protected, extraProtectedPaths.paths...)
	extraProtectedPaths.RUnlock()
	for _, rule := range protected {
		// Moving an ancestor would also move the protected directory.
		if pathContains(rule.path, path) || ancestors && pathContains(path, rule.path) {
			return rule.reason
		}
	}
	return ""
}

// Resolve ordinary filesystem paths without applying write/delete protection.
// Read-only inputs can be explicitly chosen outside the mutable resource area.
func resolveNonLinkPath(path string) (string, error) {
	return resolveOrdinaryPath(path, false)
}

// Root enumeration is only enabled by the read-only file browser. Existing
// resource inputs and every write/delete guard continue rejecting roots.
func resolveOrdinaryPath(path string, allowRoot bool) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("路径为空")
	}
	if runtime.GOOS == "windows" && (strings.HasPrefix(path, `\\.\`) || strings.HasPrefix(path, `\\?\`) || (len(path) >= 2 && path[1] == ':' && !filepath.IsAbs(path))) {
		return "", errors.New("请使用完整的普通目录路径，不支持设备路径或盘符相对路径")
	}
	p, e := filepath.Abs(path)
	if e != nil {
		return "", e
	}
	if runtime.GOOS == "darwin" {
		for _, alias := range []string{"/var", "/tmp", "/etc"} {
			if p == alias || strings.HasPrefix(p, alias+"/") {
				if target, e := os.Readlink(alias); e == nil {
					if !filepath.IsAbs(target) {
						target = filepath.Join(filepath.Dir(alias), target)
					}
					if filepath.Clean(target) == "/private"+alias {
						p = filepath.Clean(target) + strings.TrimPrefix(p, alias)
					}
				}
			}
		}
	}
	if runtime.GOOS == "windows" && (strings.HasPrefix(p, `\\.\`) || strings.HasPrefix(p, `\\?\`) || strings.Contains(strings.TrimPrefix(p, filepath.VolumeName(p)), ":")) {
		return "", errors.New("安全拦截：不支持设备路径或备用数据流路径")
	}
	if !allowRoot && filepath.Dir(p) == p {
		return "", errors.New("安全拦截：不允许处理盘符根目录或共享根目录，请选择具体的资源子目录")
	}
	if volume := filepath.VolumeName(p); runtime.GOOS == "windows" && strings.HasPrefix(volume, `\\`) && strings.HasSuffix(volume, "$") {
		return "", errors.New("安全拦截：不允许处理管理共享")
	}
	for current := p; ; current = filepath.Dir(current) {
		if pathRedirects(current) {
			return "", fmt.Errorf("安全拦截：路径包含目录链接，请选择真实目录：%s", current)
		}
		if filepath.Dir(current) == current {
			break
		}
	}
	physical, e := canonicalPath(p)
	if e != nil {
		return "", e
	}
	return physical, nil
}

func safePath(path string) (string, error) {
	physical, e := resolveNonLinkPath(path)
	if e != nil {
		return "", e
	}
	if reason := protectedPathReason(physical); reason != "" {
		return "", fmt.Errorf("安全拦截：不允许处理%s，请选择具体的资源子目录", reason)
	}
	return physical, nil
}

func unsafeEntry(path string, d fs.DirEntry) bool {
	if d.Type()&os.ModeSymlink != 0 || d.Type()&os.ModeIrregular != 0 && pathRedirects(path) {
		return true
	}
	if d.IsDir() {
		_, e := safePath(path)
		return e != nil
	}
	return false
}

type planSafety struct {
	root, output string
	rootID       fileIdentity
	outputID     *fileIdentity
	sources      map[string]fileIdentity
	targets      map[string]string
}

func capturePlanSafety(p *Plan) error {
	root, e := configRootPath(p.Config)
	if e != nil {
		return e
	}
	s := &planSafety{root: root, sources: map[string]fileIdentity{}, targets: map[string]string{}}
	s.rootID, e = snapshotFileIdentity(p.Config.Root)
	if e != nil {
		return e
	}
	if p.Config.Output != "" {
		s.output, e = safePath(p.Config.Output)
		if e != nil {
			return e
		}
		if _, statErr := os.Lstat(p.Config.Output); statErr == nil {
			id, e := snapshotFileIdentity(p.Config.Output)
			if e != nil {
				return e
			}
			s.outputID = &id
		}
	}
	for _, it := range p.Items {
		if it.Conflict != "" {
			continue
		}
		if e := validateItemPaths(p.Config, it); e != nil {
			return e
		}
		info, e := snapshotFileIdentity(it.Source)
		if e != nil {
			return e
		}
		s.sources[it.Source] = info
		s.targets[it.Target], e = safePath(it.Target)
		if e != nil {
			return e
		}
	}
	p.safety = s
	return nil
}

func validateItemPaths(c Config, it Item) error {
	if c.Tool == "extract" && c.ExtractSource != "" && !sameExtractPath(c.ExtractSource, it.Source) {
		return errors.New("所选压缩包与处理源不一致，请重新扫描")
	}
	if !pathContains(c.Root, it.Source) || sameExtractPath(c.Root, it.Source) {
		return errors.New("安全拦截：处理源超出输入目录")
	}
	if _, e := safePath(it.Source); e != nil {
		return e
	}
	if it.Action != "" {
		want := "recycle"
		if c.FormatDeleteMode == "permanent" {
			want = "delete"
		}
		if c.Tool != "format" || !c.FormatCleanOther || it.Action != want || !sameExtractPath(it.Source, it.Target) || formatMediaKind(it.Source) >= 0 {
			return errors.New("安全拦截：文件清理操作与本次预览设置不符")
		}
		info, e := os.Lstat(it.Source)
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return errors.New("安全拦截：文件清理仅允许普通文件")
		}
		if info.Size() != it.Size || !info.ModTime().Equal(it.Modified) {
			return errors.New("待清理文件已变化，请重新扫描预览")
		}
		return nil
	}
	if c.Output == "" {
		if !sameExtractPath(filepath.Dir(it.Source), filepath.Dir(it.Target)) || !validName(filepath.Base(it.Target)) {
			return errors.New("安全拦截：改名目标必须在原目录内")
		}
	} else if !pathContains(c.Output, it.Target) {
		return errors.New("安全拦截：处理目标超出输出范围")
	}
	return nil
}

func checkPlanSafety(p *Plan) error {
	if p.safety == nil {
		return capturePlanSafety(p)
	}
	root, e := configRootPath(p.Config)
	if e != nil {
		return e
	}
	if !sameExtractPath(root, p.safety.root) {
		return errors.New("输入路径已改变，请重新扫描预览")
	}
	rootID, e := snapshotFileIdentity(p.Config.Root)
	if e != nil {
		return e
	}
	if !sameFileIdentity(rootID, p.safety.rootID) {
		return errors.New("输入目录已被替换，请重新扫描预览")
	}
	if p.Config.Output != "" {
		output, e := safePath(p.Config.Output)
		if e != nil {
			return e
		}
		if !sameExtractPath(output, p.safety.output) {
			return errors.New("输出路径已改变，请重新扫描预览")
		}
		if p.safety.outputID != nil {
			id, e := snapshotFileIdentity(p.Config.Output)
			if e != nil {
				return e
			}
			if !sameFileIdentity(id, *p.safety.outputID) {
				return errors.New("输出位置已被替换，请重新扫描预览")
			}
		}
	}
	return nil
}

func checkItemSafety(p *Plan, it Item) error {
	if e := checkPlanSafety(p); e != nil {
		return e
	}
	if e := validateItemPaths(p.Config, it); e != nil {
		return e
	}
	info, e := snapshotFileIdentity(it.Source)
	if e != nil {
		return e
	}
	old, ok := p.safety.sources[it.Source]
	if !ok || !sameFileIdentity(old, info) {
		return errors.New("处理源已被替换，请重新扫描预览")
	}
	target, e := safePath(it.Target)
	if e != nil {
		return e
	}
	if !sameExtractPath(target, p.safety.targets[it.Target]) {
		return errors.New("目标路径已改变，请重新扫描预览")
	}
	return nil
}

func BuildPlan(ctx context.Context, c Config, log func(string)) (*Plan, error) {
	p, e := buildPlan(ctx, c, log)
	if e != nil {
		return nil, e
	}
	if e := capturePlanSafety(p); e != nil {
		return nil, e
	}
	return p, nil
}
