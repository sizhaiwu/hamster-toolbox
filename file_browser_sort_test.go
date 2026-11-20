package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

func TestBrowserNaturalOrderingAndDirection(t *testing.T) {
	names := []string{"1024 - album", "2 - album", "1022 - album", "10 - album", "1 - album", "3 - album"}
	items := []browserEntry{{Name: "0.jpg", Size: 1}, {Name: "11.jpg", Size: 11}, {Name: "2.jpg", Size: 2}}
	for _, name := range names {
		items = append(items, browserEntry{Name: name, Dir: true})
	}
	sortBrowserEntries(items, "name", "asc")
	got := []string{}
	for _, item := range items {
		got = append(got, item.Name)
	}
	want := []string{"1 - album", "2 - album", "3 - album", "10 - album", "1022 - album", "1024 - album", "0.jpg", "2.jpg", "11.jpg"}
	if !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
	sortBrowserEntries(items, "name", "desc")
	if !items[0].Dir || items[0].Name != "1024 - album" || items[6].Dir || items[6].Name != "11.jpg" {
		t.Fatal(items)
	}
	for _, key := range []string{"size", "modified"} {
		entries := []browserEntry{{Name: "folder", Dir: true}, {Name: "1", Size: 1, Modified: time.Unix(1, 0)}, {Name: "2", Size: 2, Modified: time.Unix(2, 0)}}
		sortBrowserEntries(entries, key, "asc")
		if entries[1].Name != "1" || !entries[0].Dir {
			t.Fatal(key, entries)
		}
		sortBrowserEntries(entries, key, "desc")
		if entries[1].Name != "2" || !entries[0].Dir {
			t.Fatal(key, entries)
		}
	}
	// Numeric runs are compared without overflowing a machine integer.
	if !toolbox.BrowserNameLess("file9.jpg", "file10000000000000000000000000000.jpg") {
		t.Fatal("long numeric names")
	}
}

func TestBrowserPaginationAndSavedControls(t *testing.T) {
	dir := t.TempDir()
	for i := 1; i <= 113; i++ {
		if err := os.Mkdir(filepath.Join(dir, fmt.Sprintf("%d - sample", i)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := newFileBrowser(t.TempDir(), "owner")
	for _, tc := range []struct {
		size, offset, count, wantOffset int
		order, first                    string
	}{{50, 0, 50, 0, "asc", "1 - sample"}, {50, 50, 50, 50, "asc", "51 - sample"}, {50, 999, 13, 100, "asc", "101 - sample"}, {100, 0, 100, 0, "desc", "113 - sample"}, {200, 0, 113, 0, "asc", "1 - sample"}} {
		w := explorerSettingsRequest(b, "list", map[string]any{"Path": dir, "Sort": "name", "Order": tc.order, "PageSize": tc.size, "Offset": tc.offset}, true)
		var page struct {
			Entries                 []browserEntry
			Offset, PageSize, Total int
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil {
			t.Fatal(w.Code, w.Body.String())
		}
		if len(page.Entries) != tc.count || page.Offset != tc.wantOffset || page.PageSize != tc.size || page.Total != 113 || page.Entries[0].Name != tc.first {
			t.Fatal(tc, page)
		}
		if page.Entries[0].Version == "" {
			t.Fatal("pagination dropped safe mutation references")
		}
	}
	for _, body := range []map[string]any{{"PageSize": -1}, {"PageSize": 20000}, {"Order": "invalid"}, {"Sort": "invalid"}} {
		body["Path"] = dir
		if w := explorerSettingsRequest(b, "list", body, true); w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if w := explorerSettingsRequest(b, "preferences", map[string]any{"pageSize": 500, "sort": "size", "order": "asc", "sidebarCollapsed": true}, true); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	restarted, _ := newFileBrowser(b.dataDir, "restarted")
	p, err := restarted.loadExplorerPreferences()
	if err != nil || p.PageSize != 500 || p.Sort != "size" || p.Order != "asc" || !p.SidebarCollapsed {
		t.Fatal(p, err)
	}
	// Legacy files gain defaults without losing their stored view or favorites.
	old := `{"version":1,"view":"list","tileSize":240,"favorites":[]}`
	if err := os.WriteFile(filepath.Join(b.dataDir, "explorer.json"), []byte(old), 0600); err != nil {
		t.Fatal(err)
	}
	p, err = b.loadExplorerPreferences()
	if err != nil || p.PageSize != 200 || p.Order != "asc" || p.View != "list" {
		t.Fatal(p, err)
	}
}

func TestToolboxSidebarSettingPreservesWorkspaces(t *testing.T) {
	s := &server{dataDir: t.TempDir(), token: "owner"}
	c := toolbox.DefaultConfig()
	c.Tool = "image"
	if _, err := s.saveWorkspace(c); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/api/ui-settings", strings.NewReader(`{"SidebarCollapsed":true}`))
	r.Header.Set("X-Toolbox-Token", s.token)
	w := httptest.NewRecorder()
	s.guard(s.api)(w, r)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	p, err := s.loadPreferences()
	if err != nil || !p.SidebarCollapsed || p.Workspaces["image"].Tool != "image" {
		t.Fatal(p, err)
	}
	c.Tool = "rename"
	p, err = s.saveWorkspace(c)
	if err != nil || !p.SidebarCollapsed {
		t.Fatal(p, err)
	}
	r = httptest.NewRequest("POST", "/api/ui-settings", strings.NewReader(`{"SidebarCollapsed":false}`))
	w = httptest.NewRecorder()
	s.guard(s.api)(w, r)
	if w.Code != 403 {
		t.Fatal("UI settings lost owner authentication")
	}
}
