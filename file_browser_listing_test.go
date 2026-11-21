package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestDirectoryListingFreshAfterWritesWithUnchangedTimestamp(t *testing.T) {
	for _, action := range []string{"rename", "move", "delete", "mkdir", "upload", "folder-upload"} {
		t.Run(action, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "source.txt")
			if e := os.WriteFile(source, []byte("synthetic file"), 0600); e != nil {
				t.Fatal(e)
			}
			if e := os.Mkdir(filepath.Join(root, "child"), 0700); e != nil {
				t.Fatal(e)
			}
			frozen := time.Unix(1700000000, 0)
			if e := os.Chtimes(root, frozen, frozen); e != nil {
				t.Fatal(e)
			}
			b, e := newFileBrowser(t.TempDir(), "listing-cache-test")
			if e != nil {
				t.Fatal(e)
			}
			read := func() []browserEntry {
				t.Helper()
				w := upgradeJSON(b, "list", map[string]any{"Path": root})
				var page struct{ Entries []browserEntry }
				if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil {
					t.Fatal(w.Code, w.Body.String())
				}
				return page.Entries
			}
			entries := read()
			version := ""
			for _, entry := range entries {
				if entry.Name == "source.txt" {
					version = entry.Version
				}
			}
			if version == "" {
				t.Fatal("source not selectable")
			}
			var result *httptest.ResponseRecorder
			switch action {
			case "rename":
				result = upgradeJSON(b, "mutate", map[string]any{"Action": "rename", "Path": source, "Name": "renamed.txt", "Version": version})
			case "move":
				result = upgradeJSON(b, "mutate", map[string]any{"Action": "move", "Path": source, "Target": filepath.Join(root, "child", "moved.txt"), "Version": version})
			case "delete":
				result = upgradeJSON(b, "mutate", map[string]any{"Action": "delete", "Path": source, "Permanent": true, "Version": version})
			case "mkdir":
				result = upgradeJSON(b, "mutate", map[string]any{"Action": "mkdir", "Path": root, "Name": "new-folder"})
			case "upload":
				r := httptest.NewRequest("POST", "/?path="+url.QueryEscape(root)+"&name=uploaded.txt", bytes.NewBufferString("synthetic upload"))
				result = httptest.NewRecorder()
				b.upload(result, r, true)
			case "folder-upload":
				result = upgradeJSON(b, "upload-init", map[string]any{"Path": root, "RelativePath": "uploaded-folder/empty", "Directory": true})
			}
			if result.Code != 200 {
				t.Fatal(result.Code, result.Body.String())
			}
			// Reproduce filesystems whose directory mtime does not change within a tick.
			if e := os.Chtimes(root, frozen, frozen); e != nil {
				t.Fatal(e)
			}
			actual, e := os.ReadDir(root)
			if e != nil {
				t.Fatal(e)
			}
			want := []string{}
			for _, entry := range actual {
				want = append(want, entry.Name())
			}
			got := []string{}
			for _, entry := range read() {
				got = append(got, entry.Name)
			}
			sort.Strings(want)
			sort.Strings(got)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("stale listing after %s: got %v, want %v", action, got, want)
			}
		})
	}
}
