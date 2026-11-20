package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrowserTransferConflictNeedsExplicitChoice(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "cleaned")
	dst := filepath.Join(root, "original")
	os.MkdirAll(src, 0700)
	os.MkdirAll(dst, 0700)
	os.WriteFile(filepath.Join(src, "image.jpg"), []byte("clean"), 0600)
	os.WriteFile(filepath.Join(dst, "image.jpg"), []byte("old"), 0600)
	b, _ := newFileBrowser(t.TempDir(), "owner")
	version := shortcutTestReference(t, b, src)
	call := func(policy string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"Path": src, "Target": dst, "Action": "copy", "Version": version, "Conflict": policy})
		r := httptest.NewRequest("POST", "/api/transfer", bytes.NewReader(body))
		r.Header.Set("X-Hamster-Browser", "1")
		w := httptest.NewRecorder()
		b.dispatch(w, r, true, "api/transfer")
		return w
	}
	w := call("")
	if w.Code != http.StatusConflict {
		t.Fatal(w.Code, w.Body.String())
	}
	if data, _ := os.ReadFile(filepath.Join(dst, "image.jpg")); string(data) != "old" {
		t.Fatal("conflict probe overwrote file")
	}
	w = call("overwrite")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"Finished":true`) || !strings.Contains(w.Body.String(), `.toolbox-replaced-`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if data, _ := os.ReadFile(filepath.Join(dst, "image.jpg")); string(data) != "clean" {
		t.Fatal("explicit replacement did not happen")
	}
}
