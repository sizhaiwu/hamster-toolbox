//go:build windows

package main

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var shortcutOLE = windows.NewLazySystemDLL("ole32.dll")
var shortcutInitialize = shortcutOLE.NewProc("CoInitializeEx")
var shortcutUninitialize = shortcutOLE.NewProc("CoUninitialize")
var shortcutCreate = shortcutOLE.NewProc("CoCreateInstance")
var shortcutClass = windows.GUID{Data1: 0x00021401, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
var shortcutInterface = windows.GUID{Data1: 0x000214f9, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
var shortcutPersist = windows.GUID{Data1: 0x0000010b, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}

type shortcutCOM struct{ table *[21]uintptr }

func shortcutHRESULT(result uintptr) error {
	if int32(result) < 0 {
		return fmt.Errorf("读取快捷方式失败（0x%08x）", uint32(result))
	}
	return nil
}

func readDirectoryShortcut(path string) (string, error) {
	// ShellLink/IPersistFile reads stored data. Never call ShellExecute, Resolve,
	// Save, or launch the stored target/arguments. No PowerShell dependency.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := shortcutInitialize.Call(0, 2) // COINIT_APARTMENTTHREADED
	if err := shortcutHRESULT(hr); err != nil {
		return "", err
	}
	defer shortcutUninitialize.Call()
	var link *shortcutCOM
	hr, _, _ = shortcutCreate.Call(uintptr(unsafe.Pointer(&shortcutClass)), 0, 1, uintptr(unsafe.Pointer(&shortcutInterface)), uintptr(unsafe.Pointer(&link)))
	if err := shortcutHRESULT(hr); err != nil {
		return "", err
	}
	defer syscall.SyscallN(link.table[2], uintptr(unsafe.Pointer(link)))
	var persist *shortcutCOM
	hr, _, _ = syscall.SyscallN(link.table[0], uintptr(unsafe.Pointer(link)), uintptr(unsafe.Pointer(&shortcutPersist)), uintptr(unsafe.Pointer(&persist)))
	if err := shortcutHRESULT(hr); err != nil {
		return "", err
	}
	defer syscall.SyscallN(persist.table[2], uintptr(unsafe.Pointer(persist)))
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	hr, _, _ = syscall.SyscallN(persist.table[5], uintptr(unsafe.Pointer(persist)), uintptr(unsafe.Pointer(file)), 0) // Load, STGM_READ
	runtime.KeepAlive(file)
	if err = shortcutHRESULT(hr); err != nil {
		return "", err
	}
	buffer := make([]uint16, 32768)
	hr, _, _ = syscall.SyscallN(link.table[3], uintptr(unsafe.Pointer(link)), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), 0, 0)
	if err = shortcutHRESULT(hr); err != nil {
		return "", err
	}
	target := windows.UTF16ToString(buffer)
	if target == "" {
		return "", errors.New("快捷方式没有可访问的文件夹目标，可能已失效或属于特殊系统入口")
	}
	return target, nil
}
