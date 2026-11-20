package toolbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBrowserDirectorySizeScopeAndCancellation(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{"one.txt": "123", "nested/two.txt": "12345"} {
		p := filepath.Join(root, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0700)
		if err := os.WriteFile(p, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := BrowserDirectorySize(context.Background(), root, nil)
	if err != nil || result.Size != 8 || result.Files != 2 || result.Directories != 1 || !result.Complete {
		t.Fatal(result, err)
	}
	scope, err := NewBrowserScope(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = BrowserDirectorySize(context.Background(), t.TempDir(), scope); err == nil {
		t.Fatal("escaped original shared scope")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = BrowserDirectorySize(ctx, root, scope); err != context.Canceled {
		t.Fatal("cancellation ignored", err)
	}
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("not counted"), 0600)
	if os.Symlink(outside, filepath.Join(root, "linked")) == nil {
		result, err = BrowserDirectorySize(context.Background(), root, scope)
		if err != nil || result.Size != 8 || result.Skipped != 1 || result.Complete {
			t.Fatal("followed link", result, err)
		}
	}
}
