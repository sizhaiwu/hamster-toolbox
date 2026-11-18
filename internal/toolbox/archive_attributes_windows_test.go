//go:build windows

package toolbox

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestArchiveUserProvidedMarkerAttributesIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") == "" {
		t.Skip("set TOOLBOX_INTEGRATION=1")
	}
	c := archiveFixture(t)
	c.SevenZip, _ = filepath.Abs(filepath.Join("..", "..", "tools", "sevenzip", "7za.exe"))
	c.ArchiveMode = "extras"
	c.ArchiveMarker = false
	writeArchiveFixture(t, filepath.Join(c.Root, "资源.txt"), []byte("resource"))
	extra := filepath.Join(t.TempDir(), ".packing_date")
	writeArchiveFixture(t, extra, []byte("user supplied marker"))
	c.ArchiveExtras = extra
	ptr, e := syscall.UTF16PtrFromString(extra)
	if e != nil {
		t.Fatal(e)
	}
	before, e := syscall.GetFileAttributes(ptr)
	if e != nil {
		t.Fatal(e)
	}
	p, e := BuildPlan(context.Background(), c, func(string) {})
	if e != nil {
		t.Fatal(e)
	}
	r := Execute(context.Background(), p, func(string) {}, func(int, int) {})
	if r.Success != 1 {
		t.Fatal(r)
	}
	after, e := syscall.GetFileAttributes(ptr)
	if e != nil || before != after {
		t.Fatal("cleanup modified original extra-file attributes", before, after, e)
	}
}
