package toolbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrowserTextHistoryIsCentralAndBounded(t *testing.T) {
	root, history := t.TempDir(), t.TempDir()
	p := filepath.Join(root, "配置.json")
	os.WriteFile(p, []byte("v0"), 0600)
	legacy := filepath.Join(root, ".toolbox-replaced-existing", "keep.txt")
	os.MkdirAll(filepath.Dir(legacy), 0700)
	os.WriteFile(legacy, []byte("old recovery"), 0600)
	var latest string
	for i := 1; i <= 25; i++ {
		opened, err := BrowserReadText(p)
		if err != nil {
			t.Fatal(err)
		}
		_, latest, err = BrowserSaveText(context.Background(), p, opened.Revision, fmt.Sprintf("v%d", i), history)
		if err != nil {
			t.Fatal(err)
		}
		if data, err := os.ReadFile(latest); err != nil || string(data) != fmt.Sprintf("v%d", i-1) {
			t.Fatal("history does not contain original bytes", err)
		}
		if rel, _ := filepath.Rel(history, latest); strings.HasPrefix(rel, "..") {
			t.Fatal("history escaped data directory", latest)
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(latest))
	if len(entries) != browserTextHistoryLimit+1 {
		t.Fatalf("expected 20 versions and source metadata, got %d", len(entries))
	}
	opened, _ := BrowserReadText(p)
	if _, backup, err := BrowserSaveText(context.Background(), p, opened.Revision, opened.Text, history); err != nil || backup != "" {
		t.Fatal("unchanged save created backup", backup, err)
	}
	entries, _ = os.ReadDir(root)
	if len(entries) != 2 {
		t.Fatal("save left temporary files or directories", entries)
	}
	if data, _ := os.ReadFile(legacy); string(data) != "old recovery" {
		t.Fatal("legacy recovery changed")
	}
	other := filepath.Join(t.TempDir(), filepath.Base(p))
	os.WriteFile(other, []byte("different file"), 0600)
	read, _ := BrowserReadText(other)
	_, second, err := BrowserSaveText(context.Background(), other, read.Revision, "updated", history)
	if err != nil || filepath.Dir(second) == filepath.Dir(latest) {
		t.Fatal("same-name histories collided", second, err)
	}
}

type cancelTextSaveContext struct {
	context.Context
	calls int
}

func (c *cancelTextSaveContext) Err() error {
	c.calls++
	if c.calls >= 2 {
		return context.Canceled
	}
	return nil
}

func TestBrowserTextHistoryFailureKeepsSource(t *testing.T) {
	root, history := t.TempDir(), t.TempDir()
	p := filepath.Join(root, "note.txt")
	os.WriteFile(p, []byte("original"), 0600)
	read, _ := BrowserReadText(p)
	blocked := filepath.Join(history, "not-a-directory")
	os.WriteFile(blocked, []byte("keep"), 0600)
	if _, _, err := BrowserSaveText(context.Background(), p, read.Revision, "new", blocked); err == nil {
		t.Fatal("backup failure ignored")
	}
	ctx := &cancelTextSaveContext{Context: context.Background()}
	if _, _, err := BrowserSaveText(ctx, p, read.Revision, "new", history); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation after moving original was not restored", err)
	}
	if data, _ := os.ReadFile(p); string(data) != "original" {
		t.Fatal("original lost")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 {
		t.Fatal("rollback/staging left behind", entries)
	}
	filepath.WalkDir(history, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(d.Name(), "version-") {
			t.Error("failed save left history version", path)
		}
		return nil
	})
}
