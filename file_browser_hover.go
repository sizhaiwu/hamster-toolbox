package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

func (b *fileBrowser) hoverBrowserVideo(w http.ResponseWriter, r *http.Request, f *os.File, st os.FileInfo, local bool) {
	if browserFileKind(f, st) != "video" {
		fail(w, errors.New("请选择视频文件"))
		return
	}
	w.Header().Set("Content-Type", "video/mp4")
	if r.Method == http.MethodHead {
		return // Probing headers must not launch FFmpeg.
	}
	state, key := b.mediaState(), browserMediaKey(f, st)
	serve := func(data []byte) { http.ServeContent(w, r, "preview.mp4", st.ModTime(), bytes.NewReader(data)) }
	if data, ok := state.hover.get(key); ok {
		serve(data)
		return
	}
	// A separate single slot keeps hovering from occupying full-video playback slots.
	select {
	case state.hoverSlots <- struct{}{}:
		defer func() { <-state.hoverSlots }()
	default:
		http.Error(w, "正在生成其他悬停预览，请稍后重试", http.StatusTooManyRequests)
		return
	}
	if data, ok := state.hover.get(key); ok {
		serve(data)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if _, err := b.probeBrowserMedia(ctx, f, st); err != nil {
		browserFail(w, local, err)
		return
	}
	data, err := toolbox.BrowserVideoHover(ctx, b.thumbnailProgram(), f)
	if err != nil {
		browserFail(w, local, err)
		return
	}
	state.hover.put(key, data, len(data))
	serve(data)
}
