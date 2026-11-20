package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestSharingNetworkValidationAndPeerIsolation(t *testing.T) {
	for _, address := range []string{"http://files.example.com", "https://user:secret@files.example.com", "https://files.example.com/share", "https://files.example.com?token=secret", "https://*.example.com", "https://files.example.com:99999"} {
		if _, _, e := validateSharingNetwork(address, nil); e == nil {
			t.Fatal("invalid external URL accepted", address)
		}
	}
	if got, _, e := validateSharingNetwork(" https://FILES.example.com:9443/ ", []string{"100.64.0.2", "192.168.10.0/24"}); e != nil || got != "https://files.example.com:9443" {
		t.Fatal("valid external URL rejected", got, e)
	}
	if got, _, e := validateSharingNetwork("https://files.example.com:443/", nil); e != nil || got != "https://files.example.com" {
		t.Fatal("default HTTPS port must match the browser Origin", got, e)
	}
	for _, proxy := range []string{"0.0.0.0/0", "::/0", "0.0.0.0", "https://proxy.example.com", "not-an-ip"} {
		if _, _, e := validateSharingNetwork("", []string{proxy}); e == nil {
			t.Fatal("unsafe proxy accepted", proxy)
		}
	}
	b, _ := newFileBrowser(t.TempDir(), "owner")
	for _, tc := range []struct {
		peer    string
		overlay bool
		code    int
	}{{"100.64.0.1:1234", false, 403}, {"100.64.0.1:1234", true, 200}, {"100.127.255.254:1234", true, 200}, {"100.63.255.255:1234", true, 403}, {"100.128.0.1:1234", true, 403}, {"203.0.113.8:1234", true, 403}, {"192.168.1.20:1234", false, 200}} {
		b.config.OverlayEnabled = tc.overlay
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tc.peer
		r.Header.Set("X-Forwarded-For", "127.0.0.1")
		r.Header.Set("X-Forwarded-Proto", "https")
		w := httptest.NewRecorder()
		b.remote(w, r)
		if w.Code != tc.code {
			t.Fatal("peer isolation failed", tc, w.Code)
		}
	}
}

func TestSharingProxyOriginAndForwardedAddress(t *testing.T) {
	cfg := sharingConfig{PublicURL: "https://files.example.com", TrustedProxies: []string{"192.168.1.10", "198.51.100.0/24"}}
	for _, tc := range []struct {
		name, peer, origin, forwarded, client string
		trusted, allowed                      bool
	}{
		{"rewritten backend host", "127.0.0.1:1234", cfg.PublicURL, "203.0.113.9", "203.0.113.9", true, true},
		{"cross site", "127.0.0.1:1234", "https://evil.example", "203.0.113.9", "203.0.113.9", true, false},
		{"untrusted proxy spoof", "192.168.1.20:1234", cfg.PublicURL, "203.0.113.9", "192.168.1.20", false, false},
		{"rightmost client", "127.0.0.1:1234", cfg.PublicURL, "1.1.1.1, 203.0.113.9", "203.0.113.9", true, true},
		{"trusted proxy chain", "127.0.0.1:1234", cfg.PublicURL, "1.1.1.1, 203.0.113.9, 198.51.100.10", "203.0.113.9", true, true},
		{"invalid chain", "127.0.0.1:1234", cfg.PublicURL, "203.0.113.9, invalid", "127.0.0.1", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "http://127.0.0.1:8788/api/boot", nil)
			r.RemoteAddr = tc.peer
			r.Header.Set("X-Hamster-Browser", "1")
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("X-Forwarded-For", tc.forwarded)
			r.Header.Set("X-Forwarded-Proto", "https")
			r.Header.Set("X-Forwarded-Host", "files.example.com")
			r = sharingProxyRequest(r, cfg, tc.trusted)
			if browserPOST(r) != tc.allowed || browserClientIP(r) != tc.client {
				t.Fatal("origin/proxy boundary failed", browserPOST(r), browserClientIP(r))
			}
		})
	}
}

func TestSharingProxyLoginThrottleUsesClientAddress(t *testing.T) {
	b, _ := newFileBrowser(t.TempDir(), "owner")
	salt := bytes.Repeat([]byte{1}, 24)
	b.config.Salt = hex.EncodeToString(salt)
	b.config.Hash = hex.EncodeToString(sharePassword("Synthetic-Proxy-Password!", salt, 600000))
	b.config.PublicURL = "https://files.example.com"
	login := func(peer, client, origin string) int {
		r := httptest.NewRequest("POST", "http://127.0.0.1:8788/api/login", strings.NewReader(`{"Password":"wrong"}`))
		r.RemoteAddr = peer
		r.Header.Set("X-Hamster-Browser", "1")
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Forwarded-For", client)
		w := httptest.NewRecorder()
		b.remote(w, r)
		return w.Code
	}
	for i := 1; i <= 6; i++ {
		if code := login("127.0.0.1:1234", "203.0.113."+strconv.Itoa(i), b.config.PublicURL); code != 401 {
			t.Fatal("different proxy clients shared a throttle bucket", i, code)
		}
	}
	for i := 0; i < 4; i++ {
		login("127.0.0.1:1234", "203.0.113.1", b.config.PublicURL)
	}
	if code := login("127.0.0.1:1234", "1.1.1.1, 203.0.113.1", b.config.PublicURL); code != 429 {
		t.Fatal("spoofed leftmost address bypassed login throttle", code)
	}
	for i := 1; i <= 6; i++ {
		code := login("192.168.1.20:1234", "203.0.113."+strconv.Itoa(i), "http://127.0.0.1:8788")
		if i == 6 && code != 429 {
			t.Fatal("untrusted forwarding headers bypassed login throttle", code)
		}
	}
}

// Exercise a real HTTPS frontend proxy to a plain HTTP backend. This models
// common tunnel products without contacting a third party or opening a LAN port.
func TestSharingHTTPSReverseProxyEndToEnd(t *testing.T) {
	root, data := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(root, "range.mp4"), []byte("0123456789"), 0600)
	b, _ := newFileBrowser(data, "private-owner-token")
	backend := httptest.NewServer(http.HandlerFunc(b.remote))
	defer backend.Close()
	target, _ := url.Parse(backend.URL)
	proxy := &httputil.ReverseProxy{Rewrite: func(p *httputil.ProxyRequest) { p.SetURL(target); p.SetXForwarded() }}
	frontend := httptest.NewTLSServer(proxy)
	defer frontend.Close()
	body, _ := json.Marshal(map[string]any{"Enabled": false, "Port": 8788, "OverlayEnabled": true, "PublicURL": frontend.URL, "Password": "Synthetic-Proxy-Password!", "Roots": []sharedFolder{{Name: "Resources", Path: root, Writable: true}}})
	if e := b.saveSharing(httptest.NewRequest("POST", "/", bytes.NewReader(body))); e != nil {
		t.Fatal(e)
	}
	reloaded, e := newFileBrowser(data, "another-owner")
	if e != nil || reloaded.config.PublicURL != frontend.URL || !reloaded.config.OverlayEnabled {
		t.Fatal("remote settings did not persist", e)
	}
	client := frontend.Client()
	client.Jar, _ = cookiejar.New(nil)
	do := func(method, path, body, origin string) (*http.Response, []byte) {
		t.Helper()
		r, _ := http.NewRequest(method, frontend.URL+path, strings.NewReader(body))
		r.Header.Set("X-Hamster-Browser", "1")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if method == "GET" && strings.HasPrefix(path, "/raw") {
			r.Header.Set("Range", "bytes=2-5")
		}
		response, e := client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		out, _ := io.ReadAll(response.Body)
		return response, out
	}
	response, _ := do("POST", "/api/login", `{"Password":"Synthetic-Proxy-Password!"}`, frontend.URL)
	if response.StatusCode != 200 || len(response.Cookies()) != 1 || !response.Cookies()[0].Secure || !response.Cookies()[0].HttpOnly {
		t.Fatal("HTTPS proxy login/cookie failed", response.StatusCode)
	}
	id := b.config.Roots[0].ID
	response, out := do("POST", "/api/boot", `{}`, frontend.URL)
	if response.StatusCode != 200 || bytes.Contains(out, []byte(root)) || bytes.Contains(out, []byte("private-owner-token")) {
		t.Fatal("remote boot failed or leaked owner state", string(out))
	}
	response, out = do("GET", "/raw?root="+id+"&path=range.mp4", "", "")
	if response.StatusCode != 206 || string(out) != "2345" {
		t.Fatal("proxy lost Range playback/download", response.StatusCode, string(out))
	}
	response, out = do("POST", "/api/upload?root="+id+"&path=&name=upload.txt", "proxy upload", frontend.URL)
	if response.StatusCode != 200 {
		t.Fatal("proxy upload failed", response.StatusCode, string(out))
	}
	response, out = do("POST", "/api/list?root="+id, `{"Path":""}`, frontend.URL)
	var page struct{ Entries []browserEntry }
	json.Unmarshal(out, &page)
	version := ""
	for _, entry := range page.Entries {
		if entry.Name == "upload.txt" {
			version = entry.Version
		}
	}
	mutation, _ := json.Marshal(map[string]string{"Action": "rename", "Path": "upload.txt", "Name": "renamed.txt", "Version": version})
	response, out = do("POST", "/api/mutate?root="+id, string(mutation), frontend.URL)
	if response.StatusCode != 200 {
		t.Fatal("proxy mutation failed", string(out))
	}
	if v, e := os.ReadFile(filepath.Join(root, "renamed.txt")); e != nil || string(v) != "proxy upload" {
		t.Fatal("proxy file operation lost content", e)
	}
	response, _ = do("POST", "/api/mutate?root="+id, `{"Action":"mkdir","Name":"evil"}`, "https://evil.example")
	if response.StatusCode != 403 {
		t.Fatal("cross-site proxy mutation accepted")
	}
	for _, path := range []string{"/api/sharing", "/api/passwords", "/files/api/sharing", "/api/plan"} {
		response, _ = do("POST", path, `{}`, frontend.URL)
		if response.StatusCode == 200 {
			t.Fatal("proxy exposed owner API", path)
		}
	}
	response, _ = do("POST", "/api/logout", `{}`, frontend.URL)
	if response.StatusCode != 200 || len(response.Cookies()) != 1 || !response.Cookies()[0].Secure || response.Cookies()[0].MaxAge != -1 {
		t.Fatal("proxy logout did not clear secure cookie")
	}
	response, _ = do("POST", "/api/boot", `{}`, frontend.URL)
	if response.StatusCode != 401 {
		t.Fatal("proxy session survived logout")
	}
}
