package main

import (
	"encoding/json"
	"errors"
	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
	"net/http"
	"path/filepath"
)

func (b *fileBrowser) editText(w http.ResponseWriter, r *http.Request, local, save bool) {
	var req struct{ Path, Revision, Text string }
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		http.Error(w, "编辑请求无效或内容过大", 400)
		return
	}
	// Reads already verify file identity before returning. They must not wait
	// behind a multi-gigabyte copy/upload; writes retain serialization.
	if save {
		b.operations.Lock()
		defer b.operations.Unlock()
	}
	if r.Context().Err() != nil {
		return
	}
	p, _, err := b.resolve(r, local, true, req.Path)
	if err != nil {
		browserFail(w, local, err)
		return
	}
	var result toolbox.BrowserText
	backup := ""
	if save {
		historyRoot, e := filepath.Abs(filepath.Join(b.dataDir, "editor-backups"))
		if e != nil {
			browserFail(w, local, e)
			return
		}
		result, backup, err = toolbox.BrowserSaveText(r.Context(), p, req.Revision, req.Text, historyRoot)
	} else {
		result, err = toolbox.BrowserReadText(p)
	}
	if err != nil {
		if errors.Is(err, toolbox.ErrTextConflict) {
			http.Error(w, err.Error(), http.StatusConflict)
		} else if errors.Is(err, toolbox.ErrTextEncoding) || errors.Is(err, toolbox.ErrTextBinary) || errors.Is(err, toolbox.ErrTextTooLarge) {
			http.Error(w, err.Error(), 400)
		} else {
			browserFail(w, local, err)
		}
		return
	}
	if save {
		b.invalidateDirectoryListings()
	}
	if !local && backup != "" {
		backup = "已保存至服务端工具箱数据目录，由管理员管理"
	}
	w.Header().Set("Cache-Control", "no-store")
	reply := map[string]any{"Revision": result.Revision, "Encoding": result.Encoding, "Newline": result.Newline, "BOM": result.BOM, "Backup": backup}
	if !save {
		reply["Text"] = result.Text
	}
	send(w, reply)
}
