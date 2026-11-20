package toolbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBrowserMergeExplicitReplacementAndBackup(t *testing.T) {
	for _, move := range []bool{false, true} {
		root := t.TempDir()
		src := filepath.Join(root, "cleaned")
		dst := filepath.Join(root, "original")
		file(t, filepath.Join(src, "sub", "image.jpg"), []byte("clean metadata"))
		file(t, filepath.Join(src, "new.jpg"), []byte("new image"))
		file(t, filepath.Join(dst, "sub", "image.jpg"), []byte("original metadata"))
		file(t, filepath.Join(dst, "keep.txt"), []byte("unrelated target"))
		result, err := BrowserMerge(context.Background(), src, dst, move, "overwrite", func(int64, int64) {})
		if err != nil || result.Copied != 2 || result.Backup == "" {
			t.Fatal(result, err)
		}
		for path, want := range map[string]string{filepath.Join(dst, "sub", "image.jpg"): "clean metadata", filepath.Join(dst, "keep.txt"): "unrelated target", filepath.Join(result.Backup, "sub", "image.jpg"): "original metadata"} {
			data, e := os.ReadFile(path)
			if e != nil || string(data) != want {
				t.Fatal(path, string(data), e)
			}
		}
		_, err = os.Stat(src)
		if move && !os.IsNotExist(err) {
			t.Fatal("moved source retained", err)
		}
		if !move && err != nil {
			t.Fatal("copy removed source", err)
		}
	}
}
func TestBrowserMergeSkipRenameAndCancellation(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "target")
	file(t, filepath.Join(src, "same.txt"), []byte("new"))
	file(t, filepath.Join(src, "added.txt"), []byte("added"))
	file(t, filepath.Join(dst, "same.txt"), []byte("old"))
	result, err := BrowserMerge(context.Background(), src, dst, true, "skip", func(int64, int64) {})
	if err != nil || result.Skipped != 1 || result.Copied != 1 {
		t.Fatal(result, err)
	}
	if data, _ := os.ReadFile(filepath.Join(src, "same.txt")); string(data) != "new" {
		t.Fatal("skipped source deleted")
	}
	if data, _ := os.ReadFile(filepath.Join(dst, "same.txt")); string(data) != "old" {
		t.Fatal("skipped destination overwritten")
	}
	if _, err = BrowserMerge(context.Background(), src, dst, false, "rename", func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(root, "target (2)", "same.txt")); string(data) != "new" {
		t.Fatal("renamed copy missing")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = BrowserMerge(ctx, src, dst, true, "overwrite", func(int64, int64) {}); err == nil {
		t.Fatal("cancelled overwrite succeeded")
	}
	if data, _ := os.ReadFile(filepath.Join(dst, "same.txt")); string(data) != "old" {
		t.Fatal("cancelled overwrite changed original")
	}
}
func TestBrowserMergeCleanedSubfolderIntoParent(t *testing.T) {
	dst := t.TempDir()
	src := filepath.Join(dst, "toolbox-output", "exif")
	file(t, filepath.Join(src, "image.jpg"), []byte("clean"))
	file(t, filepath.Join(dst, "image.jpg"), []byte("old"))
	result, err := BrowserMerge(context.Background(), src, dst, false, "overwrite", func(int64, int64) {})
	if err != nil || result.Copied != 1 {
		t.Fatal(result, err)
	}
	if data, _ := os.ReadFile(filepath.Join(dst, "image.jpg")); string(data) != "clean" {
		t.Fatal("cleaned file not placed into original")
	}
	if data, _ := os.ReadFile(filepath.Join(src, "image.jpg")); string(data) != "clean" {
		t.Fatal("nested source damaged")
	}
}
