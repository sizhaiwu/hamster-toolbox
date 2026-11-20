//go:build windows

package toolbox

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestBrowserDirectoryRecycleIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") == "" {
		t.Skip("set TOOLBOX_INTEGRATION=1")
	}
	parent := t.TempDir()
	original := filepath.Join(parent, "可还原的演示文件夹")
	os.Mkdir(original, 0755)
	os.WriteFile(filepath.Join(original, "sample.txt"), []byte("own disposable fixture"), 0600)
	expectedOriginal := longRecycleFixturePath(original)
	result, e := recycleWindowsPaths([]string{original}, true)
	for _, path := range result.Paths {
		path := path
		t.Cleanup(func() {
			name := filepath.Base(path)
			if !strings.HasPrefix(name, "$R") {
				t.Error("unexpected recycled name")
				return
			}
			metadata := filepath.Join(filepath.Dir(path), "$I"+name[2:])
			data, e := os.ReadFile(metadata)
			if e != nil || len(data) < 28 {
				t.Error("cannot verify own fixture metadata", e)
				return
			}
			offset := 24
			if binary.LittleEndian.Uint64(data[:8]) == 2 {
				offset = 28
			}
			var units []uint16
			for i := offset; i+1 < len(data); i += 2 {
				units = append(units, binary.LittleEndian.Uint16(data[i:i+2]))
			}
			verified := strings.TrimRight(syscall.UTF16ToString(units), "\x00")
			if !strings.EqualFold(verified, expectedOriginal) {
				t.Error("refusing to touch unrelated recycle item")
				return
			}
			content, e := os.ReadFile(filepath.Join(path, "sample.txt"))
			if e != nil || string(content) != "own disposable fixture" {
				t.Error("recycled folder content changed", e)
				return
			}
			if _, e := os.Lstat(original); !os.IsNotExist(e) {
				t.Error("restore destination exists")
				return
			}
			if e := os.Rename(path, original); e != nil {
				t.Error("restore own folder", e)
				return
			}
			if e := os.Remove(metadata); e != nil {
				t.Error("own recycle metadata", e)
			}
		})
	}
	if e != nil {
		t.Fatal(e)
	}
	if result.Count != 1 || len(result.Paths) != 1 {
		t.Fatal("directory recycle was not confirmed", result)
	}
	if _, e := os.Stat(original); !os.IsNotExist(e) {
		t.Fatal("source folder still exists", e)
	}
}
