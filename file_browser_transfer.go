package main

import (
	"encoding/json"
	"errors"
	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

func (b *fileBrowser) transfer(w http.ResponseWriter, r *http.Request, local bool) {
	var req struct{ Action, Path, Target, Version, Conflict string }
	if json.NewDecoder(r.Body).Decode(&req) != nil || (req.Action != "copy" && req.Action != "move") || (req.Conflict != "" && req.Conflict != "skip" && req.Conflict != "rename" && req.Conflict != "overwrite") {
		fail(w, errors.New("传输参数无效"))
		return
	}
	b.operations.Lock()
	defer b.operations.Unlock()
	defer b.invalidateDirectoryListings()
	src, _, err := b.resolve(r, local, req.Action == "move", req.Path)
	if err != nil {
		browserFail(w, local, err)
		return
	}
	dst, _, err := b.resolve(r, local, true, req.Target)
	if err != nil {
		browserFail(w, local, err)
		return
	}
	b.mu.Lock()
	reference, ok := b.references[req.Version]
	b.mu.Unlock()
	if !ok || time.Since(reference.At) > 15*time.Minute {
		browserFail(w, local, errors.New("选择已过期，请刷新"))
		return
	}
	if err = reference.Ref.Check(src); err != nil {
		browserFail(w, local, err)
		return
	}
	if _, err = os.Lstat(dst); err == nil && req.Conflict == "" {
		w.WriteHeader(http.StatusConflict)
		send(w, map[string]any{"Conflict": true, "Message": "目标已有同名项目，请选择合并、替换或保留两份"})
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("X-Accel-Buffering", "no")
	encoder := json.NewEncoder(w)
	last := time.Time{}
	emit := func(value any) {
		encoder.Encode(value)
		if flush, ok := w.(http.Flusher); ok {
			flush.Flush()
		}
	}
	progress := func(done, total int64) {
		if time.Since(last) < 150*time.Millisecond && done != total {
			return
		}
		last = time.Now()
		emit(map[string]any{"Done": done, "Total": total})
	}
	var result toolbox.BrowserTransferResult
	if req.Conflict != "" {
		result, err = toolbox.BrowserMerge(r.Context(), src, dst, req.Action == "move", req.Conflict, progress)
	} else if req.Action == "move" {
		err = toolbox.BrowserMove(src, dst)
		if err != nil && (errors.Is(err, syscall.EXDEV) || strings.Contains(err.Error(), "跨磁盘") || runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(17))) {
			err = toolbox.BrowserCopy(r.Context(), src, dst, true, progress)
		}
	} else {
		err = toolbox.BrowserCopy(r.Context(), src, dst, false, progress)
	}
	if err != nil {
		message := err.Error()
		if !local {
			message = "传输未完成，请检查权限、目标冲突、源文件变化或剩余空间；已发布的副本请核对"
		}
		if result.Backup != "" && local {
			message += "；被替换的原文件备份：" + result.Backup
		}
		emit(map[string]any{"Error": message})
		return
	}
	backup := result.Backup
	if backup != "" && !local {
		if _, scope, e := b.resolve(r, false, false, req.Target); e == nil && scope != nil {
			if rel, e := filepath.Rel(scope.Root, backup); e == nil {
				backup = filepath.ToSlash(rel)
			} else {
				backup = "共享目录内的备份文件夹"
			}
		} else {
			backup = "共享目录内的备份文件夹"
		}
	}
	emit(map[string]any{"Finished": true, "Copied": result.Copied, "Skipped": result.Skipped, "Backup": backup})
}
