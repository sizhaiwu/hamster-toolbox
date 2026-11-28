package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDesktopCloseExitAPI(t *testing.T) {
	s := &server{dataDir: t.TempDir(), desktopClient: fixtureClientManager(t)}
	done := make(chan struct{}, 1)
	s.desktopShutdown = func() { done <- struct{}{} }
	s.state.Status = "running"
	if w := clientRequest(s, `{"Action":"exit"}`); w.Code == 200 {
		t.Fatal("running task was interrupted")
	}
	if s.desktopClosing {
		t.Fatal("failed exit changed state")
	}
	s.state.Status = "idle"
	s.scheduler = &scheduleManager{active: map[string]context.CancelFunc{"job": func() {}}}
	if s.desktopCanExit() {
		t.Fatal("active scheduler was ignored")
	}
	delete(s.scheduler.active, "job")
	if w := clientRequest(s, `{"Action":"exit"}`); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("backend exit not requested")
	}
	if !s.desktopClosing {
		t.Fatal("new task guard not set")
	}
}

func fixtureClientManager(t *testing.T) *desktopClientManager {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "desktop-runtime")
	files := []desktopBundleFile{}
	for _, p := range []string{"hamster-desktop-client.exe", "icudtl.dat", "resources.pak", "resources/app/main.cjs", "resources/app/experience.cjs", "resources/app/preload.cjs", "resources/app/media.cjs", "resources/app/package.json"} {
		path := filepath.Join(root, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(path), 0700)
		content := []byte("synthetic " + p)
		if e := os.WriteFile(path, content, 0600); e != nil {
			t.Fatal(e)
		}
		h := sha256.Sum256(content)
		files = append(files, desktopBundleFile{p, hex.EncodeToString(h[:]), int64(len(content))})
	}
	b, _ := json.Marshal(desktopBundle{Version: "1.2.3", Runtime: "45.5.0", Files: files})
	os.WriteFile(filepath.Join(root, "bundle.json"), b, 0600)
	h := sha256.Sum256(b)
	return &desktopClientManager{root: root, version: "1.2.3", hash: hex.EncodeToString(h[:]), supported: true}
}
func clientRequest(s *server, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "http://127.0.0.1:12345/api/desktop-client", strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:4000"
	w := httptest.NewRecorder()
	s.desktopClientAPI(w, r)
	return w
}
func TestDesktopClientBundledDefaultAndPreferences(t *testing.T) {
	m := fixtureClientManager(t)
	s := &server{dataDir: t.TempDir(), desktopClient: m}
	out := s.clientStatus()
	if !out.Installed || out.Mode != "desktop" {
		t.Fatal(out)
	}
	if _, e := os.Stat(filepath.Join(s.dataDir, "settings.json")); !os.IsNotExist(e) {
		t.Fatal("status must not write preferences")
	}
	for _, mode := range []string{"browser", "desktop"} {
		w := clientRequest(s, `{"Action":"mode","Mode":"`+mode+`"}`)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		s2 := &server{dataDir: s.dataDir, desktopClient: m}
		if s2.clientStatus().Mode != mode {
			t.Fatal("preference did not survive restart")
		}
	}
	if clientRequest(s, `{"Action":"install"}`).Code == 200 || clientRequest(s, `{"Action":"cancel"}`).Code == 200 {
		t.Fatal("separate installer must be disabled")
	}
}
func TestDesktopClientBundledIntegrity(t *testing.T) {
	for _, kind := range []string{"hash", "version", "missing", "truncated", "unbound", "unsupported"} {
		t.Run(kind, func(t *testing.T) {
			m := fixtureClientManager(t)
			if !m.installed() {
				t.Fatal("fixture invalid")
			}
			switch kind {
			case "hash":
				m.hash = strings.Repeat("0", 64)
			case "version":
				m.version = "9.9.9"
			case "missing":
				os.Remove(m.program())
			case "truncated":
				os.WriteFile(m.program(), []byte("x"), 0600)
			case "unbound":
				m.hash = ""
			case "unsupported":
				m.supported = false
			}
			if m.installed() {
				t.Fatal("invalid runtime accepted")
			}
			s := &server{dataDir: t.TempDir(), desktopClient: m}
			if e := s.setClientMode("desktop"); e == nil {
				t.Fatal("broken desktop selected")
			}
			if e := s.setClientMode("browser"); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestDesktopClientDirectoryLinksAndURLs(t *testing.T) {
	m := fixtureClientManager(t)
	if e := os.Symlink(m.root, filepath.Join(t.TempDir(), "linked")); e == nil {
		base := filepath.Join(t.TempDir(), "links")
		os.MkdirAll(base, 0700)
		link := filepath.Join(base, "runtime")
		if e = os.Symlink(m.root, link); e == nil {
			m.root = link
			if m.installed() {
				t.Fatal("directory link accepted")
			}
		}
	}
	for _, a := range []string{"file:///test", "https://127.0.0.1:1234/", "http://localhost:1234/", "http://127.0.0.1.evil:1234/", "http://user@127.0.0.1:1234/"} {
		if validDesktopAddress(a) {
			t.Fatal(a)
		}
	}
	if !validDesktopAddress("http://127.0.0.1:1234/?session=abc") {
		t.Fatal("loopback rejected")
	}
	if desktopLaunchMode(preferences{}, false) != "browser" {
		t.Fatal("non-Windows fallback")
	}
}
func TestDesktopClientLocalBoundary(t *testing.T) {
	s := &server{dataDir: t.TempDir(), desktopClient: fixtureClientManager(t)}
	r := httptest.NewRequest("POST", "/api/desktop-client", strings.NewReader(`{"Action":"mode","Mode":"browser"}`))
	r.RemoteAddr = "192.168.1.9:4321"
	w := httptest.NewRecorder()
	s.desktopClientAPI(w, r)
	if w.Code != 403 {
		t.Fatal("remote host accepted")
	}
	if clientRequest(s, `{"Action":"status","URL":"https://example.com"}`).Code == 200 {
		t.Fatal("unknown download field accepted")
	}
}
func TestDesktopClientRemoteWorkspaceDenied(t *testing.T) {
	_, b, _ := workspaceFixture(t)
	c := workspaceSignIn(t, b)
	for _, route := range []string{"/api/desktop-client", "/api/desktop-client-import"} {
		w := workspaceCall(b, c, "POST", route, map[string]string{"Action": "open"}, map[string]string{"X-Toolbox-Token": c.csrf})
		if w.Code != 403 {
			t.Fatal(route, w.Code)
		}
	}
}
