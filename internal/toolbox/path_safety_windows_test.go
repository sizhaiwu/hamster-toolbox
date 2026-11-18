//go:build windows

package toolbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func makeSafetyJunction(t *testing.T, link, target string) {
	t.Helper()
	cmd := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", link, target)
	hideWindow(cmd)
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("create owned test junction: %v %s", e, out)
	}
	// Remove only the junction itself, never recurse into its destination.
	t.Cleanup(func() { os.Remove(link) })
}

func TestSafetyWindowsJunctionsAndPreviewSwap(t *testing.T) {
	root, victim := t.TempDir(), t.TempDir()
	file(t, filepath.Join(victim, "photo.jpg"), []byte("untouched"))
	link := filepath.Join(root, "linked")
	makeSafetyJunction(t, link, victim)
	if _, e := safePath(filepath.Join(link, "future", "out.jpg")); e == nil {
		t.Fatal("ancestor junction accepted")
	}
	c := DefaultConfig()
	c.Tool, c.Root = "format", root
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil || len(p.Items) != 0 {
		t.Fatalf("junction was scanned: %v %+v", e, p)
	}
	c.Root = link
	if _, e := BuildPlan(context.Background(), c, quiet); e == nil {
		t.Fatal("selected junction accepted")
	}
	c.Root = root
	file(t, filepath.Join(root, "normal", "a.jpg"), []byte("image"))
	p, e = BuildPlan(context.Background(), c, quiet)
	if e != nil {
		t.Fatal(e)
	}
	normal := filepath.Join(root, "normal")
	if e = os.Rename(normal, filepath.Join(root, "saved")); e != nil {
		t.Fatal(e)
	}
	makeSafetyJunction(t, normal, victim)
	if r := Execute(context.Background(), p, quiet, progress); r.Success != 0 || r.Failed == 0 {
		t.Fatal(r)
	}
	if data, e := os.ReadFile(filepath.Join(victim, "photo.jpg")); e != nil || string(data) != "untouched" {
		t.Fatalf("victim changed: %v", e)
	}
}

func TestSafetyWindowsAliasesAndDevicePaths(t *testing.T) {
	for _, path := range []string{`C:`, `C:relative`, `\\.\C:\Windows`, `\\?\C:\Windows`, `//./C:/Windows`, `C:\data:stream`, `\\server\share`, `\\server\C$\data`} {
		if _, e := safePath(path); e == nil {
			t.Fatalf("unsafe path accepted: %s", path)
		}
	}
	path := filepath.Join(filepath.VolumeName(systemDirectory())+`\`, "Program Files")
	src, _ := syscall.UTF16PtrFromString(path)
	b := make([]uint16, 32768)
	n, e := syscall.GetShortPathName(src, &b[0], uint32(len(b)))
	if e != nil || n == 0 {
		t.Skip("short names unavailable")
	}
	if _, e := safePath(syscall.UTF16ToString(b[:n])); e == nil {
		t.Fatal("short-name system directory accepted")
	}
}
