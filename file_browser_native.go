package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

func (b *fileBrowser) openLocalDirectory(r *http.Request) error {
	var req struct{ Path string }
	if json.NewDecoder(r.Body).Decode(&req) != nil || !filepath.IsAbs(req.Path) || strings.ContainsFunc(req.Path, unicode.IsControl) {
		return errors.New("请选择已进入的本机目录")
	}
	// A folder-only action: never pass URLs, shell commands or file paths to an OS launcher.
	f, err := toolbox.BrowseOpen(req.Path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.IsDir() {
		return errors.New("只能在本地文件管理器中打开文件夹")
	}
	return b.openDirectory(f.Name())
}
