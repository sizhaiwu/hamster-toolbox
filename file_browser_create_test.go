package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

func TestBrowserCreateFilePermissionsAndNoOverwrite(t *testing.T) {
	root := t.TempDir()
	b, err := newFileBrowser(t.TempDir(), "create-owner")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := toolbox.NewBrowserScope(root)
	if err != nil {
		t.Fatal(err)
	}
	b.scopes["demo"] = scope
	b.config.Roots = []sharedFolder{{ID: "demo", Name: "Demo", Path: root, Writable: true}}
	call := func(parent, name string, local, header bool) *httptest.ResponseRecorder {
		data, _ := json.Marshal(map[string]string{"Action": "create", "Path": parent, "Name": name})
		r := httptest.NewRequest("POST", "/api/mutate?root=demo", bytes.NewReader(data))
		if header {
			r.Header.Set("X-Hamster-Browser", "1")
		}
		w := httptest.NewRecorder()
		b.dispatch(w, r, local, "api/mutate")
		return w
	}
	for _, tc := range []struct {
		parent, name string
		local        bool
	}{{root, "local.txt", true}, {"", "shared.txt", false}, {"", "README", false}} {
		w := call(tc.parent, tc.name, tc.local, true)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var result struct {
			OK         bool
			Path, Kind string
		}
		if json.Unmarshal(w.Body.Bytes(), &result) != nil || !result.OK || result.Path == "" {
			t.Fatal(w.Body.String())
		}
		stat, err := os.Stat(filepath.Join(root, tc.name))
		if err != nil || stat.Size() != 0 || !stat.Mode().IsRegular() {
			t.Fatal(stat, err)
		}
	}
	target := filepath.Join(root, "shared.txt")
	os.WriteFile(target, []byte("keep original"), 0600)
	if w := call("", "shared.txt", false, true); w.Code == 200 {
		t.Fatal("overwrote existing file")
	}
	if data, _ := os.ReadFile(target); string(data) != "keep original" {
		t.Fatal("existing data changed")
	}
	os.Mkdir(filepath.Join(root, "folder.txt"), 0700)
	if w := call("", "folder.txt", false, true); w.Code == 200 {
		t.Fatal("overwrote directory")
	}
	for _, name := range []string{"../escape.txt", "sub/file.txt", "", ".."} {
		if w := call("", name, false, true); w.Code == 200 {
			t.Fatal("invalid name accepted", name)
		}
	}
	if w := call("..", "escape.txt", false, true); w.Code == 200 {
		t.Fatal("escaped shared root")
	}
	b.config.Roots[0].Writable = false
	if w := call("", "readonly.txt", false, true); w.Code == 200 {
		t.Fatal("readonly create accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "readonly.txt")); !os.IsNotExist(err) {
		t.Fatal("readonly file created")
	}
	if w := call(root, "csrf.txt", true, false); w.Code != 403 {
		t.Fatal("CSRF guard bypass", w.Code)
	}
	protected := filepath.Join(t.TempDir(), "private-data")
	os.Mkdir(protected, 0700)
	toolbox.ProtectDataDirectory(protected)
	if w := call(protected, "bad.txt", true, true); w.Code == 200 {
		t.Fatal("protected directory modified")
	}
	if _, err := os.Stat(filepath.Join(protected, "bad.txt")); !os.IsNotExist(err) {
		t.Fatal("protected file created")
	}
}
