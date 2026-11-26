package main

import (
	"bytes"
	"encoding/json"
	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBrowserEditorAuthorizationAndSave(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "note.txt")
	os.WriteFile(file, []byte("hello"), 0600)
	b, err := newFileBrowser(t.TempDir(), "owner-editor")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := toolbox.NewBrowserScope(root)
	if err != nil {
		t.Fatal(err)
	}
	b.scopes["demo"] = scope
	b.config.Roots = []sharedFolder{{ID: "demo", Path: root, Name: "Demo", Writable: false}}
	call := func(route, path, revision, text string, local, header bool) *httptest.ResponseRecorder {
		data, _ := json.Marshal(map[string]string{"Path": path, "Revision": revision, "Text": text})
		r := httptest.NewRequest("POST", "/api/"+route+"?root=demo", bytes.NewReader(data))
		if header {
			r.Header.Set("X-Hamster-Browser", "1")
		}
		w := httptest.NewRecorder()
		b.dispatch(w, r, local, "api/"+route)
		return w
	}
	if w := call("text-open", "note.txt", "", "", false, true); w.Code == 200 {
		t.Fatal("readonly editor allowed")
	}
	b.config.Roots[0].Writable = true
	w := call("text-open", "note.txt", "", "", false, true)
	var opened toolbox.BrowserText
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &opened) != nil || opened.Text != "hello" {
		t.Fatal(w.Code, w.Body.String())
	}
	w = call("text-save", "note.txt", opened.Revision, "changed 中文", false, true)
	if w.Code != 200 || strings.Contains(w.Body.String(), root) {
		t.Fatal(w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), b.dataDir) || strings.Contains(w.Body.String(), "../") {
		t.Fatal("remote save leaked server history path", w.Body.String())
	}
	if entries, _ := os.ReadDir(root); len(entries) != 1 {
		t.Fatal("editor polluted shared source directory", entries)
	}
	if data, _ := os.ReadFile(file); string(data) != "changed 中文" {
		t.Fatal("save not applied")
	}
	if w = call("text-save", "note.txt", opened.Revision, "stale", false, true); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = call("text-save", "../escape.txt", opened.Revision, "bad", false, true); w.Code == 200 {
		t.Fatal("scope escape allowed")
	}
	if w = call("text-save", file, opened.Revision, "bad", true, false); w.Code != 403 {
		t.Fatal("missing CSRF header allowed", w.Code)
	}
	b.config.Roots[0].Writable = false
	if w = call("text-save", "note.txt", opened.Revision, "bad", false, true); w.Code == 200 {
		t.Fatal("revoked permission ignored")
	}
	r := httptest.NewRequest("POST", "/api/text-open", strings.NewReader(`{}`))
	r.Header.Set("X-Hamster-Browser", "1")
	w = httptest.NewRecorder()
	b.local(w, r)
	if w.Code == 200 {
		t.Fatal("unauthenticated local editor allowed")
	}
}

func TestBrowserEditorReadsDoNotWaitForTransfers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("synthetic read fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	b, err := newFileBrowser(t.TempDir(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]string{"Path": path})
	r := httptest.NewRequest("POST", "/api/text-open", bytes.NewReader(data))
	w := httptest.NewRecorder()
	b.operations.Lock()
	done := make(chan struct{})
	go func() { b.editText(w, r, true, false); close(done) }()
	select {
	case <-done:
		b.operations.Unlock()
	case <-time.After(2 * time.Second):
		b.operations.Unlock()
		<-done
		t.Fatal("opening a file waited for the transfer write lock")
	}
	if w.Code != 200 || !strings.Contains(w.Body.String(), "synthetic read fixture") {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestBrowserEditorSaveResponseDoesNotEchoFullText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	opened, err := toolbox.BrowserReadText(path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := newFileBrowser(t.TempDir(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Repeat("synthetic performance fixture\n", 10000)
	data, _ := json.Marshal(map[string]string{"Path": path, "Text": text, "Revision": opened.Revision})
	w := httptest.NewRecorder()
	b.editText(w, httptest.NewRequest("POST", "/api/text-save", bytes.NewReader(data)), true, true)
	var result map[string]any
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || w.Body.Len() > 4096 || result["Text"] != nil || result["Revision"] == "" || result["Backup"] == "" {
		t.Fatal("save did not return compact revision/backup metadata", w.Code, w.Body.Len())
	}
	if value, err := os.ReadFile(path); err != nil || string(value) != text {
		t.Fatal("saved data differs", err)
	}
	backup, _ := result["Backup"].(string)
	if rel, err := filepath.Rel(filepath.Join(b.dataDir, "editor-backups"), backup); err != nil || strings.HasPrefix(rel, "..") {
		t.Fatal("editor backup outside data directory", backup, err)
	}
	if before, err := os.ReadFile(backup); err != nil || string(before) != "before" {
		t.Fatal("editor backup lost original", err)
	}
}
