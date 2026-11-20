//go:build windows

package main

import (
	"context"
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var openWithDialogProc = windows.NewLazySystemDLL("shell32.dll").NewProc("SHOpenWithDialog")
var openWithCOM = windows.NewLazySystemDLL("ole32.dll")
var openWithCOMInit = openWithCOM.NewProc("CoInitializeEx")
var openWithCOMClose = openWithCOM.NewProc("CoUninitialize")

type openAsInfo struct {
	File, Class *uint16
	Flags       uint32
}

func openNativeFileWith(ctx context.Context, file, program string) error {
	if program != "" {
		return startExternalFile(program, file)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := openWithDialogProc.Find(); err != nil {
		return fmt.Errorf("当前系统无法显示打开方式窗口：%w", err)
	}
	name, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	initialized, _, _ := openWithCOMInit.Call(0, 2) // COINIT_APARTMENTTHREADED
	if int32(initialized) < 0 {
		return fmt.Errorf("初始化系统程序选择窗口失败：0x%08X", uint32(initialized))
	}
	defer openWithCOMClose.Call()
	// OAIF_EXEC only: choose an app for this file without registering a default.
	// https://learn.microsoft.com/windows/win32/api/shlobj_core/ns-shlobj_core-openasinfo
	info := openAsInfo{File: name, Flags: 4}
	result, _, _ := openWithDialogProc.Call(0, uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(name)
	if uint32(result) == 0x800704c7 || uint32(result) == 0x80004004 || result == 1 {
		return errOpenWithCancelled
	}
	if int32(result) < 0 {
		return fmt.Errorf("无法打开系统程序选择窗口：0x%08X", uint32(result))
	}
	return nil
}
