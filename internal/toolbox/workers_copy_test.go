package toolbox

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestParallelFileLimitAndCancellation(t *testing.T) {
	var active, peak atomic.Int32
	items := make([]Item, 16)
	done := 0
	result := parallelFileItems(context.Background(), items, 3, func(Item) Result {
		n := active.Add(1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		time.Sleep(5 * time.Millisecond)
		active.Add(-1)
		return Result{Success: 1}
	}, func(n, total int) {
		if n <= done || total != 16 {
			t.Errorf("non-monotonic progress: %d", n)
		}
		done = n
	})
	if result.Success != 16 || peak.Load() > 3 || peak.Load() < 2 || done != 16 {
		t.Fatal(result, peak.Load(), done)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result = parallelFileItems(ctx, items, 3, func(Item) Result { t.Error("cancelled item started"); return Result{} }, func(int, int) {})
	if result != (Result{}) {
		t.Fatal(result)
	}
}
func TestBrowserVerifiedCopyAndMove(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	os.MkdirAll(filepath.Join(src, "empty"), 0755)
	data := bytes.Repeat([]byte("verified copy"), 10000)
	os.WriteFile(filepath.Join(src, "clip.dat"), data, 0600)
	dst := filepath.Join(root, "copy")
	if err := BrowserCopy(context.Background(), src, dst, false, func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dst, "clip.dat"))
	if !bytes.Equal(got, data) {
		t.Fatal("content changed")
	}
	if st, err := os.Stat(filepath.Join(dst, "empty")); err != nil || !st.IsDir() {
		t.Fatal("empty directory lost")
	}
	if err := BrowserCopy(context.Background(), src, dst, false, func(int64, int64) {}); err == nil {
		t.Fatal("overwrote destination")
	}
	moved := filepath.Join(root, "moved")
	if err := BrowserCopy(context.Background(), src, moved, true, func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("source not removed after validated move")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := BrowserCopy(ctx, moved, filepath.Join(root, "cancelled"), true, func(int64, int64) {}); err == nil {
		t.Fatal("cancel ignored")
	}
	if _, err := os.Stat(moved); err != nil {
		t.Fatal("cancel removed source")
	}
	if err := BrowserCopy(context.Background(), moved, filepath.Join(moved, "child"), false, func(int64, int64) {}); err == nil {
		t.Fatal("nested target allowed")
	}
}
func TestBrowserCopyRejectsChangedSource(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source.bin")
	os.WriteFile(src, bytes.Repeat([]byte("a"), 2<<20), 0600)
	changed := false
	dst := filepath.Join(root, "target.bin")
	err := BrowserCopy(context.Background(), src, dst, true, func(done, total int64) {
		if done > 0 && !changed {
			changed = true
			os.WriteFile(src, []byte("changed"), 0600)
		}
	})
	if err == nil {
		t.Fatal("changed source was moved")
	}
	if _, err = os.Stat(src); err != nil {
		t.Fatal("changed original removed")
	}
	if _, err = os.Stat(dst); !os.IsNotExist(err) {
		t.Fatal("incomplete target published")
	}
}
