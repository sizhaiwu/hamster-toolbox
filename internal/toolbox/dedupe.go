package toolbox

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const DedupeDownloadURL = "https://github.com/0x90d/videoduplicatefinder/releases"

type DedupeInfo struct {
	Path, Message, DownloadURL string
	Available                  bool
}

func dedupeExecutableName(name string) bool {
	for _, allowed := range []string{"VDF.GUI.exe", "VideoDuplicateFinder.exe", "VideoDuplicateFinderGUI.exe"} {
		if strings.EqualFold(name, allowed) {
			return true
		}
	}
	return false
}

func dedupePathAt(path, base string, restore bool) string {
	path = strings.Trim(strings.TrimSpace(path), `"`)
	if path == "" {
		return ""
	}
	if restore {
		if rel := portableSuffix(path); rel != "" {
			candidate := filepath.Join(base, rel)
			if within(filepath.Join(base, "tools"), candidate) && regularFile(candidate) {
				return candidate
			}
		}
	}
	if !filepath.IsAbs(path) {
		if found, e := exec.LookPath(path); e == nil {
			if absolute, e := filepath.Abs(found); e == nil {
				return absolute
			}
			return found
		}
		return filepath.Join(base, path)
	}
	return path
}

func detectDedupeAt(base string, roots []string) string {
	names := []string{"VDF.GUI.exe", "VideoDuplicateFinder.exe", "VideoDuplicateFinderGUI.exe"}
	find := func(dir string) string {
		for _, name := range names {
			p := filepath.Join(dir, name)
			if regularFile(p) {
				return p
			}
		}
		return ""
	}
	for _, rel := range []string{"tools/videoduplicatefinder", "tools/vdf", "VDF", "VideoDuplicateFinder"} {
		if p := find(filepath.Join(base, filepath.FromSlash(rel))); p != "" {
			return p
		}
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		if p := find(root); p != "" {
			return p
		}
		entries, e := os.ReadDir(root)
		if e != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			name := strings.ToLower(entry.Name())
			if !(strings.Contains(name, "video") && strings.Contains(name, "duplicate")) && !strings.Contains(name, "vdf") && !strings.HasPrefix(name, "gui-win-") {
				continue
			}
			dir := filepath.Join(root, entry.Name())
			if p := find(dir); p != "" {
				return p
			}
			children, _ := os.ReadDir(dir)
			for _, child := range children {
				if child.IsDir() && child.Type()&os.ModeSymlink == 0 {
					if p := find(filepath.Join(dir, child.Name())); p != "" {
						return p
					}
				}
			}
		}
	}
	return ""
}

func DetectDedupeProgram() string {
	base := executableDirectory()
	roots := []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		roots = append(roots, filepath.Join(local, "Programs"), filepath.Join(local, "VideoDuplicateFinder"))
	}
	if profile := os.Getenv("USERPROFILE"); profile != "" {
		roots = append(roots, filepath.Join(profile, "Downloads"))
	}
	if volume := filepath.VolumeName(base); volume != "" {
		roots = append(roots, filepath.Join(volume+string(filepath.Separator), "apps"))
	}
	if p := detectDedupeAt(base, roots); p != "" {
		return p
	}
	for _, name := range []string{"VDF.GUI.exe", "VideoDuplicateFinder.exe", "VideoDuplicateFinderGUI.exe"} {
		if p, e := exec.LookPath(name); e == nil {
			return p
		}
	}
	return ""
}

func InspectDedupeProgram(path string) DedupeInfo {
	if strings.TrimSpace(path) == "" {
		path = DetectDedupeProgram()
	}
	path = dedupePathAt(path, executableDirectory(), true)
	info := DedupeInfo{Path: path, DownloadURL: DedupeDownloadURL}
	if path == "" {
		info.Message = "尚未找到本地软件，请按下方步骤下载，或选择已解压的程序。"
		return info
	}
	if !dedupeExecutableName(filepath.Base(path)) {
		info.Message = "请选择 VDF.GUI.exe（旧版可选 VideoDuplicateFinder.exe），不要选择 CLI 或 Web 版本。"
		return info
	}
	if !regularFile(path) {
		info.Message = "程序文件不存在，请重新选择，或点击自动查找。"
		return info
	}
	info.Available = true
	info.Message = "已找到本地软件，可以打开。"
	return info
}

func DedupeLaunchCommand(path string) (*exec.Cmd, error) {
	info := InspectDedupeProgram(path)
	if !info.Available {
		return nil, errors.New(info.Message)
	}
	// A desktop GUI is intentionally visible. Do not use a request context:
	// closing the browser or toolbox must not terminate the external program.
	cmd := exec.Command(info.Path)
	cmd.Dir = filepath.Dir(info.Path)
	return cmd, nil
}
