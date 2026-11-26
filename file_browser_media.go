package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

type browserCacheEntry[T any] struct {
	Value T
	Bytes int
	Used  uint64
}

func browserVideoMIME(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4", ".m4v", ".f4v":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".mkv":
		return "video/x-matroska"
	case ".webm":
		return "video/webm"
	case ".avi", ".divx":
		return "video/x-msvideo"
	case ".ts", ".mts", ".m2ts":
		return "video/mp2t"
	case ".mpg", ".mpeg", ".mpe", ".vob", ".m2v":
		return "video/mpeg"
	case ".wmv", ".asf":
		return "video/x-ms-asf"
	case ".flv":
		return "video/x-flv"
	case ".3gp":
		return "video/3gpp"
	case ".3g2":
		return "video/3gpp2"
	case ".ogv":
		return "video/ogg"
	default:
		return "application/octet-stream"
	}
}

type browserMediaCache[T any] struct {
	mu                  sync.Mutex
	items               map[string]browserCacheEntry[T]
	bytes, limit, count int
	clock               uint64
}

func newBrowserMediaCache[T any](count, limit int) *browserMediaCache[T] {
	return &browserMediaCache[T]{items: map[string]browserCacheEntry[T]{}, count: count, limit: limit}
}
func (c *browserMediaCache[T]) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = map[string]browserCacheEntry[T]{}
	c.bytes = 0
}
func (c *browserMediaCache[T]) get(key string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.items[key]
	if ok {
		c.clock++
		item.Used = c.clock
		c.items[key] = item
	}
	return item.Value, ok
}
func (c *browserMediaCache[T]) put(key string, value T, size int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if size > c.limit {
		return
	}
	if old, ok := c.items[key]; ok {
		c.bytes -= old.Bytes
		delete(c.items, key)
	}
	for len(c.items) >= c.count || c.bytes+size > c.limit {
		var oldest string
		var age uint64 = math.MaxUint64
		for k, item := range c.items {
			if item.Used < age {
				oldest, age = k, item.Used
			}
		}
		c.bytes -= c.items[oldest].Bytes
		delete(c.items, oldest)
	}
	c.clock++
	c.items[key] = browserCacheEntry[T]{value, size, c.clock}
	c.bytes += size
}

type browserMediaState struct {
	probeSlots, playSlots, imageSlots chan struct{}
	hoverSlots                        chan struct{}
	info                              *browserMediaCache[toolbox.BrowserMediaInfo]
	thumbnails, images                *browserMediaCache[[]byte]
	hover                             *browserMediaCache[[]byte]
}

func (b *fileBrowser) mediaState() *browserMediaState {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.media == nil {
		b.media = &browserMediaState{probeSlots: make(chan struct{}, 2), playSlots: make(chan struct{}, 2), imageSlots: make(chan struct{}, 2), info: newBrowserMediaCache[toolbox.BrowserMediaInfo](1024, 1024), thumbnails: newBrowserMediaCache[[]byte](128, 16<<20), images: newBrowserMediaCache[[]byte](12, 32<<20)}
		b.media.hoverSlots = make(chan struct{}, 1)
		b.media.hover = newBrowserMediaCache[[]byte](24, 16<<20)
	}
	return b.media
}
func browserMediaKey(f *os.File, st os.FileInfo) string {
	return fmt.Sprintf("%s\x00%d\x00%d", f.Name(), st.Size(), st.ModTime().UnixNano())
}
func (b *fileBrowser) browserProbeProgram() string {
	if p, err := (&server{dataDir: b.dataDir}).loadPreferences(); err == nil {
		for _, key := range []string{"thumbnail", "video", "exif", "describe"} {
			if program := p.Workspaces[key].FFprobe; program != "" {
				if found, err := exec.LookPath(program); err == nil {
					return found
				}
			}
		}
	}
	return toolbox.DefaultConfig().FFprobe
}
func (b *fileBrowser) probeBrowserMedia(ctx context.Context, f *os.File, st os.FileInfo) (toolbox.BrowserMediaInfo, error) {
	state, key := b.mediaState(), browserMediaKey(f, st)
	if info, ok := state.info.get(key); ok {
		return info, nil
	}
	select {
	case state.probeSlots <- struct{}{}:
		defer func() { <-state.probeSlots }()
	case <-ctx.Done():
		return toolbox.BrowserMediaInfo{}, ctx.Err()
	}
	if info, ok := state.info.get(key); ok {
		return info, nil
	}
	probeCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	info, err := toolbox.ProbeBrowserMedia(probeCtx, b.browserProbeProgram(), f)
	if err == nil {
		state.info.put(key, info, 1)
	}
	return info, err
}
func (b *fileBrowser) mediaInfo(w http.ResponseWriter, r *http.Request, local bool) {
	var req struct {
		Path        string
		Lightweight bool
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		fail(w, errors.New("媒体路径无效"))
		return
	}
	p, scope, err := b.resolve(r, local, false, req.Path)
	if err != nil {
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
	if err != nil || !st.Mode().IsRegular() || browserFileKind(f, st) != "video" {
		fail(w, errors.New("请选择视频文件"))
		return
	}
	if scope != nil {
		if _, err = scope.Resolve(req.Path, false); err != nil {
			browserFail(w, false, err)
			return
		}
	}
	if req.Lightweight {
		send(w, map[string]bool{"Fragmented": toolbox.BrowserFragmentedMP4(f)})
		return
	}
	info, err := b.probeBrowserMedia(r.Context(), f, st)
	if err != nil {
		browserFail(w, local, err)
		return
	}
	send(w, info)
}

type browserStreamWriter struct {
	w       http.ResponseWriter
	started bool
}

func (s *browserStreamWriter) Write(data []byte) (int, error) {
	if !s.started {
		s.w.Header().Set("Content-Type", "video/mp4")
		s.w.Header().Set("Accept-Ranges", "none")
		s.w.Header().Set("X-Accel-Buffering", "no")
		s.w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'self'")
		s.started = true
	}
	_ = http.NewResponseController(s.w).SetWriteDeadline(time.Now().Add(30 * time.Second))
	n, err := s.w.Write(data)
	if flush, ok := s.w.(http.Flusher); ok {
		flush.Flush()
	}
	return n, err
}
func (b *fileBrowser) playBrowserMedia(w http.ResponseWriter, r *http.Request, f *os.File, st os.FileInfo, local bool) {
	mode := r.URL.Query().Get("mode")
	if mode != "auto" && mode != "convert" {
		fail(w, errors.New("播放模式无效"))
		return
	}
	start := 0.0
	if value := r.URL.Query().Get("start"); value != "" {
		var err error
		start, err = strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(start) || math.IsInf(start, 0) || start < 0 || start > 7*24*3600 {
			fail(w, errors.New("播放时间无效"))
			return
		}
	}
	if browserFileKind(f, st) != "video" {
		fail(w, errors.New("请选择视频文件"))
		return
	}
	if r.Method == "HEAD" {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Accept-Ranges", "none")
		return
	}
	state := b.mediaState()
	select {
	case state.playSlots <- struct{}{}:
		defer func() { <-state.playSlots }()
	default:
		http.Error(w, "兼容播放已达到 2 路，请关闭其他预览后重试", http.StatusTooManyRequests)
		return
	}
	info, err := b.probeBrowserMedia(r.Context(), f, st)
	if err != nil {
		browserFail(w, local, err)
		return
	}
	if info.Duration > 0 && start >= info.Duration {
		fail(w, errors.New("播放时间已超过视频长度"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Hour)
	defer cancel()
	stream := &browserStreamWriter{w: w}
	err = toolbox.StreamBrowserMedia(ctx, b.thumbnailProgram(), f, info, start, mode == "convert", stream)
	if err != nil && !stream.started {
		browserFail(w, local, err)
	}
}

func (b *fileBrowser) previewBrowserImage(w http.ResponseWriter, r *http.Request, f *os.File, st os.FileInfo, local bool) {
	if browserFileKind(f, st) != "image" || st.Size() > 128<<20 {
		fail(w, errors.New("图片过大或格式不支持，请下载原图查看"))
		return
	}
	state, key := b.mediaState(), browserMediaKey(f, st)
	serve := func(data []byte) {
		w.Header().Set("Content-Type", "image/jpeg")
		if r.Method != "HEAD" {
			w.Write(data)
		}
	}
	if data, ok := state.images.get(key); ok {
		serve(data)
		return
	}
	select {
	case state.imageSlots <- struct{}{}:
		defer func() { <-state.imageSlots }()
	case <-r.Context().Done():
		return
	}
	if data, ok := state.images.get(key); ok {
		serve(data)
		return
	}
	format := toolbox.BrowserImageFormat(f)
	if format == "" {
		fail(w, errors.New("此图片格式暂不支持兼容预览"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	data, err := toolbox.BrowserImagePreview(ctx, b.imageThumbnailProgram(), f, format)
	if err != nil {
		browserFail(w, local, err)
		return
	}
	state.images.put(key, data, len(data))
	serve(data)
}
