package main

import (
	"bytes"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// The file guest password never grants access to this administrator surface.
// Each remote browser has its own CSRF capability and independent notes memory.
type workspaceSession struct {
	timer         *time.Timer
	hash          string
	csrf          string
	last, expires time.Time
	notes         *server
}
type remoteWorkspace struct {
	mu       sync.Mutex
	owner    *server
	handler  http.Handler
	sessions map[string]*workspaceSession
	attempts map[string]loginWindow
	slots    chan struct{}
}

func newRemoteWorkspace(owner *server, handler http.Handler) *remoteWorkspace {
	return &remoteWorkspace{owner: owner, handler: handler, sessions: map[string]*workspaceSession{}, attempts: map[string]loginWindow{}, slots: make(chan struct{}, 2)}
}
func validateWorkspaceConfig(c sharingConfig) error {
	salt, a := hex.DecodeString(c.WorkspaceSalt)
	hash, b := hex.DecodeString(c.WorkspaceHash)
	if a != nil || b != nil || len(salt) != 24 || len(hash) != 32 {
		return errors.New("开启完整工具箱前请设置独立的管理员口令")
	}
	return nil
}
func lockWorkspaceNotes(s *workspaceSession) {
	if s.timer != nil {
		s.timer.Stop()
	}
	s.notes.notes.mu.Lock()
	defer s.notes.notes.mu.Unlock()
	s.notes.notes.lock()
}
func (a *remoteWorkspace) revokeAll() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for id, s := range a.sessions {
		lockWorkspaceNotes(s)
		delete(a.sessions, id)
	}
}
func (a *remoteWorkspace) session(r *http.Request, tag string) *workspaceSession {
	cookie, err := r.Cookie("hamster_admin_" + tag)
	if err != nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.sessions[cookie.Value]
	if s == nil {
		return nil
	}
	now := time.Now()
	if now.After(s.expires) || now.Sub(s.last) > 30*time.Minute {
		delete(a.sessions, cookie.Value)
		lockWorkspaceNotes(s)
		return nil
	}
	// A background task-status poll must not keep an abandoned admin page
	// authenticated indefinitely.
	passiveSchedule := r.URL.Path == "/api/schedules" && r.Header.Get("X-Hamster-Poll") == "1"
	if r.URL.Path != "/api/state" && r.URL.Path != "/admin/status" && r.URL.Path != "/api/version-update" && !passiveSchedule {
		s.last = now
		if s.timer != nil {
			s.timer.Reset(min(30*time.Minute, time.Until(s.expires)))
		}
	}
	return s
}
func workspaceSameOrigin(r *http.Request) bool {
	origin, site := r.Header.Get("Origin"), r.Header.Get("Sec-Fetch-Site")
	return (origin == "" || origin == browserRequestOrigin(r)) && (site == "" || site == "same-origin" || site == "none")
}
func (b *fileBrowser) remoteWorkspace(w http.ResponseWriter, r *http.Request, cfg sharingConfig) bool {
	if strings.HasPrefix(r.URL.Path, "/api/desktop-client") {
		http.Error(w, "桌面客户端仅限服务端电脑本机管理", 403)
		return true
	}
	scheduleRoute := r.URL.Path == "/api/schedules" || strings.HasPrefix(r.URL.Path, "/schedules.") || strings.HasPrefix(r.URL.Path, "/schedule-")
	isAdminPath := strings.HasPrefix(r.URL.Path, "/admin/")
	if !cfg.WorkspaceEnabled || b.workspace == nil {
		if scheduleRoute {
			http.Error(w, "请先开启完整工具箱远程访问并使用管理员口令登录", http.StatusForbidden)
			return true
		}
		if isAdminPath {
			http.NotFound(w, r)
			return true
		}
		return false
	}
	a := b.workspace
	if !cfg.WorkspaceHTTP && !browserRequestSecure(r) {
		if isAdminPath || scheduleRoute {
			http.Error(w, "完整工具箱需要 HTTPS。若已使用可信内网或加密组网，请在本机设置中明确允许 HTTP。", http.StatusForbidden)
			return true
		}
		return false
	}
	if r.URL.Path == "/admin/" || r.URL.Path == "/admin/login.js" {
		name, kind := "remote-login.html", "text/html; charset=utf-8"
		if strings.HasSuffix(r.URL.Path, ".js") {
			name, kind = "remote-login.js", "text/javascript; charset=utf-8"
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			http.Error(w, "GET required", 405)
			return true
		}
		data, _ := assets.ReadFile("web/" + name)
		w.Header().Set("Content-Type", kind)
		if r.Method != "HEAD" {
			w.Write(data)
		}
		return true
	}
	if r.URL.Path == "/admin/login" {
		b.workspaceLogin(w, r, cfg)
		return true
	}
	s := a.session(r, b.cookieTag)
	if s != nil && s.hash != cfg.WorkspaceHash {
		s = nil
	}
	if s == nil {
		adminRequest := isAdminPath || scheduleRoute || r.URL.Path == "/files" || strings.HasPrefix(r.URL.Path, "/files/") || strings.HasPrefix(r.URL.Path, "/notes/") || r.Header.Get("X-Toolbox-Token") != "" || (r.URL.Query().Get("session") != "" && (r.URL.Path == "/" || r.URL.Path == "/index.html"))
		if adminRequest {
			w.Header().Set("X-Hamster-Session", "expired-admin")
			if r.Method == "GET" && (r.URL.Path == "/" || r.URL.Path == "/files" || r.URL.Path == "/files/" || r.URL.Path == "/notes/" || r.URL.Path == "/notes/notes.html" || r.URL.Path == "/index.html" || r.URL.Path == "/schedules.html") {
				http.Redirect(w, r, "/admin/?reason=expired", http.StatusSeeOther)
				return true
			}
			http.Error(w, "管理员会话已过期，请重新登录", 401)
			return true
		}
		if cfg.WorkspaceEnabled && len(cfg.Roots) == 0 && (r.URL.Path == "/" || r.URL.Path == "/index.html" || r.URL.Path == "/notes/" || r.URL.Path == "/files/") {
			http.Redirect(w, r, "/admin/", http.StatusSeeOther)
			return true
		}
		return false
	}
	if !workspaceSameOrigin(r) {
		http.Error(w, "请从远程工作台操作", 403)
		return true
	}
	api := strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/files/api/")
	if api || r.URL.Path == "/admin/logout" {
		tokenOK := subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Toolbox-Token")), []byte(s.csrf)) == 1
		fileAPI := strings.HasPrefix(r.URL.Path, "/files/api/") && browserPOST(r)
		if r.Method != "POST" || (!tokenOK && !fileAPI) {
			http.Error(w, "远程请求验证失败，请重新打开工作台", 403)
			return true
		}
	}
	if r.URL.Path == "/admin/logout" || r.URL.Path == "/files/api/logout" {
		cookie, _ := r.Cookie("hamster_admin_" + b.cookieTag)
		a.mu.Lock()
		delete(a.sessions, cookie.Value)
		a.mu.Unlock()
		lockWorkspaceNotes(s)
		http.SetCookie(w, &http.Cookie{Name: "hamster_admin_" + b.cookieTag, Path: "/", HttpOnly: true, Secure: browserRequestSecure(r), SameSite: http.SameSiteStrictMode, MaxAge: -1})
		send(w, map[string]bool{"OK": true})
		return true
	}
	if r.URL.Path == "/admin/status" {
		send(w, map[string]bool{"OK": true})
		return true
	}
	if r.URL.Path == "/remote-workspace.js" || r.URL.Path == "/remote-workspace.css" {
		if r.Method != "GET" && r.Method != "HEAD" {
			http.Error(w, "GET required", 405)
			return true
		}
		data, _ := assets.ReadFile("web" + r.URL.Path)
		kind := "text/javascript"
		if strings.HasSuffix(r.URL.Path, ".css") {
			kind = "text/css"
		}
		w.Header().Set("Content-Type", kind+"; charset=utf-8")
		if r.Method != "HEAD" {
			w.Write(data)
		}
		return true
	}
	if isAdminPath {
		http.NotFound(w, r)
		return true
	}
	// A remote browser cannot display windows on the host computer.
	switch r.URL.Path {
	case "/files/api/native-player", "/files/api/open-local", "/files/api/open-with", "/api/dedupe-open":
		http.Error(w, "远程访问请使用浏览器预览；本机播放器和打开方式仅限服务端电脑本机操作", http.StatusForbidden)
		return true
	}
	// Expose only the remote CSRF capability in navigation URLs, never the local token.
	page := r.URL.Path == "/" || r.URL.Path == "/index.html" || r.URL.Path == "/files/" || r.URL.Path == "/notes/" || r.URL.Path == "/notes/notes.html" || r.URL.Path == "/schedules.html"
	if page && r.Method == "GET" && r.URL.Query().Get("session") != s.csrf {
		u := *r.URL
		q := u.Query()
		q.Set("session", s.csrf)
		u.RawQuery = q.Encode()
		http.Redirect(w, r, u.String(), 303)
		return true
	}
	clone := r.Clone(r.Context())
	u := *r.URL
	clone.URL = &u
	clone.Header = r.Header.Clone()
	clone.Header.Set("X-Toolbox-Token", a.owner.token)
	q := clone.URL.Query()
	q.Set("session", a.owner.token)
	clone.URL.RawQuery = q.Encode()
	clone.RemoteAddr = "127.0.0.1:0"
	clone.Header.Del("Cookie")
	clone.AddCookie(&http.Cookie{Name: "hamster_files_" + b.cookieTag, Value: b.localCookie})
	output := workspaceWriter{ResponseWriter: w, localToken: a.owner.token, remoteToken: s.csrf}
	if path := strings.TrimPrefix(r.URL.Path, "/"); path != "" && !strings.HasSuffix(path, ".html") && browserStaticAsset(path) {
		b.static(&output, clone, path)
		return true
	}
	// Separate per-browser notes keys, locks and drafts. Other tools share the
	// existing live task manager, so refreshing a phone does not restart a task.
	if strings.HasPrefix(r.URL.Path, "/api/notes/") {
		s.notes.notesAPI(&output, clone)
		return true
	}
	if page || r.URL.Path == "/files/api/boot" {
		buffer := &workspaceBuffer{header: make(http.Header)}
		a.handler.ServeHTTP(buffer, clone)
		data := buffer.body.Bytes()
		if page && strings.Contains(buffer.header.Get("Content-Type"), "text/html") && buffer.code < 400 {
			data = bytes.Replace(data, []byte("</head>"), []byte(`<link rel="stylesheet" href="/remote-workspace.css"><script src="/remote-workspace.js" defer></script></head>`), 1)
		} else if r.URL.Path == "/files/api/boot" {
			var value map[string]json.RawMessage
			if json.Unmarshal(data, &value) == nil {
				value["ToolboxURL"], _ = json.Marshal("/?session=" + url.QueryEscape(s.csrf))
				value["Remote"] = json.RawMessage("true")
				value["NativePrograms"] = json.RawMessage("false")
				data, _ = json.Marshal(value)
			}
		}
		for key, values := range buffer.header {
			if strings.EqualFold(key, "Set-Cookie") || strings.EqualFold(key, "Content-Length") {
				continue
			}
			w.Header()[key] = values
		}
		output.clean()
		code := buffer.code
		if code == 0 {
			code = 200
		}
		w.WriteHeader(code)
		if r.Method != "HEAD" {
			w.Write(data)
		}
		return true
	}
	a.handler.ServeHTTP(&output, clone)
	return true
}
func (b *fileBrowser) workspaceLogin(w http.ResponseWriter, r *http.Request, cfg sharingConfig) {
	if !browserPOST(r) || !workspaceSameOrigin(r) {
		http.Error(w, "无效请求", 403)
		return
	}
	a := b.workspace
	ip := browserClientIP(r)
	now := time.Now()
	a.mu.Lock()
	for id, s := range a.sessions {
		if now.After(s.expires) || now.Sub(s.last) > 30*time.Minute {
			delete(a.sessions, id)
			lockWorkspaceNotes(s)
		}
	}
	for ip, limit := range a.attempts {
		if now.Sub(limit.Start) > time.Minute {
			delete(a.attempts, ip)
		}
	}
	limit := a.attempts[ip]
	if now.Sub(limit.Start) > time.Minute {
		limit = loginWindow{Start: now}
	}
	limit.Count++
	a.attempts[ip] = limit
	limited := limit.Count > 10 || len(a.attempts) > 512 || len(a.sessions) >= 32
	a.mu.Unlock()
	if limited {
		http.Error(w, "尝试过于频繁或设备过多，请稍后重试", 429)
		return
	}
	select {
	case a.slots <- struct{}{}:
		defer func() { <-a.slots }()
	default:
		http.Error(w, "正在验证其他请求，请稍后重试", 429)
		return
	}
	var req struct{ Password string }
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&req) != nil || len(req.Password) > 512 {
		http.Error(w, "管理员口令无效", 400)
		return
	}
	salt, _ := hex.DecodeString(cfg.WorkspaceSalt)
	expected, _ := hex.DecodeString(cfg.WorkspaceHash)
	if validateWorkspaceConfig(cfg) != nil || subtle.ConstantTimeCompare(sharePassword(req.Password, salt, 600000), expected) != 1 {
		http.Error(w, "管理员口令不正确", 401)
		return
	}
	b.mu.Lock()
	valid := b.config.WorkspaceEnabled && b.config.WorkspaceHash == cfg.WorkspaceHash && b.config.WorkspaceHTTP == cfg.WorkspaceHTTP
	b.mu.Unlock()
	if !valid {
		http.Error(w, "设置已修改，请重新登录", 401)
		return
	}
	id := randomID()
	csrf := randomID()
	n := &server{dataDir: a.owner.dataDir, notes: &notesMemory{idle: notesIdle}, notesDraftName: "draft-remote-" + id + ".json"}
	a.mu.Lock()
	session := &workspaceSession{hash: cfg.WorkspaceHash, csrf: csrf, last: now, expires: now.Add(8 * time.Hour), notes: n}
	a.sessions[id] = session
	session.timer = time.AfterFunc(30*time.Minute, func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.sessions[id] == session {
			remaining := min(30*time.Minute-time.Since(session.last), time.Until(session.expires))
			if remaining > 0 {
				session.timer.Reset(remaining)
				return
			}
			delete(a.sessions, id)
			lockWorkspaceNotes(session)
		}
	})
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "hamster_admin_" + b.cookieTag, Value: id, Path: "/", HttpOnly: true, Secure: browserRequestSecure(r), SameSite: http.SameSiteStrictMode, MaxAge: 8 * 3600})
	send(w, map[string]string{"URL": "/?session=" + url.QueryEscape(csrf)})
}

// Strip internal owner cookies while preserving streaming/range behavior.
type workspaceWriter struct {
	http.ResponseWriter
	localToken, remoteToken string
}

func (w *workspaceWriter) clean() {
	w.Header().Del("Set-Cookie")
	if w.localToken != "" {
		w.Header().Set("Location", strings.ReplaceAll(w.Header().Get("Location"), w.localToken, w.remoteToken))
		if w.Header().Get("Location") == "" {
			w.Header().Del("Location")
		}
	}
}

func (w *workspaceWriter) WriteHeader(code int) {
	w.clean()
	w.ResponseWriter.WriteHeader(code)
}
func (w *workspaceWriter) Write(b []byte) (int, error) {
	w.clean()
	return w.ResponseWriter.Write(b)
}
func (w *workspaceWriter) Flush() {
	w.clean()
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
func (w *workspaceWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

type workspaceBuffer struct {
	header http.Header
	body   bytes.Buffer
	code   int
}

func (w *workspaceBuffer) Header() http.Header { return w.header }
func (w *workspaceBuffer) WriteHeader(code int) {
	if w.code == 0 {
		w.code = code
	}
}
func (w *workspaceBuffer) Write(data []byte) (int, error) {
	if w.code == 0 {
		w.code = 200
	}
	return w.body.Write(data)
}
func (s *server) notesDraftFile() string {
	if s.notesDraftName != "" {
		return s.notesDraftName
	}
	return "draft.json"
}
