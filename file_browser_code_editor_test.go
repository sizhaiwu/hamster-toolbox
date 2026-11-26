package main

import (
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestCodeEditorCSPNonceAndStaticAssets(t *testing.T) {
	b, err := newFileBrowser(t.TempDir(), "code-editor-owner")
	if err != nil {
		t.Fatal(err)
	}
	var previous string
	for i := 0; i < 2; i++ {
		r := httptest.NewRequest("GET", "/files/?session=code-editor-owner", nil)
		w := httptest.NewRecorder()
		b.local(w, r)
		match := regexp.MustCompile(`name="file-editor-nonce" content="([a-f0-9]+)"`).FindStringSubmatch(w.Body.String())
		if w.Code != 200 || len(match) != 2 || len(match[1]) < 32 {
			t.Fatal(w.Code, "missing nonce")
		}
		csp := w.Header().Get("Content-Security-Policy")
		if !strings.Contains(csp, "style-src 'self' 'nonce-"+match[1]+"'") || strings.Contains(csp, "unsafe-inline") || !strings.Contains(csp, "script-src 'self';") {
			t.Fatal(csp)
		}
		if previous == match[1] {
			t.Fatal("nonce reused")
		}
		previous = match[1]
	}
	r := httptest.NewRequest("GET", "/file-code-editor.js", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	b.remote(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "hamsterCodeEditor") {
		t.Fatal(w.Code, "offline code editor not available on login page")
	}
	r = httptest.NewRequest("GET", "/explorer-editor-window.js", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	w = httptest.NewRecorder()
	b.remote(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "minimizeEditorWindow") {
		t.Fatal(w.Code, "editor window controls not available")
	}
}
