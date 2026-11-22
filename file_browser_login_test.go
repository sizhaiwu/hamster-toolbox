package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

func guestLoginFixture(t *testing.T) *fileBrowser {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "共享登录测试.txt"), []byte("synthetic shared content"), 0600); err != nil {
		t.Fatal(err)
	}
	b, err := newFileBrowser(t.TempDir(), "owner-private-session")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := toolbox.NewBrowserScope(root)
	if err != nil {
		t.Fatal(err)
	}
	salt := bytes.Repeat([]byte{0x31}, 24)
	b.config.Salt = hex.EncodeToString(salt)
	b.config.Hash = hex.EncodeToString(sharePassword("Public-Test-Only!", salt, 600000))
	b.config.Roots = []sharedFolder{{ID: "synthetic-share", Name: "演示共享", Path: root}}
	b.scopes = map[string]*toolbox.BrowserScope{"synthetic-share": scope}
	return b
}
func TestGuestLoginPageLoadsAllAssetsBeforeAuthentication(t *testing.T) {
	b := guestLoginFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(b.remote))
	defer srv.Close()
	response, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 {
		t.Fatal(response.StatusCode, err)
	}
	refs := regexp.MustCompile(`<(?:script|link|img)\b[^>]*\b(?:src|href)="([^"]+)"`).FindAllSubmatch(body, -1)
	if len(refs) < 10 {
		t.Fatal("login page did not contain expected assets")
	}
	base, _ := url.Parse(srv.URL + "/")
	for _, ref := range refs {
		u, e := url.Parse(string(ref[1]))
		if e != nil {
			t.Fatal(e)
		}
		u = base.ResolveReference(u)
		if u.Host != base.Host {
			t.Fatal("unexpected external login asset", u)
		}
		asset, e := http.Get(u.String())
		if e != nil {
			t.Fatal(e)
		}
		asset.Body.Close()
		if asset.StatusCode != 200 {
			t.Errorf("first visit cannot load %s before login: HTTP %d", u.Path, asset.StatusCode)
		}
	}
	for _, route := range []string{"/api/boot", "/api/list", "/api/sharing", "/api/version-update", "/api/preferences", "/sharing.json", "/passwords.dat", "/api/notes/status", "/api/notes/unlock", "/api/notes/list", "/notes/", "/notes/notes.js"} {
		r := httptest.NewRequest("POST", route, strings.NewReader(`{}`))
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("X-Hamster-Browser", "1")
		w := httptest.NewRecorder()
		b.remote(w, r)
		if w.Code != 401 {
			t.Fatal("protected route became public", route, w.Code)
		}
	}
}
func TestGuestLoginCanBootstrapAndListWithoutRefresh(t *testing.T) {
	b := guestLoginFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(b.remote))
	defer srv.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	post := func(route string, data any) *http.Response {
		t.Helper()
		body, _ := json.Marshal(data)
		request, _ := http.NewRequest("POST", srv.URL+route, bytes.NewReader(body))
		request.Header.Set("X-Hamster-Browser", "1")
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	response := post("/api/boot", map[string]any{})
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal(response.StatusCode)
	}
	response = post("/api/login", map[string]string{"Password": "Public-Test-Only!"})
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("login failed", response.StatusCode)
	}
	cookies := response.Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("session cookie protections changed")
	}
	// No page GET or reload occurs between login and these protected requests.
	response = post("/api/boot", map[string]any{})
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || !bytes.Contains(body, []byte("synthetic-share")) || bytes.Contains(body, []byte("owner-private-session")) {
		t.Fatal(response.StatusCode, string(body))
	}
	response = post("/api/list?root=synthetic-share", map[string]string{"Path": ""})
	body, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || !bytes.Contains(body, []byte("共享登录测试.txt")) {
		t.Fatal("listing required a refresh", response.StatusCode, string(body))
	}
}

// Optional real-browser acceptance server, isolated on loopback with synthetic
// files and a public test-only password. No saved user sharing is changed.
func TestGuestLoginBrowserAcceptance(t *testing.T) {
	output := os.Getenv("TOOLBOX_GUEST_LOGIN_QA")
	if output == "" {
		t.Skip("set TOOLBOX_GUEST_LOGIN_QA to an isolated session JSON path")
	}
	b := guestLoginFixture(t)
	// Public synthetic cards make phone grid density and long names visible in
	// the optional browser acceptance session, without reading user resources.
	image, err := os.ReadFile("samples/assets/color-card.jpg")
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 18; i++ {
		name := fmt.Sprintf("演示图片 %02d · 手机网格与长文件名.jpg", i)
		if err := os.WriteFile(filepath.Join(b.config.Roots[0].Path, name), image, 0600); err != nil {
			t.Fatal(err)
		}
	}
	video, err := os.ReadFile("samples/assets/motion-demo.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b.config.Roots[0].Path, "00 - 演示视频.mp4"), video, 0600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(b.remote))
	defer srv.Close()
	data, _ := json.Marshal(map[string]string{"URL": srv.URL + "/", "Password": "Public-Test-Only!", "StopFile": output + ".stop"})
	if err := os.WriteFile(output, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("isolated browser acceptance server ready")
	deadline := time.NewTimer(3 * time.Minute)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("browser acceptance did not complete before deadline")
		case <-tick.C:
			if _, err := os.Stat(output + ".stop"); err == nil {
				return
			}
		}
	}
}
