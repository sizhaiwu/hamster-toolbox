//go:build windows

package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var desktopUser = windows.NewLazySystemDLL("user32.dll")
var desktopKernel = windows.NewLazySystemDLL("kernel32.dll")
var desktopShell = windows.NewLazySystemDLL("shell32.dll")

const desktopTrayEvent = 0x8001
const desktopOpenEvent = 0x8002
const desktopServerStopped = 0x8003

func desktopUTF16(s string) *uint16 { p, _ := windows.UTF16PtrFromString(s); return p }

// Like LazyProc.Call, this wrapper receives native pointers disguised as
// uintptr. Keep their objects alive and off the movable Go stack for the whole
// call, including synchronous Windows callbacks and a blocking GetMessageW.
// Without the directive a callback can grow the stack while user32 still holds
// the old MSG address, causing delayed memory corruption in an idle tray.
//
//go:uintptrescapes
func desktopCall(name string, args ...uintptr) uintptr {
	r, _, _ := desktopUser.NewProc(name).Call(args...)
	return r
}

func desktopMessage(owner uintptr, text string, flags uintptr) uintptr {
	return desktopCall("MessageBoxW", owner, uintptr(unsafe.Pointer(desktopUTF16(text))), uintptr(unsafe.Pointer(desktopUTF16("仓鼠工具箱"))), flags)
}
func desktopError(text string) { desktopMessage(0, text, 0x10) }

func prepareDesktop(data string, tray, noOpen bool) (func(), bool, error) {
	// The installer checks this mutex and asks the user to exit normally;
	// it must never terminate a compression task in order to overwrite files.
	running, err := windows.CreateMutex(nil, false, desktopUTF16(`Local\HamsterToolbox.Running`))
	if running == 0 {
		return nil, false, fmt.Errorf("无法创建程序运行标记：%w", err)
	}
	cleanup := func() { windows.CloseHandle(running) }
	if !tray {
		return cleanup, false, nil
	}
	key := desktopInstanceName(data)
	instance, err := windows.CreateMutex(nil, false, desktopUTF16(`Local\`+key))
	if instance == 0 {
		cleanup()
		return nil, false, fmt.Errorf("无法创建托盘实例：%w", err)
	}
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		defer windows.CloseHandle(instance)
		defer cleanup()
		if noOpen {
			return func() {}, true, nil
		}
		for attempt := 0; attempt < 50; attempt++ {
			hwnd := desktopCall("FindWindowW", uintptr(unsafe.Pointer(desktopUTF16(key))), 0)
			if hwnd != 0 {
				desktopCall("PostMessageW", hwnd, desktopOpenEvent, 0, 0)
				return func() {}, true, nil
			}
			time.Sleep(100 * time.Millisecond)
		}
		return nil, false, errors.New("工具箱正在启动，请稍后再次打开")
	}
	// FreeConsole detaches only this process, never hides the caller's terminal.
	desktopKernel.NewProc("FreeConsole").Call()
	return func() { windows.CloseHandle(instance); cleanup() }, false, nil
}

type desktopPoint struct{ X, Y int32 }
type desktopMessageData struct {
	Hwnd           uintptr
	Message        uint32
	Wparam, Lparam uintptr
	Time           uint32
	Point          desktopPoint
	Private        uint32
}
type desktopWindowClass struct {
	Size, Style                        uint32
	Proc                               uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	SmallIcon                          uintptr
}
type desktopNotifyIcon struct {
	Size                uint32
	Hwnd                uintptr
	ID, Flags, Callback uint32
	Icon                uintptr
	Tip                 [128]uint16
	State, StateMask    uint32
	Info                [256]uint16
	Version             uint32
	InfoTitle           [64]uint16
	InfoFlags           uint32
	Guid                windows.GUID
	BalloonIcon         uintptr
}

func runDesktopTray(url string, s *server, service *http.Server, listener net.Listener) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	key := desktopInstanceName(s.dataDir)
	module, _, _ := desktopKernel.NewProc("GetModuleHandleW").Call(0)
	icon := desktopCall("LoadImageW", uintptr(module), 1, 1, 32, 32, 0x8000) // embedded brand icon, LR_SHARED
	if icon == 0 {
		icon = desktopCall("LoadIconW", 0, 32512)
	}
	taskbarCreated := desktopCall("RegisterWindowMessageW", uintptr(unsafe.Pointer(desktopUTF16("TaskbarCreated"))))
	var hwnd uintptr
	var notify desktopNotifyIcon
	addIcon := func() bool {
		notify = desktopNotifyIcon{Size: uint32(unsafe.Sizeof(desktopNotifyIcon{})), Hwnd: hwnd, ID: 1, Flags: 1 | 2 | 4, Callback: desktopTrayEvent, Icon: icon}
		copy(notify.Tip[:], syscall.StringToUTF16("仓鼠工具箱 · 单击打开，右键菜单"))
		r, _, _ := desktopShell.NewProc("Shell_NotifyIconW").Call(0, uintptr(unsafe.Pointer(&notify)))
		if r != 0 {
			notify.Version = 4
			desktopShell.NewProc("Shell_NotifyIconW").Call(4, uintptr(unsafe.Pointer(&notify)))
		}
		return r != 0
	}
	open := func(address string) {
		cmd := openBrowserCommand(address)
		if err := cmd.Start(); err != nil {
			desktopMessage(hwnd, "无法打开浏览器，请检查默认浏览器设置。", 0x10)
		} else {
			go cmd.Wait()
		}
	}
	requestExit := func() {
		if !s.desktopCanExit() {
			desktopMessage(hwnd, "工具箱任务仍在执行。请先在页面等待完成或取消，再退出。", 0x40)
			return
		}
		if desktopMessage(hwnd, "退出仓鼠工具箱？\n\n内网共享、上传、文件传输和网页播放会停止。关闭浏览器页面不会退出程序。", 0x24|0x100) != 6 {
			return
		}
		// Recheck after the modal: another browser tab may have started a task.
		if s.beginDesktopExit() {
			desktopCall("DestroyWindow", hwnd)
		} else {
			desktopMessage(hwnd, "有新任务正在执行，请先完成或取消。", 0x40)
		}
	}
	callback := windows.NewCallback(func(window uintptr, message uint32, wparam, lparam uintptr) uintptr {
		if uintptr(message) == taskbarCreated {
			addIcon()
			return 0
		}
		switch message {
		case desktopOpenEvent:
			open(url)
			return 0
		case desktopTrayEvent:
			switch uint32(lparam) & 0xffff {
			case 0x202, 0x203, 0x400, 0x401: // mouse release/double-click, NIN_SELECT/KEYSELECT
				// Version 4 emits NIN_SELECT for mouse activation; handle that
				// instead of WM_LBUTTONUP to avoid launching two browser tabs.
				if uint32(lparam)&0xffff != 0x202 && uint32(lparam)&0xffff != 0x203 {
					open(url)
				}
			case 0x7b, 0x205:
				menu := desktopCall("CreatePopupMenu")
				if menu == 0 {
					return 0
				}
				defer desktopCall("DestroyMenu", menu)
				appendItem := func(id uintptr, text string, flags uintptr) {
					desktopCall("AppendMenuW", menu, flags, id, uintptr(unsafe.Pointer(desktopUTF16(text))))
				}
				appendItem(1, "打开工具箱", 0)
				appendItem(2, "打开资源管理器", 0)
				appendItem(0, "", 0x800)
				if s.desktopCanExit() {
					appendItem(3, "退出工具箱", 0)
				} else {
					appendItem(0, "任务执行中，请完成或取消后退出", 1)
				}
				var point desktopPoint
				desktopCall("GetCursorPos", uintptr(unsafe.Pointer(&point)))
				desktopCall("SetForegroundWindow", window)
				choice := desktopCall("TrackPopupMenu", menu, 0x100|0x2, uintptr(point.X), uintptr(point.Y), 0, window, 0)
				desktopCall("PostMessageW", window, 0, 0, 0)
				switch choice {
				case 1:
					open(url)
				case 2:
					open(url[:len(url)-len("/?session="+s.token)] + "/files/?session=" + s.token)
				case 3:
					requestExit()
				}
			}
			return 0
		case 0x10:
			requestExit()
			return 0
		case desktopServerStopped:
			desktopCall("DestroyWindow", window)
			return 0
		case 2:
			desktopCall("PostQuitMessage", 0)
			return 0
		}
		return desktopCall("DefWindowProcW", window, uintptr(message), wparam, lparam)
	})
	class := desktopWindowClass{Size: uint32(unsafe.Sizeof(desktopWindowClass{})), Proc: callback, Instance: uintptr(module), ClassName: desktopUTF16(key), Icon: icon, SmallIcon: icon}
	if desktopCall("RegisterClassExW", uintptr(unsafe.Pointer(&class))) == 0 {
		return errors.New("无法注册托盘窗口")
	}
	defer desktopCall("UnregisterClassW", uintptr(unsafe.Pointer(class.ClassName)), uintptr(module))
	hwnd = desktopCall("CreateWindowExW", 0, uintptr(unsafe.Pointer(class.ClassName)), uintptr(unsafe.Pointer(desktopUTF16("仓鼠工具箱"))), 0, 0, 0, 0, 0, 0, 0, uintptr(module), 0)
	if hwnd == 0 {
		return errors.New("无法创建托盘窗口")
	}
	if !addIcon() {
		desktopCall("DestroyWindow", hwnd)
		return errors.New("无法显示系统托盘图标，请使用普通启动方式")
	}
	defer desktopShell.NewProc("Shell_NotifyIconW").Call(2, uintptr(unsafe.Pointer(&notify)))
	serveErr := make(chan error, 1)
	go func() {
		err := service.Serve(listener)
		serveErr <- err
		desktopCall("PostMessageW", hwnd, desktopServerStopped, 0, 0)
	}()
	var message desktopMessageData
	for {
		result := desktopCall("GetMessageW", uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(result) == -1 {
			return errors.New("托盘消息循环异常")
		}
		if result == 0 {
			break
		}
		desktopCall("TranslateMessage", uintptr(unsafe.Pointer(&message)))
		desktopCall("DispatchMessageW", uintptr(unsafe.Pointer(&message)))
	}
	service.Close()
	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	default:
	}
	return nil
}
