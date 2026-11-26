package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

func (b *fileBrowser) checkPropertyReference(path, version string) error {
	if version == "" {
		return nil
	}
	b.mu.Lock()
	ref, ok := b.references[version]
	b.mu.Unlock()
	if !ok || time.Since(ref.At) > 15*time.Minute {
		return errors.New("选择已过期，请刷新目录后重新选择")
	}
	return ref.Ref.Check(path)
}

func (b *fileBrowser) resourceDirectorySize(w http.ResponseWriter, r *http.Request, local bool) {
	var req struct{ Path, Version string }
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(&req) != nil {
		browserFail(w, local, errors.New("目录统计请求无效"))
		return
	}
	p, scope, err := b.resolve(r, local, false, req.Path)
	if err == nil {
		err = b.checkPropertyReference(p, req.Version)
	}
	if err != nil {
		browserFail(w, local, err)
		return
	}
	b.mu.Lock()
	if b.directorySizeRunning {
		b.mu.Unlock()
		browserFail(w, local, errors.New("已有目录正在计算大小，请稍后重试"))
		return
	}
	b.directorySizeRunning = true
	b.mu.Unlock()
	defer func() { b.mu.Lock(); b.directorySizeRunning = false; b.mu.Unlock() }()
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	result, err := toolbox.BrowserDirectorySize(ctx, p, scope)
	if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) && !errors.Is(err, context.DeadlineExceeded) {
		browserFail(w, local, err)
		return
	}
	send(w, result)
}

func (b *fileBrowser) resourceProperties(w http.ResponseWriter, r *http.Request, local bool) {
	var req struct{ Path, Version string }
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(&req) != nil {
		browserFail(w, local, errors.New("属性请求无效"))
		return
	}
	p, _, err := b.resolve(r, local, false, req.Path)
	if err != nil {
		browserFail(w, local, err)
		return
	}
	if err = b.checkPropertyReference(p, req.Version); err != nil {
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
	if err != nil {
		browserFail(w, local, err)
		return
	}
	// Directory metadata only: never recursively walk a disk just to open a dialog.
	display := req.Path
	if local {
		display = f.Name()
	} else {
		display = filepath.ToSlash(filepath.Clean(filepath.FromSlash(req.Path)))
		if display == "." {
			display = ""
		}
	}
	name := st.Name()
	if !local && display == "" {
		name = "共享根目录"
	}
	kind := browserFileKind(f, st)
	result := map[string]any{"Name": name, "Path": display, "Kind": kind, "Dir": st.IsDir(), "Size": st.Size(), "Modified": st.ModTime(), "Permissions": st.Mode().String()}
	addNativeFileProperties(f, result)
	if st.Mode().IsRegular() {
		switch kind {
		case "image":
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			state := b.mediaState()
			select {
			case state.imageSlots <- struct{}{}:
				width, height, format, e := toolbox.BrowserImageDimensions(ctx, b.imageThumbnailProgram(), f)
				<-state.imageSlots
				if e == nil {
					result["Width"], result["Height"], result["ImageFormat"] = width, height, format
				} else {
					result["MediaError"] = "无法读取图片尺寸，请检查图片或 ImageMagick 组件"
				}
			case <-ctx.Done():
				result["MediaError"] = "读取图片信息超时，请稍后重试"
			}
		case "video", "audio":
			info, e := b.probeBrowserMedia(r.Context(), f, st)
			if e == nil {
				result["Width"], result["Height"], result["Duration"] = info.Width, info.Height, info.Duration
				result["VideoCodec"], result["AudioCodec"], result["Container"] = info.VideoCodec, info.AudioCodec, info.Container
			} else {
				result["MediaError"] = "无法读取媒体信息，请检查文件或 FFprobe 组件"
			}
		}
	}
	send(w, result)
}
