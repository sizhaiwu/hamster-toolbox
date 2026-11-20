package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

// Read directory shortcuts only on demand. Listing never resolves targets or
// contacts disconnected drives. Mutation continues to operate on the .lnk file.
func (b *fileBrowser) directoryShortcut(w http.ResponseWriter, r *http.Request, local bool) {
	var req struct{ Path, Version string }
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(&req) != nil || !strings.EqualFold(filepath.Ext(req.Path), ".lnk") || strings.ContainsFunc(req.Path, unicode.IsControl) {
		browserFail(w, local, errors.New("请选择目录快捷方式（.lnk）"))
		return
	}
	p, scope, err := b.resolve(r, local, false, req.Path)
	if err != nil {
		browserFail(w, local, err)
		return
	}
	b.mu.Lock()
	ref, ok := b.references[req.Version]
	b.mu.Unlock()
	if !ok || time.Since(ref.At) > 15*time.Minute {
		browserFail(w, local, errors.New("选择已过期，请刷新目录后重新选择"))
		return
	}
	if err = ref.Ref.Check(p); err != nil {
		browserFail(w, local, err)
		return
	}
	f, err := toolbox.BrowseOpen(p)
	if err != nil {
		browserFail(w, local, err)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	var header [20]byte
	if err != nil || !st.Mode().IsRegular() || st.Size() > 1<<20 {
		browserFail(w, local, errors.New("快捷方式无效或过大"))
		return
	}
	_, err = io.ReadFull(f, header[:])
	// ShellLinkHeader size and CLSID_ShellLink; do not feed arbitrary files to COM.
	if err != nil || !bytes.Equal(header[:], []byte{0x4c, 0, 0, 0, 1, 0x14, 2, 0, 0, 0, 0, 0, 0xc0, 0, 0, 0, 0, 0, 0, 0x46}) {
		browserFail(w, local, errors.New("文件不是有效的 Windows 快捷方式"))
		return
	}
	target, err := readDirectoryShortcut(f.Name())
	if err == nil {
		err = ref.Ref.Check(f.Name())
	}
	if err == nil && (!filepath.IsAbs(target) || strings.ContainsFunc(target, unicode.IsControl)) {
		err = errors.New("快捷方式没有可访问的文件夹目标，不支持网址、程序或特殊系统入口")
	}
	display := target
	if err == nil && !local {
		// Check lexical scope before opening the target, including network paths.
		var relative string
		relative, err = filepath.Rel(scope.Root, target)
		if err == nil {
			display = filepath.ToSlash(relative)
			target, err = scope.Resolve(display, false)
		}
		if err == nil && display == "." {
			display = ""
		}
	}
	if err == nil {
		opened, openErr := toolbox.BrowseOpen(target)
		if openErr != nil {
			err = errors.New("快捷方式目标不可访问，请检查磁盘是否连接、目录是否已移动或删除")
		} else {
			info, statErr := opened.Stat()
			if statErr != nil || !info.IsDir() {
				err = errors.New("此快捷方式不是文件夹入口；不会启动程序或执行命令")
			}
			if local {
				display = opened.Name()
			}
			opened.Close()
		}
	}
	if err != nil {
		browserFail(w, local, err)
		return
	}
	if r.Context().Err() == nil {
		send(w, map[string]string{"Path": display})
	}
}
