package toolbox

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrowserCanReadDriveRootWithoutRelaxingResourceGuards(t *testing.T) {
	root := filepath.VolumeName(os.TempDir()) + string(filepath.Separator)
	f, e := BrowseOpen(root)
	if e != nil {
		t.Fatalf("file browser cannot enter drive root: %v", e)
	}
	defer f.Close()
	if st, e := f.Stat(); e != nil || !st.IsDir() {
		t.Fatal("drive root is not an open directory", e)
	}
	if _, e := f.ReadDir(1); e != nil && e != io.EOF {
		t.Fatal("cannot list drive root", e)
	}
	if _, e := safePath(root); e == nil {
		t.Fatal("write/delete guard accepted drive root")
	}
	if _, e := resolveNonLinkPath(root); e == nil {
		t.Fatal("ordinary resource input accepted drive root")
	}
	scope, e := NewBrowserScope(root)
	if e != nil {
		t.Fatal("explicit drive sharing failed", e)
	}
	if _, e := scope.Resolve("", false); e != nil {
		t.Fatal("shared drive cannot be listed", e)
	}
	if _, e := scope.Resolve("", true); e == nil {
		t.Fatal("shared drive itself became mutable")
	}
	if _, e := scope.Resolve(strings.TrimPrefix(systemDirectory(), root), false); e == nil {
		t.Fatal("root share exposed a system directory")
	}
}

func TestBrowserShareHidesPrivateData(t *testing.T) {
	root := t.TempDir()
	private := filepath.Join(root, "private")
	os.Mkdir(private, 0700)
	file(t, filepath.Join(private, "settings.txt"), []byte("secret"))
	scope, e := NewBrowserScope(root)
	if e != nil {
		t.Fatal(e)
	}
	ProtectDataDirectory(private)
	for _, path := range []string{"private", "private/settings.txt"} {
		if _, e := scope.Resolve(path, false); e == nil {
			t.Fatal("shared protected private data", path)
		}
	}
}

func TestBrowserFilesystemBoundsAndNoOverwrite(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	src := filepath.Join(root, "a.txt")
	os.WriteFile(src, []byte("original"), 0600)
	scope, e := NewBrowserScope(root)
	if e != nil {
		t.Fatal(e)
	}
	for _, rel := range []string{"../x", `..\x`, `C:\Windows\win.ini`, "a.txt:stream"} {
		if _, e := scope.Resolve(rel, false); e == nil {
			t.Fatal("path escaped scope", rel)
		}
	}
	if _, e := scope.Resolve("", true); e == nil {
		t.Fatal("shared root mutable")
	}
	if e := BrowserUpload(context.Background(), src, strings.NewReader("replacement")); e == nil {
		t.Fatal("upload overwrote existing source")
	}
	other := filepath.Join(root, "b.txt")
	os.WriteFile(other, []byte("other"), 0600)
	if e := BrowserMove(src, other); e == nil {
		t.Fatal("move replaced existing target")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := BrowserUpload(ctx, filepath.Join(root, "cancel.txt"), strings.NewReader("test")); e == nil {
		t.Fatal("canceled upload published")
	}
	if _, e := os.Stat(filepath.Join(root, "cancel.txt")); !os.IsNotExist(e) {
		t.Fatal("canceled upload exists")
	}
	link := filepath.Join(root, "link")
	if os.Symlink(outside, link) == nil {
		if f, e := BrowseOpen(link); e == nil {
			f.Close()
			t.Fatal("read-only browser followed a directory symlink")
		}
		if _, e := scope.Resolve("link", false); e == nil {
			t.Fatal("followed directory symlink")
		}
		if e := BrowserDelete(context.Background(), root, true); e == nil {
			t.Fatal("recursively deleted tree containing a link")
		}
	}
	if data, _ := os.ReadFile(src); string(data) != "original" {
		t.Fatal("source changed")
	}
}
func TestBrowserScopeRejectsReplacedRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	os.Mkdir(root, 0755)
	scope, e := NewBrowserScope(root)
	if e != nil {
		t.Fatal(e)
	}
	if e := os.Rename(root, filepath.Join(parent, "original")); e != nil {
		t.Fatal(e)
	}
	os.Mkdir(root, 0755)
	if _, e := scope.Resolve("", false); e == nil {
		t.Fatal("replaced shared root accepted")
	}
}

func TestBrowserSelectionRejectsReplacedFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "selected.txt")
	os.WriteFile(path, []byte("before"), 0600)
	st, _ := os.Stat(path)
	ref, e := CaptureBrowserItem(path)
	if e != nil {
		t.Fatal(e)
	}
	os.Rename(path, filepath.Join(root, "saved.txt"))
	os.WriteFile(path, []byte("after!"), 0600)
	os.Chtimes(path, st.ModTime(), st.ModTime())
	if e := ref.Check(path); e == nil {
		t.Fatal("same name, size and time must not bypass file identity validation")
	}
}
