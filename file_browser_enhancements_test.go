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

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

func TestSharedDriveRootStillHidesSystemAndPrivateData(t *testing.T) {
	b, e := newFileBrowser(t.TempDir(), "owner")
	if e != nil {
		t.Fatal(e)
	}
	root := filepath.VolumeName(os.TempDir()) + string(filepath.Separator)
	data, _ := json.Marshal(map[string]any{"Port": 8788, "Password": "Synthetic-Root-Share!", "Roots": []sharedFolder{{Name: "Disk", Path: root}}})
	if e := b.saveSharing(httptest.NewRequest("POST", "/", bytes.NewReader(data))); e != nil {
		t.Fatal(e)
	}
	if len(b.config.Roots) != 1 || b.config.Roots[0].Writable {
		t.Fatal("root sharing did not default to read-only")
	}
	id := b.config.Roots[0].ID
	req := httptest.NewRequest("POST", "/api/list?root="+id, strings.NewReader(`{"Path":""}`))
	req.Header.Set("X-Hamster-Browser", "1")
	w := httptest.NewRecorder()
	b.dispatch(w, req, false, "api/list")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var page struct{ Entries []browserEntry }
	json.Unmarshal(w.Body.Bytes(), &page)
	for _, item := range page.Entries {
		if item.Name == "Windows" || item.Name == "etc" || item.Name == "proc" || item.Name == "$Recycle.Bin" {
			t.Fatal("disk share listed system contents", item.Name)
		}
	}
	if _, e := b.scopes[id].Resolve("", true); e == nil {
		t.Fatal("drive root mutable")
	}
	c := toolbox.DefaultConfig()
	c.Tool, c.Root = "rename", root
	if _, e := toolbox.BuildPlan(req.Context(), c, func(string) {}); e == nil {
		t.Fatal("batch renaming accepted drive root")
	}
}

func TestOwnerFolderPickerFiltersFilesAndKeepsPagination(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "folder"), 0755)
	os.WriteFile(filepath.Join(dir, "image.jpg"), []byte("fixture"), 0600)
	b, _ := newFileBrowser(t.TempDir(), "owner")
	body, _ := json.Marshal(map[string]any{"Path": dir, "DirectoriesOnly": true})
	req := httptest.NewRequest("POST", "/files/api/list", bytes.NewReader(body))
	req.Header.Set("X-Hamster-Browser", "1")
	req.AddCookie(&http.Cookie{Name: "hamster_files_" + b.cookieTag, Value: b.localCookie})
	w := httptest.NewRecorder()
	b.local(w, req)
	var result struct {
		Entries []browserEntry
		Total   int
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Total != 1 || len(result.Entries) != 1 || !result.Entries[0].Dir {
		t.Fatal(w.Code, w.Body.String())
	}
}
