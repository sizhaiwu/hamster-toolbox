package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenLocalDirectoryOwnerOnlyAndFolderOnly(t *testing.T) {
	b, _ := newFileBrowser(t.TempDir(), "owner")
	folder := filepath.Join(t.TempDir(), "演示 & 资源, 空格")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	var opened []string
	b.openDirectory = func(path string) error { opened = append(opened, path); return nil }
	w := explorerSettingsRequest(b, "open-local", map[string]string{"Path": folder}, true)
	if w.Code != 200 || len(opened) != 1 {
		t.Fatal("owner could not open a directory", w.Code, w.Body.String(), opened)
	}
	st, err := os.Stat(opened[0])
	if err != nil || !st.IsDir() {
		t.Fatal("launcher received something other than the validated directory", err)
	}
	if w := explorerSettingsRequest(b, "open-local", map[string]string{"Path": folder}, false); w.Code != 404 || len(opened) != 1 {
		t.Fatal("a guest can launch the host file manager", w.Code, opened)
	}
	file := filepath.Join(folder, "sample.txt")
	if err := os.WriteFile(file, []byte("demo"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", ".", "relative", "https://example.com", "file:///tmp", "-select", folder + "\x00", folder + "\n", file, filepath.Join(folder, "missing")} {
		w := explorerSettingsRequest(b, "open-local", map[string]string{"Path": path}, true)
		if w.Code != 400 || len(opened) != 1 {
			t.Fatal("invalid/non-directory path was launched", path, w.Code, opened)
		}
	}
	b.openDirectory = func(string) error { return errors.New("desktop unavailable") }
	if w := explorerSettingsRequest(b, "open-local", map[string]string{"Path": folder}, true); w.Code != 400 {
		t.Fatal("failed native launch reported success", w.Code)
	}
}

func TestOpenLocalDirectoryRejectsDirectoryLinks(t *testing.T) {
	b, _ := newFileBrowser(t.TempDir(), "owner")
	root := t.TempDir()
	link := filepath.Join(root, "alias")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Skip("directory links unavailable:", err)
	}
	b.openDirectory = func(string) error { t.Fatal("directory link reached launcher"); return nil }
	if w := explorerSettingsRequest(b, "open-local", map[string]string{"Path": link}, true); w.Code != 400 {
		t.Fatal("directory link was accepted", w.Code)
	}
}
