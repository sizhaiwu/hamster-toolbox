package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

// A complete distribution binds the inventory of its bundled desktop runtime.
var desktopBundleSHA256 string

type desktopClientStatus struct {
	Supported, Installed   bool
	Version, Mode, Message string
}
type desktopClientManager struct {
	root, version, hash string
	supported           bool
}
type desktopBundleFile struct {
	Path, SHA256 string
	Size         int64
}
type desktopBundle struct {
	Version, Runtime string
	Files            []desktopBundleFile
}

func newDesktopClientManager(program string) *desktopClientManager {
	return &desktopClientManager{root: filepath.Join(filepath.Dir(program), "desktop-runtime"), version: appVersion, hash: strings.ToLower(desktopBundleSHA256), supported: runtime.GOOS == "windows" && runtime.GOARCH == "amd64"}
}
func (s *server) clientManager() *desktopClientManager {
	s.desktopClientMu.Lock()
	defer s.desktopClientMu.Unlock()
	if s.desktopClient == nil {
		p, _ := os.Executable()
		s.desktopClient = newDesktopClientManager(p)
	}
	return s.desktopClient
}
func regularClientFile(path string) bool {
	st, e := os.Lstat(path)
	return e == nil && st.Mode().IsRegular() && st.Mode()&os.ModeSymlink == 0
}
func cleanClientDirectories(path string) error {
	abs, e := filepath.Abs(path)
	if e != nil {
		return e
	}
	for p := abs; ; p = filepath.Dir(p) {
		st, e := os.Lstat(p)
		if e == nil && (!st.IsDir() || st.Mode()&os.ModeSymlink != 0) {
			return errors.New("桌面程序目录不能是链接或普通文件")
		}
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			return e
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	return nil
}
func (m *desktopClientManager) program() string {
	return filepath.Join(m.root, "hamster-desktop-client.exe")
}
func (m *desktopClientManager) inventory() (desktopBundle, error) {
	var bundle desktopBundle
	expected, e := hex.DecodeString(m.hash)
	if !m.supported || e != nil || len(expected) != 32 {
		return bundle, errors.New("当前构建未包含桌面界面")
	}
	if e = cleanClientDirectories(m.root); e != nil {
		return bundle, e
	}
	path := filepath.Join(m.root, "bundle.json")
	if !regularClientFile(path) {
		return bundle, errors.New("桌面程序不完整，请使用完整安装包或便携包")
	}
	f, e := os.Open(path)
	if e != nil {
		return bundle, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, 1<<20))
	if e != nil {
		return bundle, e
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != m.hash || json.Unmarshal(b, &bundle) != nil || bundle.Version != m.version || len(bundle.Files) == 0 || len(bundle.Files) > 1500 {
		return bundle, errors.New("桌面程序清单与工具箱版本不匹配")
	}
	return bundle, nil
}
func (m *desktopClientManager) installed() bool {
	bundle, e := m.inventory()
	if e != nil {
		return false
	}
	required := map[string]bool{"hamster-desktop-client.exe": false, "icudtl.dat": false, "resources.pak": false, "resources/app/main.cjs": false, "resources/app/experience.cjs": false, "resources/app/preload.cjs": false, "resources/app/media.cjs": false, "resources/app/package.json": false}
	for _, file := range bundle.Files {
		if _, ok := required[file.Path]; ok {
			path := filepath.Join(m.root, filepath.FromSlash(file.Path))
			st, e := os.Lstat(path)
			if e != nil || !st.Mode().IsRegular() || st.Size() != file.Size {
				return false
			}
			required[file.Path] = true
		}
	}
	for _, found := range required {
		if !found {
			return false
		}
	}
	return true
}
func (s *server) clientStatus() desktopClientStatus {
	m := s.clientManager()
	out := desktopClientStatus{Supported: m.supported, Installed: m.installed(), Version: m.version}
	s.settingsMu.Lock()
	p, e := s.loadPreferences()
	s.settingsMu.Unlock()
	out.Mode = desktopLaunchMode(p, m.supported)
	switch {
	case e != nil:
		out.Message = e.Error()
	case !out.Supported:
		out.Message = "此平台使用浏览器界面。"
	case out.Installed:
		out.Message = "桌面界面已随工具箱内置，无需联网下载。"
	default:
		out.Message = "当前目录缺少匹配的桌面程序，将使用浏览器。请使用完整安装包或完整便携包。"
	}
	return out
}
func desktopLaunchMode(p preferences, supported bool) string {
	if !supported || p.DesktopLaunch == "browser" {
		return "browser"
	}
	return "desktop"
}
func (s *server) setClientMode(mode string) error {
	if mode != "desktop" && mode != "browser" {
		return errors.New("启动方式无效")
	}
	if mode == "desktop" && !s.clientManager().installed() {
		return errors.New("桌面程序不完整，请使用完整安装包或完整便携包")
	}
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	p, e := s.loadPreferences()
	if e != nil {
		return e
	}
	p.DesktopLaunch = mode
	return s.writePreferences(p)
}
func desktopLoopback(r *http.Request) bool {
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	return e == nil && ip != nil && ip.IsLoopback()
}
func (s *server) desktopClientAPI(w http.ResponseWriter, r *http.Request) {
	if !desktopLoopback(r) {
		http.Error(w, "桌面窗口仅允许服务端电脑本机管理", 403)
		return
	}
	var req struct {
		Action, Mode string
		Paths        []string
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(&req); e != nil {
		fail(w, errors.New("桌面界面请求无效"))
		return
	}
	if e := d.Decode(new(any)); !errors.Is(e, io.EOF) {
		fail(w, errors.New("桌面界面请求无效"))
		return
	}
	switch req.Action {
	case "drop-items":
		if s.browser == nil || len(req.Paths) == 0 || len(req.Paths) > 1000 {
			fail(w, errors.New("请拖入文件或文件夹，单次最多 1000 项"))
			return
		}
		items := []map[string]any{}
		for _, input := range req.Paths {
			p, _, e := s.browser.resolve(r, true, false, input)
			if e != nil {
				fail(w, e)
				return
			}
			ref, e := toolbox.CaptureBrowserItem(p)
			if e != nil {
				fail(w, e)
				return
			}
			st, e := os.Stat(p)
			if e != nil {
				fail(w, e)
				return
			}
			version := randomID()
			s.browser.mu.Lock()
			if len(s.browser.references) >= 8192 {
				s.browser.references = map[string]browserReference{}
			}
			s.browser.references[version] = browserReference{ref, time.Now()}
			s.browser.mu.Unlock()
			items = append(items, map[string]any{"Name": st.Name(), "Path": p, "Dir": st.IsDir(), "Version": version})
		}
		send(w, items)
		return
	case "background":
		if !s.clientManager().supported {
			fail(w, errors.New("此平台暂不支持系统托盘"))
			return
		}
		if !s.desktopTray {
			detachDesktopConsole()
		}
		send(w, map[string]bool{"OK": true})
		return
	case "exit-check":
		send(w, map[string]bool{"CanExit": s.desktopCanExit()})
		return
	case "exit":
		if s.desktopShutdown == nil {
			fail(w, errors.New("此服务暂不支持桌面退出，请从启动终端退出"))
			return
		}
		if !s.beginDesktopExit() {
			fail(w, errors.New("工具箱任务仍在执行，请等待完成或取消；可先退出到系统托盘"))
			return
		}
		send(w, map[string]bool{"OK": true})
		time.AfterFunc(150*time.Millisecond, s.desktopShutdown)
		return
	case "", "status":
	case "mode":
		if e := s.setClientMode(req.Mode); e != nil {
			fail(w, e)
			return
		}
	case "open":
		if e := s.launchDesktopClient("http://" + r.Host + "/?session=" + s.token); e != nil {
			fail(w, e)
			return
		}
	case "browser":
		address := "http://" + r.Host + "/?session=" + s.token
		if e := openDesktopBrowser(address); e != nil {
			fail(w, e)
			return
		}
	default:
		fail(w, errors.New("桌面界面随完整包内置，无需单独安装或下载"))
		return
	}
	send(w, s.clientStatus())
}
func openDesktopBrowser(address string) error {
	if !validDesktopAddress(address) {
		return errors.New("本机工具箱地址无效")
	}
	cmd := openBrowserCommand(address)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
func validDesktopAddress(address string) bool {
	u, e := url.Parse(address)
	return e == nil && u.Scheme == "http" && u.Hostname() == "127.0.0.1" && u.Port() != "" && u.User == nil
}
func (s *server) launchDesktopClient(address string) error {
	if !validDesktopAddress(address) {
		return errors.New("桌面界面只连接本机工具箱地址")
	}
	m := s.clientManager()
	if !m.installed() {
		return errors.New("桌面程序未就绪或版本不匹配")
	}
	profile, e := filepath.Abs(filepath.Join(s.dataDir, "desktop-profile"))
	if e != nil {
		return e
	}
	cmd := exec.Command(m.program(), "--toolbox-url="+address, "--toolbox-profile="+profile)
	if s.desktopTray {
		cmd.Args = append(cmd.Args, "--toolbox-tray")
	}
	cmd.Dir = m.root
	for _, entry := range os.Environ() {
		key := strings.ToUpper(strings.SplitN(entry, "=", 2)[0])
		if !strings.HasPrefix(key, "ELECTRON_") && !strings.HasPrefix(key, "NODE_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	if e = cmd.Start(); e != nil {
		return e
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case e := <-done:
		if e != nil {
			return fmt.Errorf("桌面窗口启动失败：%w", e)
		}
		return nil
	case <-time.After(750 * time.Millisecond):
		return nil
	}
}
func (s *server) openInterface(address string) (string, error) {
	s.settingsMu.Lock()
	p, e := s.loadPreferences()
	s.settingsMu.Unlock()
	fallback := ""
	if e == nil && desktopLaunchMode(p, s.clientManager().supported) == "desktop" {
		if e = s.launchDesktopClient(address); e == nil {
			return "已打开桌面窗口", nil
		}
		fallback = "桌面窗口不可用，已改用浏览器 · "
	}
	cmd := openBrowserCommand(address)
	if e = cmd.Start(); e != nil {
		return "", e
	}
	go cmd.Wait()
	return fallback + "已请求打开默认浏览器 · 未弹出时可使用下方链接", nil
}
