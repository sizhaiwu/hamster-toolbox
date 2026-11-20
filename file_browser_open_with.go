package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

var errOpenWithCancelled = errors.New("已取消选择程序")

func (b *fileBrowser) openLocalFileWith(r *http.Request) error {
	var req struct{ Path, Version, Program string }
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(&req) != nil || !filepath.IsAbs(req.Path) || strings.ContainsFunc(req.Path, unicode.IsControl) {
		return errors.New("请选择资源管理器中的本机文件")
	}
	if !b.openFileWithMu.TryLock() {
		return errors.New("已有打开方式窗口，请先完成或关闭它")
	}
	defer b.openFileWithMu.Unlock()
	if err := r.Context().Err(); err != nil {
		return err
	}
	b.mu.Lock()
	ref, ok := b.references[req.Version]
	b.mu.Unlock()
	if !ok || time.Since(ref.At) > 15*time.Minute {
		return errors.New("选择已过期，请刷新目录后重新选择")
	}
	if err := ref.Ref.Check(req.Path); err != nil {
		return err
	}
	st, err := os.Lstat(req.Path)
	if err != nil || !st.Mode().IsRegular() {
		return errors.New("打开方式只支持普通文件，请选择图片、视频或需要编辑的文件")
	}
	f, err := toolbox.BrowseOpen(req.Path)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err = f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return errors.New("打开方式只支持普通文件，请选择图片、视频或需要编辑的文件")
	}
	if err := ref.Ref.Check(f.Name()); err != nil {
		return err
	}
	if req.Program != "" {
		req.Program = strings.TrimSpace(req.Program)
		if len(req.Program) >= 2 && ((req.Program[0] == '"' && req.Program[len(req.Program)-1] == '"') || (req.Program[0] == '\'' && req.Program[len(req.Program)-1] == '\'')) {
			req.Program = req.Program[1 : len(req.Program)-1]
		}
		if req.Program == "" {
			return errors.New("请选择程序文件的完整路径")
		}
		if err := validateExternalProgram(req.Program); err != nil {
			return err
		}
	}
	return b.openFileWith(r.Context(), f.Name(), req.Program)
}

func validateExternalProgram(program string) error {
	if !filepath.IsAbs(program) || strings.ContainsFunc(program, unicode.IsControl) {
		return errors.New("请填写程序文件的完整路径，不要填写网址或命令参数")
	}
	st, err := os.Stat(program)
	if err != nil {
		return errors.New("找不到所选程序，请检查程序的完整路径")
	}
	if runtime.GOOS == "darwin" && st.IsDir() && strings.EqualFold(filepath.Ext(filepath.Clean(program)), ".app") {
		return nil
	}
	if !st.Mode().IsRegular() {
		return errors.New("请选择程序文件")
	}
	if runtime.GOOS == "windows" {
		if !strings.EqualFold(filepath.Ext(program), ".exe") {
			return errors.New("请选择 EXE 程序文件")
		}
	} else if st.Mode().Perm()&0111 == 0 {
		return errors.New("所选文件没有执行权限，请选择可执行程序")
	}
	return nil
}

func externalFileCommand(platform, program, file string) *exec.Cmd {
	if platform == "darwin" && strings.EqualFold(filepath.Ext(filepath.Clean(program)), ".app") {
		return exec.Command("/usr/bin/open", "-a", program, "--", file)
	}
	// Separate arguments are essential: filenames may contain spaces, quotes or '&'.
	return exec.Command(program, file)
}
func startExternalFile(program, file string) error {
	cmd := externalFileCommand(runtime.GOOS, program, file)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动所选程序失败：%w", err)
	}
	go func() { _ = cmd.Wait() }() // The application outlives this HTTP request.
	return nil
}
