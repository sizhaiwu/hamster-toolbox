//go:build windows

package toolbox

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"unsafe"
)

func TestRecycleRejectsPermanentFallback(t *testing.T) {
	sink := &recycleSink{vtbl: &recycleSinkTable}
	if result := recyclePreDelete(sink, 0, nil); uint32(result) != 0x80004004 || !sink.refused {
		t.Fatal("permanent-delete fallback was allowed")
	}
	if e := recycleLocalPath(`\\server\share\archive.zip`); e == nil {
		t.Fatal("network recycle silently permitted")
	}
}

func TestRecycleExtractFilesIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") == "" {
		t.Skip("set TOOLBOX_INTEGRATION=1")
	}
	root := t.TempDir()
	expected := map[string]string{}
	var sources []string
	for _, name := range []string{"分卷 [test].7z.001", "分卷 [test].7z.002"} {
		path := filepath.Join(root, name)
		content := "recoverable fixture " + name
		writeArchiveFixture(t, path, []byte(content))
		sources = append(sources, path)
		expected[longRecycleFixturePath(path)] = content
	}
	result, e := recycleExtractFiles(sources)
	// Restore only these generated fixtures. Verify the Recycle Bin metadata
	// points at our temp paths before touching any recycled file; never empty or
	// enumerate unrelated user items in the Recycle Bin.
	for _, path := range result.Paths {
		path := path
		t.Cleanup(func() {
			name := filepath.Base(path)
			if !strings.HasPrefix(name, "$R") {
				t.Errorf("unexpected recycled name")
				return
			}
			infoPath := filepath.Join(filepath.Dir(path), "$I"+name[2:])
			data, e := os.ReadFile(infoPath)
			if e != nil || len(data) < 28 {
				t.Errorf("cannot verify fixture recycle metadata: %v", e)
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
			original := strings.TrimRight(syscall.UTF16ToString(units), "\x00")
			body, ok := expected[original]
			if !ok {
				t.Errorf("recycle metadata does not identify our fixture")
				return
			}
			got, e := os.ReadFile(path)
			if e != nil || string(got) != body {
				t.Errorf("recycled content changed: %v", e)
				return
			}
			if _, e = os.Lstat(original); !os.IsNotExist(e) {
				t.Errorf("fixture restore target already exists")
				return
			}
			if e = os.Rename(path, original); e != nil {
				t.Errorf("cannot restore own fixture: %v", e)
				return
			}
			if e = os.Remove(infoPath); e != nil {
				t.Errorf("cannot remove own restored fixture metadata: %v", e)
			}
		})
	}
	if e != nil {
		t.Fatal(e)
	}
	if result.Count != 2 || len(result.Paths) != 2 {
		t.Fatal("recycle operation did not confirm both files", result.Count, len(result.Paths))
	}
	for _, path := range sources {
		if _, e = os.Stat(path); !os.IsNotExist(e) {
			t.Fatal("source not moved to Recycle Bin")
		}
	}
}

func longRecycleFixturePath(path string) string {
	p, e := syscall.UTF16PtrFromString(path)
	if e != nil {
		return path
	}
	buffer := make([]uint16, 32768)
	n, _, _ := recycleKernel.NewProc("GetLongPathNameW").Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	if n == 0 || n >= uintptr(len(buffer)) {
		return path
	}
	return syscall.UTF16ToString(buffer[:n])
}
