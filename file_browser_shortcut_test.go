package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

func shortcutTestRequest(b *fileBrowser, path, version string, local bool, root string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"Path": path, "Version": version})
	r := httptest.NewRequest("POST", "/files/api/shortcut?root="+url.QueryEscape(root), bytes.NewReader(body))
	r.Header.Set("X-Hamster-Browser", "1")
	w := httptest.NewRecorder()
	b.dispatch(w, r, local, "api/shortcut")
	return w
}
func shortcutTestReference(t *testing.T, b *fileBrowser, path string) string {
	t.Helper()
	ref, err := toolbox.CaptureBrowserItem(path)
	if err != nil {
		t.Fatal(err)
	}
	id := randomID()
	b.references[id] = browserReference{ref, time.Now()}
	return id
}
func TestDirectoryShortcutInvalidInput(t *testing.T) {
	b, _ := newFileBrowser(t.TempDir(), "owner")
	path := filepath.Join(t.TempDir(), "fake.lnk")
	if err := os.WriteFile(path, []byte("not a shell link"), 0600); err != nil {
		t.Fatal(err)
	}
	version := shortcutTestReference(t, b, path)
	for _, pair := range [][2]string{{path, version}, {path, "expired"}, {path + ".exe", version}, {"https://example.com/folder.lnk", version}} {
		if w := shortcutTestRequest(b, pair[0], pair[1], true, ""); w.Code != 400 {
			t.Fatal("invalid shortcut accepted", w.Code)
		}
	}
	r := httptest.NewRequest("POST", "/files/api/shortcut", bytes.NewBufferString(`{"Path":"test.lnk"}`))
	r.Header.Set("X-Hamster-Browser", "1")
	w := httptest.NewRecorder()
	b.local(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatal("owner authentication missing")
	}
	if browserKind("Folder.LNK") != "shortcut" {
		t.Fatal("case-insensitive shortcut kind missing")
	}
}
