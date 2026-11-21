package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type resumableUpload struct {
	ID, Scope, Target, Part, Fingerprint string
	Size                                 int64
	Created                              time.Time
}

func uploadScope(r *http.Request, local bool) string {
	if local {
		return "owner"
	}
	return "share:" + r.URL.Query().Get("root")
}
func (b *fileBrowser) uploadRecord(id string) (resumableUpload, error) {
	var u resumableUpload
	if len(id) != 48 {
		return u, errors.New("上传标识无效")
	}
	if _, err := hex.DecodeString(id); err != nil {
		return u, err
	}
	data, err := os.ReadFile(filepath.Join(b.dataDir, "uploads", id+".json"))
	if err != nil {
		return u, err
	}
	err = json.Unmarshal(data, &u)
	if u.ID != id {
		return u, errors.New("上传记录无效")
	}
	return u, err
}
func (b *fileBrowser) resumable(w http.ResponseWriter, r *http.Request, local bool, route string) {
	b.uploadMu.Lock()
	defer b.uploadMu.Unlock()
	b.operations.Lock()
	defer b.operations.Unlock()
	if route != "api/upload-chunk" {
		defer b.invalidateDirectoryListings()
	}
	if r.Context().Err() != nil {
		return
	}
	if route == "api/upload-init" {
		var req struct {
			Path, RelativePath, Sample string
			Directory                  bool
			Size, Modified             int64
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil || req.Size < 0 || req.Size > 32<<30 {
			fail(w, errors.New("上传参数无效或文件超过 32 GiB"))
			return
		}
		sample, err := base64.StdEncoding.DecodeString(req.Sample)
		if err != nil || len(sample) > 128<<10 {
			fail(w, errors.New("文件指纹无效"))
			return
		}
		relative := strings.ReplaceAll(req.RelativePath, "\\", "/")
		parts := strings.Split(relative, "/")
		for _, part := range parts {
			if !toolbox.BrowserName(part) {
				fail(w, errors.New("文件夹或文件名称无效"))
				return
			}
		}
		base, _, err := b.resolve(r, local, false, req.Path)
		if err != nil {
			browserFail(w, local, err)
			return
		}
		f, err := toolbox.BrowseOpen(base)
		if err != nil {
			browserFail(w, local, err)
			return
		}
		st, err := f.Stat()
		f.Close()
		if err != nil || !st.IsDir() {
			fail(w, errors.New("请选择上传目标目录"))
			return
		}
		targetLogical := req.Path
		for _, part := range parts {
			targetLogical = filepath.Join(targetLogical, part)
			if !local {
				targetLogical = filepath.ToSlash(targetLogical)
			}
		}
		target, _, err := b.resolve(r, local, true, targetLogical)
		if err != nil {
			browserFail(w, local, err)
			return
		}
		if _, err = os.Lstat(target); !req.Directory && !os.IsNotExist(err) {
			fail(w, errors.New("目标已存在，不会覆盖，请改名或选择其他目录"))
			return
		}
		sum := sha256.Sum256(append([]byte(fmt.Sprintf("%d|%d|", req.Size, req.Modified)), sample...))
		fingerprint := hex.EncodeToString(sum[:])
		scope := uploadScope(r, local)
		dir := filepath.Join(b.dataDir, "uploads")
		if err = os.MkdirAll(dir, 0700); err != nil {
			browserFail(w, local, err)
			return
		}
		records, _ := os.ReadDir(dir)
		activeRecords := 0
		for _, record := range records {
			if !strings.HasSuffix(record.Name(), ".json") {
				continue
			}
			old, e := b.uploadRecord(strings.TrimSuffix(record.Name(), ".json"))
			if e == nil && time.Since(old.Created) > 7*24*time.Hour {
				if st, err := os.Lstat(old.Part); err == nil && st.Mode().IsRegular() {
					if part, err := toolbox.BrowserUploadPart(old.Part, old.Target); err == nil {
						part.Close()
						os.Remove(old.Part)
					}
				}
				os.Remove(filepath.Join(dir, old.ID+".json"))
				continue
			}
			if e == nil {
				activeRecords++
			}
			if e != nil || old.Scope != scope || old.Target != target || old.Fingerprint != fingerprint || old.Size != req.Size || time.Since(old.Created) > 7*24*time.Hour {
				continue
			}
			part, e := toolbox.BrowserUploadPart(old.Part, target)
			if e != nil {
				continue
			}
			st, e := part.Stat()
			part.Close()
			if e == nil && st.Size() <= old.Size {
				send(w, map[string]any{"ID": old.ID, "Offset": st.Size(), "Size": old.Size})
				return
			}
		}
		if !req.Directory && activeRecords >= 1024 {
			fail(w, errors.New("未完成的上传过多，请先续传现有文件或等待过期清理"))
			return
		}
		parent := base
		directoryParts := parts[:len(parts)-1]
		if req.Directory {
			directoryParts = parts
		}
		for _, name := range directoryParts {
			parent = filepath.Join(parent, name)
			if st, e := os.Lstat(parent); e == nil {
				if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
					fail(w, errors.New("上传路径含链接或同名文件"))
					return
				}
			} else if e = toolbox.BrowserMkdir(parent); e != nil {
				browserFail(w, local, e)
				return
			}
		}
		if req.Directory {
			send(w, map[string]bool{"Directory": true})
			return
		}
		id := randomID()
		u := resumableUpload{id, scope, target, filepath.Join(filepath.Dir(target), ".toolbox-upload-"+id+".part"), fingerprint, req.Size, time.Now()}
		part, err := toolbox.BrowserUploadPart(u.Part, u.Target)
		if err != nil {
			browserFail(w, local, err)
			return
		}
		part.Close()
		data, _ := json.Marshal(u)
		if err = os.WriteFile(filepath.Join(dir, id+".json"), data, 0600); err != nil {
			os.Remove(u.Part)
			browserFail(w, local, err)
			return
		}
		send(w, map[string]any{"ID": id, "Offset": int64(0), "Size": u.Size})
		return
	}
	id := r.URL.Query().Get("id")
	u, err := b.uploadRecord(id)
	if err != nil || u.Scope != uploadScope(r, local) || time.Since(u.Created) > 7*24*time.Hour {
		fail(w, errors.New("上传已过期，请重新选择文件"))
		return
	}
	logical := u.Target
	if !local {
		b.mu.Lock()
		scope := b.scopes[r.URL.Query().Get("root")]
		b.mu.Unlock()
		if scope == nil {
			http.NotFound(w, r)
			return
		}
		rel, e := filepath.Rel(scope.Root, u.Target)
		if e != nil {
			fail(w, e)
			return
		}
		logical = filepath.ToSlash(rel)
	}
	target, _, err := b.resolve(r, local, true, logical)
	if err != nil || target != u.Target {
		browserFail(w, local, errors.New("上传权限或路径已变化"))
		return
	}
	part, err := toolbox.BrowserUploadPart(u.Part, target)
	if err != nil {
		browserFail(w, local, err)
		return
	}
	defer part.Close()
	st, err := part.Stat()
	if err != nil {
		browserFail(w, local, err)
		return
	}
	if route == "api/upload-chunk" {
		var offset int64
		if _, err = fmt.Sscan(r.URL.Query().Get("offset"), &offset); err != nil || offset != st.Size() || r.ContentLength < 0 || r.ContentLength > 4<<20 || offset+r.ContentLength > u.Size {
			fail(w, errors.New("上传位置不一致，请重新选择文件以续传"))
			return
		}
		part.Seek(offset, io.SeekStart)
		n, err := io.Copy(part, io.LimitReader(r.Body, (4<<20)+1))
		if err == nil {
			err = r.Context().Err()
		}
		if err != nil || n != r.ContentLength || n > 4<<20 {
			part.Truncate(offset)
			fail(w, errors.New("本段未完成，断点已保留"))
			return
		}
		if err = part.Sync(); err != nil {
			part.Truncate(offset)
			browserFail(w, local, err)
			return
		}
		send(w, map[string]any{"Offset": offset + n})
		return
	}
	if route != "api/upload-finish" || st.Size() != u.Size {
		fail(w, errors.New("文件尚未上传完整"))
		return
	}
	part.Close()
	if err = toolbox.BrowserMove(u.Part, u.Target); err != nil {
		browserFail(w, local, err)
		return
	}
	os.Remove(filepath.Join(b.dataDir, "uploads", u.ID+".json"))
	send(w, map[string]bool{"OK": true})
}
