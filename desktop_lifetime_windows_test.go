//go:build windows

package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

//go:noinline
func desktopGrowCallbackStack(depth int) byte {
	var space [2048]byte
	space[depth%len(space)] = byte(depth)
	if depth > 0 {
		space[0] = desktopGrowCallbackStack(depth - 1)
	}
	runtime.KeepAlive(&space)
	return space[0]
}

// Native calls can synchronously invoke a window procedure on the same Go
// stack. The native pointer must stay stable across stack growth in callbacks.
// No native code dereferences the test pointer, so the old bug fails safely.
//
//go:noinline
func desktopPointerSurvivesCallback(window uintptr) bool {
	var message desktopMessageData
	before := uintptr(unsafe.Pointer(&message))
	desktopCall("SendMessageW", window, 0x8055, uintptr(unsafe.Pointer(&message)), 0)
	return before == uintptr(unsafe.Pointer(&message))
}

func TestDesktopNativePointerLifetime(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	name := desktopUTF16("HamsterToolbox.PointerLifetime." + randomID())
	callback := windows.NewCallback(func(window uintptr, message uint32, wparam, lparam uintptr) uintptr {
		if message == 0x8055 {
			return uintptr(desktopGrowCallbackStack(256))
		}
		return desktopCall("DefWindowProcW", window, uintptr(message), wparam, lparam)
	})
	class := desktopWindowClass{Size: uint32(unsafe.Sizeof(desktopWindowClass{})), ClassName: name, Proc: callback}
	// Use direct calls for setup so the regression isolates desktopCall itself.
	registered, _, err := desktopUser.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&class)))
	if registered == 0 {
		t.Fatal("register test class", err)
	}
	defer desktopUser.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(name)), 0)
	window, _, err := desktopUser.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(name)), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	if window == 0 {
		t.Fatal("create test window", err)
	}
	defer desktopCall("DestroyWindow", window)
	if !desktopPointerSurvivesCallback(window) {
		t.Fatal("native pointer moved during a synchronous window callback")
	}
}

func TestDesktopTraySurvivesGCAndTaskbarRecreation(t *testing.T) {
	if os.Getenv("TOOLBOX_DESKTOP_TRAY_TEST") != "1" {
		t.Skip("requires an interactive Windows desktop with a system tray")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	service := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})}
	defer service.Close()
	s := &server{dataDir: t.TempDir(), token: "synthetic-tray-test"}
	address := "http://" + listener.Addr().String()
	key := desktopUTF16(desktopInstanceName(s.dataDir))
	checked := make(chan error, 1)
	go func() {
		defer service.Close() // exercise the normal server-stopped tray cleanup
		client := &http.Client{Timeout: time.Second}
		defer client.CloseIdleConnections()
		var window uintptr
		for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
			window = desktopCall("FindWindowW", uintptr(unsafe.Pointer(key)), 0)
			if window != 0 {
				break
			}
		}
		if window == 0 {
			checked <- fmt.Errorf("test tray window did not appear")
			return
		}
		created := desktopCall("RegisterWindowMessageW", uintptr(unsafe.Pointer(desktopUTF16("TaskbarCreated"))))
		for i := 0; i < 60; i++ {
			runtime.GC()
			// Delete only this synthetic tray icon, then deliver the Explorer
			// restart notification only to this test window. Never restart the
			// user's Explorer or broadcast test messages to other applications.
			if i%10 == 0 {
				notify := desktopNotifyIcon{Size: uint32(unsafe.Sizeof(desktopNotifyIcon{})), Hwnd: window, ID: 1}
				desktopShell.NewProc("Shell_NotifyIconW").Call(2, uintptr(unsafe.Pointer(&notify)))
				desktopCall("SendMessageW", window, created, 0, 0)
			}
			desktopCall("PostMessageW", window, 0, 0, 0)
			response, err := client.Get(address)
			if err != nil {
				checked <- fmt.Errorf("tray service stopped during GC cycle %d: %w", i, err)
				return
			}
			response.Body.Close()
			if response.StatusCode != http.StatusNoContent {
				checked <- fmt.Errorf("unexpected tray service status: %d", response.StatusCode)
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		checked <- nil
	}()
	if err := runDesktopTray(address, s, service, listener); err != nil {
		t.Fatal(err)
	}
	if err := <-checked; err != nil {
		t.Fatal(err)
	}
}
