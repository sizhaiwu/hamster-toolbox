package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSharingPasswordDerivation(t *testing.T) {
	for _, v := range []struct {
		n    int
		hash string
	}{{1, "120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b"}, {2, "ae4d0c95af6b46d32d0adff928f06dd02a303f8ef3c251dfd6e2d85a95474c43"}} {
		if hex.EncodeToString(sharePassword("password", []byte("salt"), v.n)) != v.hash {
			t.Fatal("PBKDF2 test vector failed", v.n)
		}
	}
}

func TestOwnerBrowserCanListDriveRoots(t *testing.T) {
	b, e := newFileBrowser(t.TempDir(), "owner-root-test")
	if e != nil {
		t.Fatal(e)
	}
	cwd, e := os.Getwd()
	if e != nil {
		t.Fatal(e)
	}
	seen := map[string]bool{}
	for _, path := range []string{os.TempDir(), cwd} {
		root := filepath.VolumeName(path) + string(filepath.Separator)
		if seen[root] {
			continue
		}
		seen[root] = true
		t.Run(root, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"Path": root})
			r := httptest.NewRequest("POST", "/files/api/list", bytes.NewReader(body))
			r.Header.Set("X-Hamster-Browser", "1")
			r.AddCookie(&http.Cookie{Name: "hamster_files_" + b.cookieTag, Value: b.localCookie})
			w := httptest.NewRecorder()
			b.local(w, r)
			if w.Code != 200 {
				t.Fatalf("opening drive root returned %d: %s", w.Code, w.Body.String())
			}
			var page struct {
				Path, Parent string
				Entries      []browserEntry
			}
			if e := json.Unmarshal(w.Body.Bytes(), &page); e != nil {
				t.Fatal(e)
			}
			if page.Path != root || page.Parent != "" {
				t.Fatal("drive root navigation returned the wrong location")
			}
		})
	}
}
func TestSharedBrowserAuthenticationIsolationAndFileOperations(t *testing.T) {
	root := t.TempDir()
	data := t.TempDir()
	source := filepath.Join(root, "演示 [100%].txt")
	os.WriteFile(source, []byte("demo-content"), 0600)
	b, e := newFileBrowser(data, "owner-only-token")
	if e != nil {
		t.Fatal(e)
	}
	save := func(write bool) {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"Port": 8788, "Password": "Public-Test-Password!", "Roots": []sharedFolder{{Name: "演示", Path: root, Writable: write}}})
		if e := b.saveSharing(httptest.NewRequest("POST", "/", bytes.NewReader(body))); e != nil {
			t.Fatal(e)
		}
	}
	save(false)
	stored, _ := os.ReadFile(filepath.Join(data, "sharing.json"))
	if bytes.Contains(stored, []byte("Public-Test-Password!")) {
		t.Fatal("plaintext password saved")
	}
	reloaded, e := newFileBrowser(data, "different-owner")
	if e != nil || reloaded.server != nil {
		t.Fatal("sharing must stay off after restart", e)
	}
	srv := httptest.NewServer(http.HandlerFunc(b.remote))
	defer srv.Close()
	publicScript, e := http.Get(srv.URL + "/explorer-tabs.js")
	if e != nil {
		t.Fatal(e)
	}
	publicScript.Body.Close()
	if publicScript.StatusCode != 200 {
		t.Fatal("guest login page cannot load tab support script", publicScript.StatusCode)
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	do := func(method, path string, body any, origin string) (int, []byte) {
		t.Helper()
		var reader io.Reader
		if body != nil {
			if raw, ok := body.([]byte); ok {
				reader = bytes.NewReader(raw)
			} else {
				v, _ := json.Marshal(body)
				reader = bytes.NewReader(v)
			}
		}
		r, _ := http.NewRequest(method, srv.URL+path, reader)
		r.Header.Set("X-Hamster-Browser", "1")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		response, e := client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		v, _ := io.ReadAll(response.Body)
		return response.StatusCode, v
	}
	if code, _ := do("POST", "/api/boot", map[string]any{}, ""); code != 401 {
		t.Fatal("unauthenticated request accepted", code)
	}
	if code, _ := do("POST", "/api/login", map[string]string{"Password": "wrong"}, ""); code != 401 {
		t.Fatal("bad password accepted")
	}
	login := func() {
		t.Helper()
		if code, out := do("POST", "/api/login", map[string]string{"Password": "Public-Test-Password!"}, ""); code != 200 {
			t.Fatal(code, string(out))
		}
	}
	login()
	code, body := do("POST", "/api/boot", map[string]any{}, "")
	if code != 200 || bytes.Contains(body, []byte(root)) || bytes.Contains(body, []byte("owner-only-token")) || bytes.Contains(body, []byte(b.config.Hash)) {
		t.Fatal("remote boot leaked private configuration", code, string(body))
	}
	rootID := b.config.Roots[0].ID
	endpoint := func(route string) string { return route + "?root=" + url.QueryEscape(rootID) }
	if code, out := do("POST", endpoint("/api/list"), map[string]string{"Path": ""}, ""); code != 200 || !bytes.Contains(out, []byte("演示")) {
		t.Fatal(code, string(out))
	}
	for _, bad := range []string{"../outside.txt", `..\outside.txt`, `C:\Windows\win.ini`, "file.txt:stream"} {
		if code, _ := do("GET", endpoint("/raw")+"&path="+url.QueryEscape(bad), nil, ""); code == 200 {
			t.Fatal("escaped share root", bad)
		}
	}
	for _, route := range []string{"/api/plan", "/api/passwords", "/api/sharing", "/files/api/sharing"} {
		if code, _ := do("POST", route, map[string]bool{"Enabled": true}, ""); code == 200 {
			t.Fatal("management API exposed on LAN", route)
		}
	}
	if code, _ := do("POST", endpoint("/api/mutate"), map[string]any{"Action": "delete", "Path": filepath.Base(source), "Permanent": true}, ""); code == 200 {
		t.Fatal("read-only guest deleted file")
	}
	if _, e := os.Stat(source); e != nil {
		t.Fatal("read-only operation changed source")
	}
	request, _ := http.NewRequest("GET", srv.URL+endpoint("/raw")+"&path="+url.QueryEscape(filepath.Base(source)), nil)
	request.Header.Set("Range", "bytes=0-3")
	response, e := client.Do(request)
	if e != nil {
		t.Fatal(e)
	}
	out, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 206 || string(out) != "demo" {
		t.Fatal("range playback/download support", response.StatusCode, string(out))
	}
	save(true)
	if code, _ := do("POST", "/api/boot", map[string]any{}, ""); code != 401 {
		t.Fatal("old session survived permission update")
	}
	rootID = b.config.Roots[0].ID
	login()
	if code, _ := do("POST", endpoint("/api/mutate"), map[string]string{"Action": "mkdir", "Path": "", "Name": "拒绝跨站"}, "http://untrusted.example"); code != 403 {
		t.Fatal("cross-origin write accepted", code)
	}
	mutate := func(v map[string]any) {
		t.Helper()
		if v["Action"] != "mkdir" {
			source := v["Path"].(string)
			parent := filepath.Dir(source)
			if parent == "." {
				parent = ""
			}
			code, body := do("POST", endpoint("/api/list"), map[string]string{"Path": parent}, "")
			var page struct{ Entries []browserEntry }
			if code != 200 || json.Unmarshal(body, &page) != nil {
				t.Fatal("cannot capture selection", code)
			}
			for _, item := range page.Entries {
				if filepath.ToSlash(item.Path) == filepath.ToSlash(source) {
					v["Version"] = item.Version
				}
			}
		}
		if code, out := do("POST", endpoint("/api/mutate"), v, ""); code != 200 {
			t.Fatal(code, string(out))
		}
	}
	mutate(map[string]any{"Action": "mkdir", "Path": "", "Name": "新文件夹"})
	upload := endpoint("/api/upload") + "&path=&name=" + url.QueryEscape("上传.txt")
	if code, out := do("POST", upload, []byte("uploaded bytes"), ""); code != 200 {
		t.Fatal(code, string(out))
	}
	if code, _ := do("POST", upload, []byte("overwrite attempt"), ""); code == 200 {
		t.Fatal("upload overwrote existing file")
	}
	mutate(map[string]any{"Action": "rename", "Path": "上传.txt", "Name": "改名.txt"})
	mutate(map[string]any{"Action": "move", "Path": "改名.txt", "Target": "新文件夹/改名.txt"})
	if content, e := os.ReadFile(filepath.Join(root, "新文件夹", "改名.txt")); e != nil || string(content) != "uploaded bytes" {
		t.Fatal("file operations lost data", e)
	}
	mutate(map[string]any{"Action": "delete", "Path": "新文件夹", "Permanent": true})
	if _, e := os.Stat(filepath.Join(root, "新文件夹")); !os.IsNotExist(e) {
		t.Fatal("folder not deleted", e)
	}
	if code, _ := do("POST", endpoint("/api/mutate"), map[string]any{"Action": "delete", "Path": "", "Permanent": true}, ""); code == 200 {
		t.Fatal("shared root deleted")
	}
	if _, e := os.Stat(root); e != nil {
		t.Fatal("root missing")
	}
	// Active documents must download as inert bytes; the text viewer receives
	// text/plain and renders text nodes rather than executing uploaded HTML.
	os.WriteFile(filepath.Join(root, "page.html"), []byte("<script>alert('no')</script>"), 0600)
	raw, _ := http.NewRequest("GET", srv.URL+endpoint("/raw")+"&path=page.html", nil)
	response, e = client.Do(raw)
	if e != nil {
		t.Fatal(e)
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if !strings.HasPrefix(response.Header.Get("Content-Disposition"), "attachment") || response.Header.Get("Content-Type") != "application/octet-stream" || !strings.Contains(response.Header.Get("Content-Security-Policy"), "sandbox") {
		t.Fatal("active document served as executable HTML")
	}
	if code, out := do("GET", endpoint("/text")+"&path=page.html", nil, ""); code != 200 || !bytes.Contains(out, []byte("<script>")) {
		t.Fatal("text source preview failed", code)
	}
}
func TestOwnerBrowserCookieAndSharingLifecycle(t *testing.T) {
	b, _ := newFileBrowser(t.TempDir(), "owner-capability")
	srv := httptest.NewServer(http.HandlerFunc(b.local))
	defer srv.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	response, e := client.Get(srv.URL + "/files/")
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("owner auth bypass")
	}
	response, e = client.Get(srv.URL + "/files/?session=owner-capability")
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("owner page failed", response.StatusCode)
	}
	response, e = client.Get(srv.URL + "/files/explorer.js")
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("authenticated static resource failed")
	}
	var bound net.Listener
	b.listen = func(_, _ string) (net.Listener, error) {
		var e error
		bound, e = net.Listen("tcp4", "127.0.0.1:0")
		return bound, e
	}
	root := t.TempDir()
	v, _ := json.Marshal(map[string]any{"Enabled": true, "Port": 8788, "Password": "Synthetic-Test-Password!", "Roots": []sharedFolder{{Name: "Demo", Path: root}}})
	if e := b.saveSharing(httptest.NewRequest("POST", "/", bytes.NewReader(v))); e != nil {
		t.Fatal(e)
	}
	response, e = http.Get("http://" + bound.Addr().String() + "/")
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("shared listener failed")
	}
	v, _ = json.Marshal(map[string]any{"Enabled": false, "Port": 8788, "Roots": []sharedFolder{{Name: "Demo", Path: root}}})
	if e := b.saveSharing(httptest.NewRequest("POST", "/", bytes.NewReader(v))); e != nil {
		t.Fatal(e)
	}
	if b.status()["Enabled"] != false {
		t.Fatal("listener still enabled")
	}
	if conn, e := net.Dial("tcp4", bound.Addr().String()); e == nil {
		conn.Close()
		t.Fatal("closed sharing port still accepts connections")
	}
	if strings.Contains(fmtBrowserStatus(b), "owner-capability") {
		t.Fatal("sharing status leaked owner token")
	}
}
func fmtBrowserStatus(b *fileBrowser) string { v, _ := json.Marshal(b.status()); return string(v) }

func TestFloatingMediaAssetsAvailableForLocalAndSharedPages(t *testing.T) {
	b, err := newFileBrowser(t.TempDir(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	for _, asset := range []string{"explorer-floating.js", "explorer-floating.css"} {
		for _, local := range []bool{true, false} {
			r := httptest.NewRequest("GET", "/"+asset, nil)
			r.RemoteAddr = "127.0.0.1:12345"
			w := httptest.NewRecorder()
			if local {
				b.dispatch(w, r, true, asset)
			} else {
				b.remote(w, r)
			}
			if w.Code != 200 || w.Body.Len() == 0 || !strings.Contains(w.Header().Get("Content-Type"), map[bool]string{true: "javascript", false: "css"}[strings.HasSuffix(asset, ".js")]) {
				t.Fatal("floating assets unavailable", asset, local, w.Code)
			}
		}
	}
}
