package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRemotePlaybackAndExpiredAdministrator(t *testing.T) {
	_, b, _ := workspaceFixture(t)
	c := workspaceSignIn(t, b)
	for _, route := range []string{"/files/api/native-player", "/files/api/open-local", "/files/api/open-with", "/api/dedupe-open"} {
		w := c.post(route, map[string]any{"Action": "open", "Path": "synthetic.mp4"})
		if w.Code != 403 || !strings.Contains(w.Body.String(), "浏览器预览") {
			t.Fatal("remote host launch was not rejected", route, w.Code, w.Body.String())
		}
	}
	if b.native != nil {
		t.Fatal("remote request initialized host player")
	}
	w := workspaceCall(b, c, "POST", "/files/api/boot", map[string]any{}, map[string]string{"X-Toolbox-Token": c.csrf, "X-Hamster-Browser": "1"})
	var boot struct{ Local, Remote, NativePrograms bool }
	if json.Unmarshal(w.Body.Bytes(), &boot) != nil || !boot.Local || !boot.Remote || boot.NativePrograms {
		t.Fatal("remote browser capabilities", w.Code, w.Body.String())
	}
	b.workspace.mu.Lock()
	b.workspace.sessions[c.cookie.Value].last = time.Now().Add(-31 * time.Minute)
	b.workspace.mu.Unlock()
	for _, route := range []string{"/admin/status", "/files/api/boot", "/api/settings"} {
		w := c.post(route, map[string]any{})
		if w.Code != 401 || w.Header().Get("X-Hamster-Session") != "expired-admin" || !strings.Contains(w.Body.String(), "管理员会话已过期") {
			t.Fatal("admin expiry fell back to guest password", route, w.Code, w.Body.String())
		}
	}
	w = workspaceCall(b, c, "GET", "/files/?session="+c.csrf, nil, nil)
	if w.Code != 303 || w.Header().Get("Location") != "/admin/?reason=expired" {
		t.Fatal("expired page did not return to admin login", w.Code, w.Header())
	}
	// Expiry on a remote listener must not affect host-only player access.
	if w := nativeRequest(b, `{"Action":"status"}`, true); w.Code != 200 {
		t.Fatal("host player no longer available", w.Code, w.Body.String())
	}
}

func TestGuestExpiredSessionMessage(t *testing.T) {
	b := guestLoginFixture(t)
	r := httptest.NewRequest("POST", "http://local/api/boot", strings.NewReader(`{}`))
	r.RemoteAddr = "192.168.10.20:3000"
	r.Header.Set("X-Hamster-Browser", "1")
	r.AddCookie(&http.Cookie{Name: "hamster_guest_" + b.cookieTag, Value: "expired-test-only"})
	w := httptest.NewRecorder()
	b.remote(w, r)
	if w.Code != 401 || w.Header().Get("X-Hamster-Session") != "expired-guest" || !strings.Contains(w.Body.String(), "共享会话已过期") {
		t.Fatal(w.Code, w.Body.String())
	}
}
