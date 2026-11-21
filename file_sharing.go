package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

type sharedFolder struct {
	ID, Name, Path string
	Writable       bool
}

const maxSharedFolders = 128
const defaultSharingPort = 18788

func sharingListenError(port int, cause error, state string) error {
	reason := "无法开启，请检查系统网络限制后重试"
	switch {
	case errors.Is(cause, syscall.EADDRINUSE):
		reason = "已被其他程序的服务或对外连接占用，请更换端口或等待占用释放"
	case errors.Is(cause, syscall.EACCES), errors.Is(cause, syscall.EPERM):
		reason = "被系统拒绝，可能属于系统保留端口或受到权限策略限制，请换用其他端口"
	}
	return fmt.Errorf("共享端口 %d %s；%s。系统原因：%w", port, reason, state, cause)
}

type sharingConfig struct {
	Enabled        bool
	OverlayEnabled bool
	PublicURL      string
	TrustedProxies []string
	Port           int
	Salt, Hash     string
	Roots          []sharedFolder
}
type guestSession struct{ Expires time.Time }
type browserReference struct {
	Ref *toolbox.BrowserItemRef
	At  time.Time
}
type loginWindow struct {
	Start time.Time
	Count int
}
type fileBrowser struct {
	diskThumbIndex                   map[string]diskThumbnailEntry
	diskThumbBytes                   int64
	uploadMu                         sync.Mutex
	diskCacheMu                      sync.Mutex
	listingMu                        sync.Mutex
	listingEpoch                     uint64
	directorySizeRunning             bool
	listings                         map[string]directoryListingSnapshot
	mu                               sync.Mutex
	configuration                    sync.Mutex
	revision                         uint64
	operations                       sync.Mutex
	explorerSettings                 sync.Mutex
	config                           sharingConfig
	scopes                           map[string]*toolbox.BrowserScope
	sessions                         map[string]guestSession
	logins                           map[string]loginWindow
	server                           *http.Server
	dataDir, ownerToken, localCookie string
	cookieTag                        string
	thumbSlots, loginSlots           chan struct{}
	references                       map[string]browserReference
	sharingError                     string
	unavailable                      map[string]string
	listen                           func(string, string) (net.Listener, error)
	openDirectory                    func(string) error
	openFileWith                     func(context.Context, string, string) error
	openFileWithMu                   sync.Mutex
	media                            *browserMediaState
	native                           *nativePlayerManager
}

func newFileBrowser(dataDir, ownerToken string) (*fileBrowser, error) {
	b := &fileBrowser{dataDir: dataDir, ownerToken: ownerToken, localCookie: randomID(), config: sharingConfig{Port: defaultSharingPort, Roots: []sharedFolder{}}, scopes: map[string]*toolbox.BrowserScope{}, sessions: map[string]guestSession{}, logins: map[string]loginWindow{}, thumbSlots: make(chan struct{}, 2), loginSlots: make(chan struct{}, 2)}
	b.listen = net.Listen
	b.openDirectory = openNativeDirectory
	b.openFileWith = openNativeFileWith
	b.cookieTag = randomID()[:12]
	b.references = map[string]browserReference{}
	data, e := os.ReadFile(filepath.Join(dataDir, "sharing.json"))
	if e != nil && !os.IsNotExist(e) {
		return b, e
	}
	if e == nil {
		var loaded sharingConfig
		if e = json.Unmarshal(data, &loaded); e != nil || loaded.Port < 1024 || loaded.Port > 65535 || len(loaded.Roots) > maxSharedFolders {
			return b, errors.New("共享配置损坏，未开启网络服务；本地功能仍可使用")
		}
		b.config = loaded
		publicURL, proxies, e := validateSharingNetwork(loaded.PublicURL, loaded.TrustedProxies)
		if e != nil {
			b.config.Enabled = false
			return b, errors.New("远程访问配置无效，未开启共享；请在本机共享设置中重新保存")
		}
		b.config.PublicURL = publicURL
		b.config.TrustedProxies = proxies
	}
	return b, nil // The caller starts saved sharing after local initialization.
}

// Revalidate every saved root on startup. Missing disks remain in the saved
// list, but are never exposed to guests until the owner retries the settings.
func (b *fileBrowser) startSavedSharing() error {
	b.configuration.Lock()
	defer b.configuration.Unlock()
	b.mu.Lock()
	cfg := b.config
	running := b.server != nil
	b.mu.Unlock()
	if !cfg.Enabled || running {
		return nil
	}
	salt, se := hex.DecodeString(cfg.Salt)
	hash, he := hex.DecodeString(cfg.Hash)
	var e error
	if se != nil || he != nil || len(salt) != 24 || len(hash) != 32 {
		e = errors.New("共享口令配置无效，请在本机重新设置口令")
	}
	next, scopes, unavailable, rootErr := prepareSharedRoots(cfg.Roots, cfg.Roots, true)
	if e == nil {
		e = rootErr
	}
	if e == nil && len(scopes) == 0 {
		e = errors.New("没有可用的共享目录，请连接资源磁盘后重新保存共享设置")
	}
	var listener net.Listener
	if e == nil {
		listener, e = b.listen("tcp4", net.JoinHostPort("0.0.0.0", strconv.Itoa(cfg.Port)))
		if e != nil {
			e = sharingListenError(cfg.Port, e, "未开启共享，已保存的设置保留")
		}
	}
	b.mu.Lock()
	b.unavailable = unavailable
	if e != nil {
		b.sharingError = e.Error()
	} else {
		b.config.Roots = next
		b.scopes = scopes
		b.sharingError = ""
	}
	b.mu.Unlock()
	if e != nil {
		return e
	}
	b.serveSharing(listener)
	return nil
}

func prepareSharedRoots(requested, previous []sharedFolder, allowMissing bool) ([]sharedFolder, map[string]*toolbox.BrowserScope, map[string]string, error) {
	roots := []sharedFolder{}
	scopes := map[string]*toolbox.BrowserScope{}
	unavailable := map[string]string{}
	seen := map[string]bool{}
	for _, root := range requested {
		// IDs come from the saved path, never from a client's supplied ID.
		var prior *sharedFolder
		for i := range previous {
			if sameSharedPath(previous[i].Path, root.Path) {
				prior = &previous[i]
				break
			}
		}
		scope, e := toolbox.NewBrowserScope(root.Path)
		if e != nil && (!allowMissing || prior == nil) {
			return nil, nil, nil, fmt.Errorf("共享目录：%w", e)
		}
		p := root.Path
		if scope != nil && e == nil {
			p = scope.Root
			for i := range previous {
				if sameSharedPath(previous[i].Path, p) {
					prior = &previous[i]
					break
				}
			}
		}
		key := filepath.Clean(p)
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if seen[key] {
			return nil, nil, nil, errors.New("同一目录不要重复添加")
		}
		seen[key] = true
		name := strings.TrimSpace(root.Name)
		if name == "" {
			name = filepath.Base(p)
		}
		if len([]rune(name)) > 80 {
			return nil, nil, nil, errors.New("共享名称不能超过80字")
		}
		id := randomID()
		if prior != nil && validShareID(prior.ID) {
			id = prior.ID
		}
		if _, duplicate := scopes[id]; duplicate {
			return nil, nil, nil, errors.New("共享目录标识重复，请重新添加目录")
		}
		if _, duplicate := unavailable[id]; duplicate {
			return nil, nil, nil, errors.New("共享目录标识重复，请重新添加目录")
		}
		roots = append(roots, sharedFolder{id, name, p, root.Writable})
		if e != nil {
			unavailable[id] = "目录不可用，请连接磁盘或检查路径后重新保存"
		} else {
			scopes[id] = scope
		}
	}
	return roots, scopes, unavailable, nil
}

func validShareID(id string) bool {
	v, e := hex.DecodeString(id)
	return e == nil && len(v) == 24
}
func sameSharedPath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func (b *fileBrowser) serveSharing(listener net.Listener) {
	srv := &http.Server{Handler: http.HandlerFunc(b.remote), ReadHeaderTimeout: 8 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	b.mu.Lock()
	b.server = srv
	b.mu.Unlock()
	go func() {
		e := srv.Serve(listener)
		b.mu.Lock()
		if b.server == srv {
			b.server = nil
			if e != nil && !errors.Is(e, http.ErrServerClosed) {
				b.sharingError = "共享服务已停止，请重新保存共享设置以重试"
			}
		}
		b.mu.Unlock()
	}()
}

// PBKDF2-HMAC-SHA256, a single 32-byte output block. The storage work factor
// follows OWASP's 600,000-iteration recommendation. Never store plaintext.
func sharePassword(password string, salt []byte, iterations int) []byte {
	mac := hmac.New(sha256.New, []byte(password))
	mac.Write(salt)
	mac.Write([]byte{0, 0, 0, 1})
	u := mac.Sum(nil)
	result := append([]byte(nil), u...)
	for i := 1; i < iterations; i++ {
		mac.Reset()
		mac.Write(u)
		u = mac.Sum(u[:0])
		for j := range result {
			result[j] ^= u[j]
		}
	}
	return result
}
func (b *fileBrowser) status() map[string]any {
	b.mu.Lock()
	defer b.mu.Unlock()
	urls := []string{}
	if b.server != nil {
		if addresses, e := net.InterfaceAddrs(); e == nil {
			for _, address := range addresses {
				ip, _, e := net.ParseCIDR(address.String())
				if e == nil && ip.To4() != nil && (ip.IsPrivate() || b.config.OverlayEnabled && overlayIPv4(ip)) {
					urls = append(urls, "http://"+net.JoinHostPort(ip.String(), strconv.Itoa(b.config.Port))+"/")
				}
			}
		}
	}
	unavailable := map[string]string{}
	for id, reason := range b.unavailable {
		unavailable[id] = reason
	}
	return map[string]any{"MaxRoots": maxSharedFolders, "Enabled": b.server != nil, "StartupEnabled": b.config.Enabled, "OverlayEnabled": b.config.OverlayEnabled, "PublicURL": b.config.PublicURL, "TrustedProxies": append([]string{}, b.config.TrustedProxies...), "Error": b.sharingError, "Unavailable": unavailable, "Port": b.config.Port, "PasswordSet": b.config.Hash != "", "Roots": append([]sharedFolder{}, b.config.Roots...), "URLs": urls}
}
func (b *fileBrowser) saveSharing(r *http.Request) error {
	b.configuration.Lock()
	defer b.configuration.Unlock()
	var req struct {
		Enabled        bool
		OverlayEnabled bool
		PublicURL      string
		TrustedProxies []string
		Port           int
		Password       string
		Roots          []sharedFolder
	}
	if e := json.NewDecoder(r.Body).Decode(&req); e != nil {
		return errors.New("共享设置无效")
	}
	if req.Port < 1024 || req.Port > 65535 {
		return errors.New("端口须为1024–65535")
	}
	if len(req.Roots) > maxSharedFolders {
		return fmt.Errorf("最多共享%d个资源目录", maxSharedFolders)
	}
	b.mu.Lock()
	current := b.config
	b.mu.Unlock()
	publicURL, proxies, networkErr := validateSharingNetwork(req.PublicURL, req.TrustedProxies)
	if networkErr != nil {
		return networkErr
	}
	next := sharingConfig{Enabled: req.Enabled, OverlayEnabled: req.OverlayEnabled, PublicURL: publicURL, TrustedProxies: proxies, Port: req.Port, Salt: current.Salt, Hash: current.Hash, Roots: []sharedFolder{}}
	if req.Password != "" {
		if len([]rune(req.Password)) < 8 || len(req.Password) > 512 {
			return errors.New("访问密码至少8个字符，最多512字节")
		}
		salt := make([]byte, 24)
		if _, e := rand.Read(salt); e != nil {
			return e
		}
		next.Salt = hex.EncodeToString(salt)
		next.Hash = hex.EncodeToString(sharePassword(req.Password, salt, 600000))
	}
	roots, scopes, unavailable, e := prepareSharedRoots(req.Roots, current.Roots, true)
	if e != nil {
		return e
	}
	next.Roots = roots
	if req.Enabled && (next.Hash == "" || len(next.Roots) == 0) {
		return errors.New("开启前请设置密码，并添加至少一个共享目录")
	}
	if req.Enabled {
		salt, se := hex.DecodeString(next.Salt)
		hash, he := hex.DecodeString(next.Hash)
		if se != nil || he != nil || len(salt) != 24 || len(hash) != 32 {
			return errors.New("共享口令配置无效，请重新输入访问口令后保存")
		}
	}
	data, e := json.MarshalIndent(next, "", "  ")
	if e != nil {
		return e
	}
	if e = os.MkdirAll(b.dataDir, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(b.dataDir, ".sharing-*.json")
	if e != nil {
		return e
	}
	temp := f.Name()
	defer os.Remove(temp)
	if _, e = f.Write(data); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	// Bind before touching an existing listener when the port changes.
	var listener net.Listener
	b.mu.Lock()
	samePort := b.server != nil && b.config.Port == req.Port
	b.mu.Unlock()
	if req.Enabled && !samePort {
		listener, e = b.listen("tcp4", net.JoinHostPort("0.0.0.0", strconv.Itoa(req.Port)))
		if e != nil {
			return sharingListenError(req.Port, e, "原设置未变")
		}
	}
	if e = os.Rename(temp, filepath.Join(b.dataDir, "sharing.json")); e != nil {
		if listener != nil {
			listener.Close()
		}
		return e
	}
	b.mu.Lock()
	old := b.server
	b.server = nil
	b.config = next
	b.revision++
	b.scopes = scopes
	b.unavailable = unavailable
	b.sharingError = ""
	b.sessions = map[string]guestSession{}
	if b.media != nil {
		b.media.thumbnails.clear()
		b.media.images.clear()
		b.media.info.clear()
		b.media.hover.clear()
	}
	b.mu.Unlock()
	if old != nil {
		old.Close()
	}
	if req.Enabled {
		if samePort {
			listener, e = b.listen("tcp4", net.JoinHostPort("0.0.0.0", strconv.Itoa(req.Port)))
			if e != nil {
				err := sharingListenError(req.Port, e, "设置已保存，但共享已关闭，请重新保存以重试")
				b.mu.Lock()
				b.sharingError = err.Error()
				b.mu.Unlock()
				return err
			}
		}
		b.serveSharing(listener)
	}
	return nil
}
func browserPOST(r *http.Request) bool {
	return r.Method == "POST" && r.Header.Get("X-Hamster-Browser") == "1" && (r.Header.Get("Origin") == "" || r.Header.Get("Origin") == browserRequestOrigin(r))
}
func (b *fileBrowser) guestOK(r *http.Request) bool {
	c, e := r.Cookie("hamster_guest_" + b.cookieTag)
	if e != nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	s, ok := b.sessions[c.Value]
	if !ok || time.Now().After(s.Expires) {
		delete(b.sessions, c.Value)
		return false
	}
	return true
}
func (b *fileBrowser) login(w http.ResponseWriter, r *http.Request) {
	if !browserPOST(r) {
		http.Error(w, "无效请求", 403)
		return
	}
	var req struct{ Password string }
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&req) != nil || len(req.Password) > 512 {
		http.Error(w, "密码无效", 400)
		return
	}
	ip := browserClientIP(r)
	now := time.Now()
	b.mu.Lock()
	window := b.logins[ip]
	if now.Sub(window.Start) > time.Minute {
		window = loginWindow{Start: now}
	}
	window.Count++
	if len(b.logins) > 512 {
		for k, v := range b.logins {
			if now.Sub(v.Start) > time.Minute {
				delete(b.logins, k)
			}
		}
	}
	if len(b.logins) > 512 {
		b.mu.Unlock()
		http.Error(w, "稍后再试", 429)
		return
	}
	b.logins[ip] = window
	cfg := b.config
	revision := b.revision
	b.mu.Unlock()
	if window.Count > 5 {
		http.Error(w, "尝试过多，请1分钟后重试", 429)
		return
	}
	select {
	case b.loginSlots <- struct{}{}:
		defer func() { <-b.loginSlots }()
	default:
		http.Error(w, "登录繁忙，请稍后重试", 429)
		return
	}
	salt, se := hex.DecodeString(cfg.Salt)
	expected, he := hex.DecodeString(cfg.Hash)
	if se != nil || he != nil || len(expected) != 32 || subtle.ConstantTimeCompare(sharePassword(req.Password, salt, 600000), expected) != 1 {
		http.Error(w, "访问密码不正确", 401)
		return
	}
	b.mu.Lock()
	if b.revision != revision || b.config.Hash != cfg.Hash || b.config.Salt != cfg.Salt {
		b.mu.Unlock()
		http.Error(w, "共享设置已更新，请重试", 401)
		return
	}
	for k, s := range b.sessions {
		if now.After(s.Expires) {
			delete(b.sessions, k)
		}
	}
	if len(b.sessions) >= 64 {
		b.mu.Unlock()
		http.Error(w, "连接设备过多", 429)
		return
	}
	token := randomID()
	b.sessions[token] = guestSession{now.Add(8 * time.Hour)}
	b.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "hamster_guest_" + b.cookieTag, Value: token, Path: "/", HttpOnly: true, Secure: browserRequestSecure(r), SameSite: http.SameSiteStrictMode, MaxAge: 8 * 3600})
	send(w, map[string]bool{"OK": true})
}
func (b *fileBrowser) local(w http.ResponseWriter, r *http.Request) {
	securityBrowserHeaders(w)
	if r.URL.Path == "/files/" && r.URL.Query().Get("session") == b.ownerToken && b.ownerToken != "" {
		http.SetCookie(w, &http.Cookie{Name: "hamster_files_" + b.cookieTag, Value: b.localCookie, Path: "/files/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	}
	c, e := r.Cookie("hamster_files_" + b.cookieTag)
	authorized := e == nil && hmac.Equal([]byte(c.Value), []byte(b.localCookie))
	if r.URL.Path == "/files/" && r.URL.Query().Get("session") == b.ownerToken && b.ownerToken != "" {
		authorized = true
	}
	if !authorized {
		http.Error(w, "请从本机工具箱打开文件浏览", 403)
		return
	}
	b.dispatch(w, r, true, strings.TrimPrefix(r.URL.Path, "/files/"))
}
func (b *fileBrowser) remote(w http.ResponseWriter, r *http.Request) {
	securityBrowserHeaders(w)
	peer, _, e := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(peer)
	b.mu.Lock()
	cfg := b.config
	b.mu.Unlock()
	trustedProxy := cfg.PublicURL != "" && isSharingProxy(ip, cfg.TrustedProxies)
	if e != nil || ip == nil || !(ip.IsLoopback() || ip.IsPrivate() || cfg.OverlayEnabled && overlayIPv4(ip) || trustedProxy) {
		http.Error(w, "仅接受内网或已启用的组网地址；请在本机共享设置检查虚拟组网访问选项", 403)
		return
	}
	r = sharingProxyRequest(r, cfg, trustedProxy)
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "api/login" {
		b.login(w, r)
		return
	}
	if browserStaticAsset(path) {
		b.static(w, r, path)
		return
	}
	if !b.guestOK(r) {
		http.Error(w, "请先输入访问密码", 401)
		return
	}
	b.dispatch(w, r, false, path)
}
