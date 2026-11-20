package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResumableUploadGuestPermissionsAndEmptyDirectories(t *testing.T) {
	root := t.TempDir()
	b, _ := newFileBrowser(t.TempDir(), "guest-upload")
	scope, _ := toolbox.NewBrowserScope(root)
	b.scopes["one"] = scope
	b.config.Roots = []sharedFolder{{ID: "one", Writable: false}}
	call := func(body any) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", "/?root=one", bytes.NewReader(data))
		r.Header.Set("X-Hamster-Browser", "1")
		w := httptest.NewRecorder()
		b.dispatch(w, r, false, "api/upload-init")
		return w
	}
	request := map[string]any{"Path": "", "RelativePath": "folder/empty", "Directory": true, "Size": 0, "Sample": ""}
	if w := call(request); w.Code == 200 {
		t.Fatal("read-only upload accepted")
	}
	b.config.Roots[0].Writable = true
	if w := call(request); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if st, err := os.Stat(filepath.Join(root, "folder", "empty")); err != nil || !st.IsDir() {
		t.Fatal("empty directory upload failed")
	}
	request["RelativePath"] = "../escape"
	if w := call(request); w.Code == 200 {
		t.Fatal("shared traversal accepted")
	}
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{}`))
	r.Header.Set("X-Hamster-Browser", "1")
	w := httptest.NewRecorder()
	b.dispatch(w, r, false, "api/cache-clear")
	if w.Code != 404 {
		t.Fatal("guest cleared owner cache")
	}
}

func TestGeneralSettingsPartialUpdateAndValidation(t *testing.T) {
	s := &server{dataDir: t.TempDir(), token: "test-session"}
	w := requestAPI(t, s, "/api/ui-settings", map[string]any{"BrowserAutoPlay": false, "BrowserMuted": true, "RememberPlayback": true, "RestoreTabs": true, "FontScale": 110, "ThumbnailCacheMB": 0, "DefaultPlayer": "mpv"})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = requestAPI(t, s, "/api/ui-settings", map[string]any{"TaskSound": false})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	p, err := (&server{dataDir: s.dataDir}).loadPreferences()
	if err != nil || p.BrowserAutoPlay == nil || *p.BrowserAutoPlay || !(*p.BrowserMuted) || !(*p.RememberPlayback) || !(*p.RestoreTabs) || p.FontScale != 110 || p.ThumbnailCacheMB == nil || *p.ThumbnailCacheMB != 0 || p.DefaultPlayer != "mpv" {
		t.Fatal(p, err)
	}
	for _, bad := range []map[string]any{{"FontScale": 300}, {"ThumbnailCacheMB": -1}, {"DefaultPlayer": "unknown"}} {
		if w = requestAPI(t, s, "/api/ui-settings", bad); w.Code == 200 {
			t.Fatal("invalid setting accepted", bad)
		}
	}
}

func TestSavedExplorerWorkspaceAcrossServerRestart(t *testing.T) {
	dir := t.TempDir()
	b, _ := newFileBrowser(dir, "workspace-first")
	root := t.TempDir()
	workspace := explorerSavedWorkspace{Version: 1, Active: "tab-one", Tabs: []explorerSavedTab{{ID: "tab-one", Path: root, Sort: "name", Order: "asc", View: "grid", Offset: 25000, Scroll: 800}}}
	w := upgradeJSON(b, "preferences", map[string]any{"workspace": workspace})
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	restarted, _ := newFileBrowser(dir, "workspace-restarted")
	p, err := restarted.loadExplorerPreferences()
	if err != nil || p.Workspace == nil || p.Workspace.Tabs[0].Path != root || p.Workspace.Tabs[0].Offset != 25000 {
		t.Fatal("workspace not restored", p, err)
	}
	w = upgradeJSON(b, "preferences", map[string]any{"workspace": map[string]any{"version": 1, "active": "tab-one", "tabs": []map[string]any{{"id": "tab-one", "path": root, "session": "never-persist"}}}})
	if w.Code == 200 {
		t.Fatal("credential fields accepted in workspace")
	}
	workspace.Tabs[0].Root = "share-id"
	if w = upgradeJSON(b, "preferences", map[string]any{"workspace": workspace}); w.Code == 200 {
		t.Fatal("remote scope persisted as owner workspace")
	}
}

func upgradeRequest(b *fileBrowser, route string, body []byte) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/files/api/"+route, bytes.NewReader(body))
	r.Header.Set("X-Hamster-Browser", "1")
	r.AddCookie(&http.Cookie{Name: "hamster_files_" + b.cookieTag, Value: b.localCookie})
	w := httptest.NewRecorder()
	b.local(w, r)
	return w
}
func upgradeJSON(b *fileBrowser, route string, body any) *httptest.ResponseRecorder {
	data, _ := json.Marshal(body)
	return upgradeRequest(b, route, data)
}
func TestResumableFolderUploadAcrossRestart(t *testing.T) {
	dataDir, root := t.TempDir(), t.TempDir()
	b, err := newFileBrowser(dataDir, "test-upload")
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"Path": root, "RelativePath": "album/sub/test.txt", "Size": 6, "Modified": 1, "Sample": "YWJjZGVm"}
	init := upgradeJSON(b, "upload-init", input)
	if init.Code != 200 {
		t.Fatal(init.Code, init.Body.String())
	}
	var session struct {
		ID     string
		Offset int64
	}
	json.Unmarshal(init.Body.Bytes(), &session)
	w := upgradeRequest(b, "upload-chunk?id="+session.ID+"&offset=0", []byte("abc"))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	b, err = newFileBrowser(dataDir, "restarted-upload")
	if err != nil {
		t.Fatal(err)
	}
	again := upgradeJSON(b, "upload-init", input)
	var resumed struct {
		ID     string
		Offset int64
	}
	json.Unmarshal(again.Body.Bytes(), &resumed)
	if again.Code != 200 || resumed.ID != session.ID || resumed.Offset != 3 {
		t.Fatal("resume failed", again.Body.String())
	}
	w = upgradeRequest(b, "upload-chunk?id="+session.ID+"&offset=0", []byte("xyz"))
	if w.Code == 200 {
		t.Fatal("wrong offset accepted")
	}
	w = upgradeJSON(b, "upload-finish?id="+session.ID, map[string]any{})
	if w.Code == 200 {
		t.Fatal("partial upload published")
	}
	w = upgradeRequest(b, "upload-chunk?id="+session.ID+"&offset=3", []byte("def"))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = upgradeJSON(b, "upload-finish?id="+session.ID, map[string]any{})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	got, _ := os.ReadFile(filepath.Join(root, "album", "sub", "test.txt"))
	if string(got) != "abcdef" {
		t.Fatal("bad content", string(got))
	}
	if w = upgradeJSON(b, "upload-init", input); w.Code == 200 {
		t.Fatal("existing target overwritten")
	}
	input["RelativePath"] = "../escape.txt"
	if w = upgradeJSON(b, "upload-init", input); w.Code == 200 {
		t.Fatal("path traversal accepted")
	}
}
func TestFullDirectorySearchBeyondOldLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("large directory fixture")
	}
	root := t.TempDir()
	for i := 0; i < 20003; i++ {
		f, err := os.Create(filepath.Join(root, fmt.Sprintf("%05d.txt", i)))
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	os.Mkdir(filepath.Join(root, "nested"), 0755)
	os.WriteFile(filepath.Join(root, "nested", "needle.txt"), []byte("x"), 0600)
	b, _ := newFileBrowser(t.TempDir(), "listing-test")
	w := upgradeJSON(b, "list", map[string]any{"Path": root, "Search": "20002", "PageSize": 50})
	var result struct {
		Entries   []browserEntry
		Total     int
		Truncated bool
	}
	json.Unmarshal(w.Body.Bytes(), &result)
	if w.Code != 200 || result.Total != 1 || result.Truncated || result.Entries[0].Name != "20002.txt" {
		t.Fatal(w.Code, w.Body.String())
	}
	w = upgradeJSON(b, "list", map[string]any{"Path": root, "Search": "needle", "Recursive": true})
	json.Unmarshal(w.Body.Bytes(), &result)
	if w.Code != 200 || result.Total != 1 {
		t.Fatal(w.Body.String())
	}
	w = upgradeJSON(b, "list", map[string]any{"Path": root, "Sort": "name", "Order": "desc", "PageSize": 50, "Force": true})
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Total != 20004 || result.Entries[1].Name != "20002.txt" {
		t.Fatal("full sorting missing", w.Body.String()[:min(300, w.Body.Len())])
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := httptest.NewRequest("POST", "/", nil).WithContext(ctx)
	if _, err := b.readDirectoryListing(r, root, nil, "never", false, true, true); err == nil {
		t.Fatal("search ignored cancellation")
	}
}
func TestPersistentThumbnailCacheQuotaAndClear(t *testing.T) {
	dir := t.TempDir()
	b, _ := newFileBrowser(dir, "cache-test")
	data := append([]byte{0xff, 0xd8, 0xff}, bytes.Repeat([]byte("x"), 600<<10)...)
	one := 1
	s := &server{dataDir: dir}
	p, _ := s.loadPreferences()
	p.ThumbnailCacheMB = &one
	s.writePreferences(p)
	b.storeDiskThumbnail("one", data)
	b.storeDiskThumbnail("two", data)
	restarted, _ := newFileBrowser(dir, "cache-restart")
	if _, ok := restarted.diskThumbnail("two"); !ok {
		t.Fatal("cache did not survive restart")
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "thumbnails"))
	if len(entries) != 1 {
		t.Fatal("quota exceeded", len(entries))
	}
	if err := restarted.clearDiskThumbnails(); err != nil {
		t.Fatal(err)
	}
	if _, ok := restarted.diskThumbnail("two"); ok {
		t.Fatal("clear failed")
	}
}
func TestTransferStreamingCopy(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source.txt")
	dst := filepath.Join(root, "copy.txt")
	os.WriteFile(src, []byte("copy me"), 0600)
	b, _ := newFileBrowser(t.TempDir(), "transfer-test")
	page := upgradeJSON(b, "list", map[string]any{"Path": root})
	var listing struct{ Entries []browserEntry }
	json.Unmarshal(page.Body.Bytes(), &listing)
	w := upgradeJSON(b, "transfer", map[string]any{"Action": "copy", "Path": src, "Target": dst, "Version": listing.Entries[0].Version})
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"Finished":true`) {
		t.Fatal(w.Code, w.Body.String())
	}
	f, err := os.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(f)
	f.Close()
	if string(got) != "copy me" {
		t.Fatal(string(got))
	}
}
