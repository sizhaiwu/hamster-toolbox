//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
	"golang.org/x/sys/windows"
)

func createShortcutFixture(t *testing.T, path, target string) {
	t.Helper()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	check := func(hr uintptr) {
		t.Helper()
		if err := shortcutHRESULT(hr); err != nil {
			t.Fatal(err)
		}
	}
	hr, _, _ := shortcutInitialize.Call(0, 2)
	check(hr)
	defer shortcutUninitialize.Call()
	var link, persist *shortcutCOM
	hr, _, _ = shortcutCreate.Call(uintptr(unsafe.Pointer(&shortcutClass)), 0, 1, uintptr(unsafe.Pointer(&shortcutInterface)), uintptr(unsafe.Pointer(&link)))
	check(hr)
	defer syscall.SyscallN(link.table[2], uintptr(unsafe.Pointer(link)))
	text, _ := windows.UTF16PtrFromString(target)
	hr, _, _ = syscall.SyscallN(link.table[20], uintptr(unsafe.Pointer(link)), uintptr(unsafe.Pointer(text))) // SetPath
	runtime.KeepAlive(text)
	check(hr)
	hr, _, _ = syscall.SyscallN(link.table[0], uintptr(unsafe.Pointer(link)), uintptr(unsafe.Pointer(&shortcutPersist)), uintptr(unsafe.Pointer(&persist)))
	check(hr)
	defer syscall.SyscallN(persist.table[2], uintptr(unsafe.Pointer(persist)))
	text, _ = windows.UTF16PtrFromString(path)
	hr, _, _ = syscall.SyscallN(persist.table[6], uintptr(unsafe.Pointer(persist)), uintptr(unsafe.Pointer(text)), 1) // Save test fixture only
	runtime.KeepAlive(text)
	check(hr)
}
func TestDirectoryShortcutNavigationAndSharedScope(t *testing.T) {
	root := t.TempDir()
	scope, err := toolbox.NewBrowserScope(root)
	if err != nil {
		t.Fatal(err)
	}
	root = scope.Root
	target := filepath.Join(root, "相册 & 示例 [01]")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "相册入口.lnk")
	createShortcutFixture(t, link, target)
	original, _ := os.ReadFile(link)
	b, _ := newFileBrowser(t.TempDir(), "owner")
	version := shortcutTestReference(t, b, link)
	w := shortcutTestRequest(b, link, version, true, "")
	var response struct{ Path string }
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &response) != nil || response.Path != target {
		t.Fatal("local shortcut did not navigate", w.Code, w.Body.String())
	}
	b.scopes["fixture"] = scope
	w = shortcutTestRequest(b, filepath.Base(link), version, false, "fixture")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &response) != nil || response.Path != filepath.Base(target) {
		t.Fatal("shared shortcut lost relative scope", w.Code, w.Body.String())
	}
	outside := t.TempDir()
	outsideLink := filepath.Join(root, "outside.lnk")
	createShortcutFixture(t, outsideLink, outside)
	w = shortcutTestRequest(b, filepath.Base(outsideLink), shortcutTestReference(t, b, outsideLink), false, "fixture")
	if w.Code != 400 || strings.Contains(w.Body.String(), outside) {
		t.Fatal("shortcut escaped or exposed shared root", w.Code, w.Body.String())
	}
	for _, tc := range []struct{ name, target string }{{"broken.lnk", filepath.Join(root, "missing")}, {"program.lnk", filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")}} {
		p := filepath.Join(root, tc.name)
		createShortcutFixture(t, p, tc.target)
		if w = shortcutTestRequest(b, p, shortcutTestReference(t, b, p), true, ""); w.Code != 400 {
			t.Fatal("non-directory shortcut accepted", tc.name, w.Code)
		}
	}
	current, _ := os.ReadFile(link)
	if !bytes.Equal(current, original) {
		t.Fatal("reading changed shortcut bytes")
	}
	if err = os.WriteFile(link, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if w = shortcutTestRequest(b, link, version, true, ""); w.Code != 400 {
		t.Fatal("stale selection accepted")
	}
}
