package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

func securityBrowserHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data: blob:; media-src 'self' blob:; connect-src 'self'; frame-src 'self'; object-src 'none'; frame-ancestors 'self'; base-uri 'none'; form-action 'self'")
}

// The login page needs the same public scripts and styles as the authenticated
// Explorer. Keep one exact allowlist so new shared UI modules cannot be missed.
// This includes only embedded UI assets, never configuration or file APIs.
func browserStaticAsset(path string) bool {
	switch path {
	case "", "explorer.html", "file-code-editor.js", "explorer-editor-window.js", "version-info.js", "network-settings.js", "network-settings.css", "general-settings.css", "task-feedback.js", "explorer-upgrades.js", "explorer.css", "theme.css", "explorer-layout.css", "explorer.js", "explorer-tabs.js", "explorer-media.js", "explorer-shell.js", "explorer-native.js", "explorer-floating.js", "explorer-floating.css":
		return true
	}
	return brandContentType(path) != ""
}
func (b *fileBrowser) static(w http.ResponseWriter, r *http.Request, path string) {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, "GET required", 405)
		return
	}
	if path == "" {
		path = "explorer.html"
	}
	if !browserStaticAsset(path) {
		http.NotFound(w, r)
		return
	}
	data, e := assets.ReadFile("web/" + path)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	contentType := brandContentType(path)
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(path))
	}
	w.Header().Set("Content-Type", contentType)
	if path == "explorer.html" {
		nonce := randomID()
		data = bytes.ReplaceAll(data, []byte("__FILE_EDITOR_NONCE__"), []byte(nonce))
		w.Header().Set("Content-Security-Policy", strings.Replace(w.Header().Get("Content-Security-Policy"), "style-src 'self'", "style-src 'self' 'nonce-"+nonce+"'", 1))
		data = sharedSettingsHTML(data)
		b.mu.Lock()
		enabled := b.config.WorkspaceEnabled
		b.mu.Unlock()
		if enabled {
			data = bytes.Replace(data, []byte(`<button id="login"`), []byte(`<a class="login-nav-link" href="/admin/"><span>管理员工作台</span><span aria-hidden="true">→</span></a><button id="login"`), 1)
		}
	}
	if r.Method != "HEAD" {
		w.Write(data)
	}
}
func (b *fileBrowser) resolve(r *http.Request, local, write bool, path string) (string, *toolbox.BrowserScope, error) {
	if local {
		if !filepath.IsAbs(path) {
			return "", nil, errors.New("请输入完整目录路径")
		}
		return path, nil, nil
	}
	id := r.URL.Query().Get("root")
	b.mu.Lock()
	scope := b.scopes[id]
	writable := false
	for _, root := range b.config.Roots {
		if root.ID == id {
			writable = root.Writable
			break
		}
	}
	b.mu.Unlock()
	if scope == nil {
		return "", nil, errors.New("共享目录不存在或已更新，请返回首页")
	}
	if write && !writable {
		return "", nil, errors.New("此目录为只读共享，不能修改")
	}
	p, e := scope.Resolve(path, write)
	return p, scope, e
}
func browserKind(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == ".lnk" {
		return "shortcut"
	}
	for kind, list := range map[string]string{"image": ".jpg .jpeg .jpe .jfif .png .webp .gif .bmp .avif .heic .heif .tif .tiff", "video": ".mp4 .m4v .mov .mkv .webm .avi .m2ts .mpg .mpeg .mpe .wmv .flv .f4v .3gp .3g2 .vob .m2v .asf .rm .rmvb .ogv .divx .mxf", "audio": ".mp3 .wav .m4a .aac .ogg .flac .opus", "text": ".txt .md .json .csv .log .xml .yaml .yml .ini .srt .ass .vtt .html .htm .svg .css .js .jsx .mjs .cjs .ts .tsx .mts .cts .go .py .ps1 .bat .cmd", "pdf": ".pdf"} {
		if strings.Contains(" "+list+" ", " "+ext+" ") {
			return kind
		}
	}
	if ext != "" && toolbox.IsExtractArchiveName(name) {
		return "archive"
	}
	return "file"
}

type browserEntry struct {
	Name, Path, Kind string
	Version          string
	Dir              bool
	Size             int64
	Modified         time.Time
}

func (b *fileBrowser) dispatch(w http.ResponseWriter, r *http.Request, local bool, path string) {
	if browserStaticAsset(path) {
		b.static(w, r, path)
		return
	}
	if path == "raw" || path == "thumb" || path == "text" || path == "media-play" || path == "image-preview" || path == "video-preview" {
		if r.Method != "GET" && r.Method != "HEAD" {
			http.Error(w, "GET required", 405)
			return
		}
		if path == "thumb" || path == "media-play" || path == "image-preview" || path == "video-preview" {
			site, origin := r.Header.Get("Sec-Fetch-Site"), r.Header.Get("Origin")
			if site != "" && site != "same-origin" && site != "none" || origin != "" && origin != browserRequestOrigin(r) {
				http.Error(w, "请从工具箱页面预览资源", http.StatusForbidden)
				return
			}
		}
		b.content(w, r, local, path)
		return
	}
	if !browserPOST(r) {
		http.Error(w, "需要同源POST请求", 403)
		return
	}
	if path == "api/upload-init" || path == "api/upload-chunk" || path == "api/upload-finish" {
		if path == "api/upload-chunk" {
			r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
		} else {
			r.Body = http.MaxBytesReader(w, r.Body, 512<<10)
		}
		b.resumable(w, r, local, path)
		return
	}
	if path == "api/upload" {
		b.upload(w, r, local)
		return
	}
	bodyLimit := int64(128 * 1024)
	if path == "api/preferences" {
		bodyLimit = 512 * 1024
	}
	if path == "api/text-save" {
		bodyLimit = 16 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, bodyLimit)
	switch path {
	case "api/text-open", "api/text-save":
		b.editText(w, r, local, path == "api/text-save")
	case "api/transfer":
		b.transfer(w, r, local)
	case "api/cache-clear":
		if !local {
			http.NotFound(w, r)
			return
		}
		if err := b.clearDiskThumbnails(); err != nil {
			fail(w, err)
			return
		}
		send(w, map[string]bool{"OK": true})
	case "api/boot":
		if local {
			preferences, err := b.loadExplorerPreferences()
			preferencesError := ""
			if err != nil {
				preferencesError = err.Error()
			}
			send(w, map[string]any{"Local": true, "Sharing": b.status(), "Platform": runtime.GOOS, "Separator": string(filepath.Separator), "ToolboxURL": "/?session=" + b.ownerToken, "Preferences": preferences, "PreferencesError": preferencesError})
			return
		}
		b.mu.Lock()
		roots := []map[string]any{}
		for _, root := range b.config.Roots {
			if b.scopes[root.ID] == nil {
				continue
			}
			roots = append(roots, map[string]any{"ID": root.ID, "Name": root.Name, "Writable": root.Writable})
		}
		b.mu.Unlock()
		send(w, map[string]any{"Local": false, "Platform": runtime.GOOS, "Separator": "/", "Roots": roots})
	case "api/preferences":
		if !local {
			http.NotFound(w, r)
			return
		}
		preferences, err := b.saveExplorerPreferences(r)
		if err != nil {
			fail(w, err)
			return
		}
		send(w, preferences)
	case "api/sharing":
		if !local {
			http.NotFound(w, r)
			return
		}
		if e := b.saveSharing(r); e != nil {
			fail(w, e)
			return
		}
		send(w, b.status())
	case "api/logout":
		if c, e := r.Cookie("hamster_guest_" + b.cookieTag); e == nil {
			b.mu.Lock()
			delete(b.sessions, c.Value)
			b.mu.Unlock()
		}
		http.SetCookie(w, &http.Cookie{Name: "hamster_guest_" + b.cookieTag, Path: "/", MaxAge: -1, HttpOnly: true, Secure: browserRequestSecure(r), SameSite: http.SameSiteStrictMode})
		send(w, map[string]bool{"OK": true})
	case "api/list":
		b.list(w, r, local)
	case "api/media-info":
		b.mediaInfo(w, r, local)
	case "api/shortcut":
		b.directoryShortcut(w, r, local)
	case "api/properties":
		b.resourceProperties(w, r, local)
	case "api/properties-size":
		b.resourceDirectorySize(w, r, local)
	case "api/native-player":
		if !local {
			http.NotFound(w, r)
			return
		}
		b.nativePlayerAPI(w, r)
	case "api/open-local":
		if !local {
			http.NotFound(w, r)
			return
		}
		if err := b.openLocalDirectory(r); err != nil {
			fail(w, err)
			return
		}
		send(w, map[string]bool{"OK": true})
	case "api/open-with":
		if !local {
			http.NotFound(w, r)
			return
		}
		err := b.openLocalFileWith(r)
		if errors.Is(err, errOpenWithCancelled) {
			send(w, map[string]bool{"OK": true, "Cancelled": true})
			return
		}
		if err != nil {
			fail(w, err)
			return
		}
		send(w, map[string]bool{"OK": true})
	case "api/mutate":
		b.mutate(w, r, local)
	default:
		http.NotFound(w, r)
	}
}
func (b *fileBrowser) list(w http.ResponseWriter, r *http.Request, local bool) {
	if r.Context().Err() != nil {
		return
	}
	var req struct {
		Path, Search, Sort, Order         string
		DirectoriesOnly, Recursive, Force bool
		Offset, PageSize                  int
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		fail(w, errors.New("目录参数无效"))
		return
	}
	var optionErr error
	req.Sort, req.Order, req.PageSize, optionErr = browserListOptions(req.Sort, req.Order, req.PageSize)
	if optionErr != nil {
		fail(w, optionErr)
		return
	}
	if local && req.Path == "" {
		drives := []browserEntry{}
		for _, p := range browserLocations() {
			drives = append(drives, browserEntry{Name: p, Path: p, Dir: true, Kind: "drive"})
		}
		sortBrowserEntries(drives, req.Sort, req.Order)
		send(w, map[string]any{"Path": "", "Parent": "", "Entries": drives, "Total": len(drives), "Offset": 0, "PageSize": req.PageSize, "Order": req.Order})
		return
	}
	p, scope, e := b.resolve(r, local, false, req.Path)
	if e != nil {
		browserFail(w, local, e)
		return
	}
	f, e := toolbox.BrowseOpen(p)
	if e != nil {
		browserFail(w, local, e)
		return
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.IsDir() {
		browserFail(w, local, errors.New("不是可读取的目录"))
		return
	}
	items, e := b.readDirectoryListing(r, p, scope, req.Search, req.DirectoriesOnly, req.Recursive && req.Search != "", req.Force)
	if e != nil {
		browserFail(w, local, e)
		return
	}
	truncated := false
	sortBrowserEntries(items, req.Sort, req.Order)
	total := len(items)
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}
	if offset >= total && total > 0 {
		offset = (total - 1) / req.PageSize * req.PageSize
	}
	if total == 0 {
		offset = 0
	}
	offset = offset / req.PageSize * req.PageSize
	end := offset + req.PageSize
	if end > total {
		end = total
	}
	for i := offset; i < end; i++ {
		if r.Context().Err() != nil {
			return
		}
		actual := items[i].Path
		if !local {
			actual = filepath.Join(scope.Root, filepath.FromSlash(actual))
		}
		ref, e := toolbox.CaptureBrowserItem(actual)
		if e != nil {
			continue
		}
		version := randomID()
		b.mu.Lock()
		if len(b.references) >= 8192 {
			b.references = map[string]browserReference{}
		}
		b.references[version] = browserReference{ref, time.Now()}
		b.mu.Unlock()
		items[i].Version = version
	}
	current, parent := p, filepath.Dir(p)
	if parent == p {
		parent = ""
	}
	if !local {
		current = filepath.ToSlash(strings.TrimPrefix(strings.TrimPrefix(p, scope.Root), string(filepath.Separator)))
		if current == "" {
			parent = ""
		} else {
			parent = filepath.ToSlash(filepath.Dir(current))
			if parent == "." {
				parent = ""
			}
		}
		if _, e = scope.Resolve(req.Path, false); e != nil {
			browserFail(w, false, e)
			return
		}
	}
	send(w, map[string]any{"Path": current, "Parent": parent, "Entries": items[offset:end], "Total": total, "Offset": offset, "PageSize": req.PageSize, "Order": req.Order, "Truncated": truncated})
}
func browserFail(w http.ResponseWriter, local bool, e error) {
	if local {
		fail(w, e)
	} else {
		http.Error(w, "无法访问或执行操作：请检查权限、名称是否重复、共享是否变化；未授权访问共享外目录", 400)
	}
}
func (b *fileBrowser) mutate(w http.ResponseWriter, r *http.Request, local bool) {
	var req struct {
		Action, Path, Target, Name, Version string
		Permanent                           bool
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		fail(w, errors.New("操作参数无效"))
		return
	}
	b.operations.Lock()
	defer b.operations.Unlock()
	defer b.invalidateDirectoryListings()
	if r.Context().Err() != nil {
		return
	}
	// Resolve the selected item for writes except when creating a child inside
	// a shared root, whose identity itself must remain immutable.
	creating := req.Action == "mkdir" || req.Action == "create"
	p, _, e := b.resolve(r, local, !creating, req.Path)
	if e != nil {
		browserFail(w, local, e)
		return
	}
	if !creating {
		b.mu.Lock()
		ref, ok := b.references[req.Version]
		b.mu.Unlock()
		if !ok || time.Since(ref.At) > 15*time.Minute {
			browserFail(w, local, errors.New("选择已过期，请刷新目录后重新选择"))
			return
		}
		if e := ref.Ref.Check(p); e != nil {
			browserFail(w, local, e)
			return
		}
	}
	switch req.Action {
	case "mkdir", "create":
		if !toolbox.BrowserName(req.Name) {
			e = errors.New("名称不合法，请不要包含路径或特殊字符")
			break
		}
		target := filepath.Join(req.Path, req.Name)
		if !local {
			target = strings.TrimSuffix(req.Path, "/") + "/" + req.Name
			target = strings.TrimPrefix(target, "/")
		}
		dst, _, err := b.resolve(r, local, true, target)
		if err != nil {
			e = err
			break
		}
		if req.Action == "create" {
			e = toolbox.BrowserUpload(r.Context(), dst, strings.NewReader(""))
			if e == nil {
				send(w, map[string]any{"OK": true, "Path": target, "Kind": browserKind(req.Name)})
				return
			}
		} else {
			e = toolbox.BrowserMkdir(dst)
		}
	case "rename":
		if !toolbox.BrowserName(req.Name) {
			e = errors.New("名称不合法")
			break
		}
		target := filepath.Join(filepath.Dir(req.Path), req.Name)
		dst, _, err := b.resolve(r, local, true, target)
		if err != nil {
			e = err
			break
		}
		e = toolbox.BrowserMove(p, dst)
	case "move":
		dst, _, err := b.resolve(r, local, true, req.Target)
		if err != nil {
			e = err
			break
		}
		e = toolbox.BrowserMove(p, dst)
	case "delete":
		e = toolbox.BrowserDelete(r.Context(), p, req.Permanent)
	default:
		e = errors.New("不支持此操作")
	}
	if e != nil {
		browserFail(w, local, e)
		return
	}
	send(w, map[string]bool{"OK": true})
}
func (b *fileBrowser) upload(w http.ResponseWriter, r *http.Request, local bool) {
	name := r.URL.Query().Get("name")
	if !toolbox.BrowserName(name) {
		fail(w, errors.New("上传文件名不合法"))
		return
	}
	path := r.URL.Query().Get("path")
	target := filepath.Join(path, name)
	if !local {
		target = strings.TrimPrefix(strings.TrimSuffix(path, "/")+"/"+name, "/")
	}
	b.operations.Lock()
	defer b.operations.Unlock()
	defer b.invalidateDirectoryListings()
	p, _, e := b.resolve(r, local, true, target)
	if e != nil {
		browserFail(w, local, e)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<30)
	e = toolbox.BrowserUpload(r.Context(), p, r.Body)
	if e != nil {
		browserFail(w, local, e)
		return
	}
	send(w, map[string]bool{"OK": true})
}
func (b *fileBrowser) content(w http.ResponseWriter, r *http.Request, local bool, action string) {
	path := r.URL.Query().Get("path")
	p, scope, e := b.resolve(r, local, false, path)
	if e != nil {
		browserFail(w, local, e)
		return
	}
	f, e := toolbox.BrowseOpen(p)
	if e != nil {
		browserFail(w, local, e)
		return
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() {
		http.Error(w, "不是普通文件", 400)
		return
	}
	if scope != nil {
		if _, e = scope.Resolve(path, false); e != nil {
			browserFail(w, false, e)
			return
		}
	}
	if action == "thumb" {
		b.thumbnail(w, r, f, st)
		return
	}
	if action == "media-play" {
		b.playBrowserMedia(w, r, f, st, local)
		return
	}
	if action == "video-preview" {
		b.hoverBrowserVideo(w, r, f, st, local)
		return
	}
	if action == "image-preview" {
		b.previewBrowserImage(w, r, f, st, local)
		return
	}
	if action == "text" {
		if st.Size() > 1<<20 {
			http.Error(w, "文本超过1MiB，请下载查看", 400)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.Copy(w, f)
		return
	}
	head := make([]byte, 512)
	n, _ := f.Read(head)
	f.Seek(0, 0)
	detected := http.DetectContentType(head[:n])
	kind := browserFileKind(f, st)
	contentType := mime.TypeByExtension(filepath.Ext(st.Name()))
	inline := false
	if kind == "image" {
		inline = true
		if !strings.HasPrefix(contentType, "image/") {
			contentType = "image/" + strings.TrimPrefix(strings.ToLower(filepath.Ext(st.Name())), ".")
		}
	}
	if strings.HasPrefix(detected, "image/") && (kind == "image") {
		contentType = detected
		inline = true
	}
	if kind == "video" || kind == "audio" {
		inline = true
		if kind == "video" {
			contentType = browserVideoMIME(st.Name())
		}
		if !strings.HasPrefix(contentType, kind+"/") {
			contentType = "application/octet-stream"
		}
	}
	if kind == "pdf" && bytes.HasPrefix(head[:n], []byte("%PDF-")) {
		contentType = "application/pdf"
		inline = true
	}
	if !inline || r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": st.Name()}))
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	policy := "default-src 'none'; frame-ancestors 'self'"
	if !inline {
		policy += "; sandbox"
	}
	w.Header().Set("Content-Security-Policy", policy)
	if kind == "video" && r.URL.Query().Get("download") != "1" {
		w.Header().Set("ETag", fmt.Sprintf("\"%x-%x\"", st.ModTime().UnixNano(), st.Size()))
		r = fairBrowserVideoRange(r, st.Size(), local)
	}
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
}
func (b *fileBrowser) thumbnail(w http.ResponseWriter, r *http.Request, f *os.File, st os.FileInfo) {
	kind := browserFileKind(f, st)
	if kind != "image" && kind != "video" {
		http.Error(w, "无缩略图", http.StatusNotFound)
		return
	}
	key, state := browserMediaKey(f, st), b.mediaState()
	if cached, ok := state.thumbnails.get(key); ok {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(cached)
		return
	}
	if cached, ok := b.diskThumbnail(key); ok {
		state.thumbnails.put(key, cached, len(cached))
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(cached)
		return
	}
	select {
	case b.thumbSlots <- struct{}{}:
		defer func() { <-b.thumbSlots }()
	case <-r.Context().Done():
		return
	}
	if cached, ok := state.thumbnails.get(key); ok {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(cached)
		return
	}
	var data []byte
	if kind == "image" && st.Size() <= 128<<20 {
		cfg, _, e := image.DecodeConfig(f)
		f.Seek(0, 0)
		if e == nil && st.Size() <= 32<<20 && cfg.Width > 0 && cfg.Height > 0 && int64(cfg.Width)*int64(cfg.Height) <= 24_000_000 {
			src, _, e := image.Decode(f)
			if e == nil {
				width, height := cfg.Width, cfg.Height
				if width > 640 || height > 480 {
					ratio := min(640.0/float64(width), 480.0/float64(height))
					width = max(1, int(float64(width)*ratio))
					height = max(1, int(float64(height)*ratio))
				}
				dst := image.NewRGBA(image.Rect(0, 0, width, height))
				for y := 0; y < height; y++ {
					for x := 0; x < width; x++ {
						r, g, blue, alpha := src.At(src.Bounds().Min.X+x*cfg.Width/width, src.Bounds().Min.Y+y*cfg.Height/height).RGBA()
						white := uint32(65535) - alpha
						dst.SetRGBA(x, y, color.RGBA{uint8((r + white) >> 8), uint8((g + white) >> 8), uint8((blue + white) >> 8), 255})
					}
				}
				var out bytes.Buffer
				jpeg.Encode(&out, dst, &jpeg.Options{Quality: 78})
				data = out.Bytes()
			}
		}
		if len(data) == 0 && (e != nil || cfg.Width > 0 && cfg.Height > 0 && int64(cfg.Width)*int64(cfg.Height) <= 120_000_000) {
			// Read the real encoding, including WebP stored under a .jpg/.png name.
			format := toolbox.BrowserImageFormat(f)
			if format != "" {
				select {
				case state.imageSlots <- struct{}{}:
				case <-r.Context().Done():
					return
				}
				ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
				data, _ = toolbox.BrowserImageThumbnail(ctx, b.imageThumbnailProgram(), f, format)
				cancel()
				<-state.imageSlots
			}
		}
	} else if kind == "video" {
		program := b.thumbnailProgram()
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		data, _ = toolbox.BrowserVideoThumbnail(ctx, program, f)
	}
	if len(data) == 0 {
		http.Error(w, "无缩略图", 404)
		return
	}
	state.thumbnails.put(key, data, len(data))
	b.storeDiskThumbnail(key, data)
	w.Header().Set("Content-Type", "image/jpeg")
	w.Write(data)
}

func (b *fileBrowser) thumbnailProgram() string {
	// Use the user's saved program first; the browser is its own workspace.
	if p, e := (&server{dataDir: b.dataDir}).loadPreferences(); e == nil {
		for _, key := range []string{"thumbnail", "video", "exif"} {
			if program := p.Workspaces[key].FFmpeg; program != "" {
				if found, e := exec.LookPath(program); e == nil {
					return found
				}
			}
		}
	}
	return toolbox.DefaultConfig().FFmpeg
}

func (b *fileBrowser) imageThumbnailProgram() string {
	if p, err := (&server{dataDir: b.dataDir}).loadPreferences(); err == nil {
		for _, key := range []string{"image", "pdf", "thumbnail"} {
			if program := p.Workspaces[key].Magick; program != "" {
				if found, err := exec.LookPath(program); err == nil {
					return found
				}
			}
		}
	}
	return toolbox.DefaultConfig().Magick
}
