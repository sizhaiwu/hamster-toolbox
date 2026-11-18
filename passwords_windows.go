//go:build windows

package main

import (
	"errors"
	"runtime"
	"syscall"
	"unsafe"
)

type passwordBlob struct {
	Size uint32
	Data *byte
}

var passwordCryptDLL = syscall.NewLazyDLL("crypt32.dll")
var passwordProtectProc = passwordCryptDLL.NewProc("CryptProtectData")
var passwordUnprotectProc = passwordCryptDLL.NewProc("CryptUnprotectData")
var passwordLocalFree = syscall.NewLazyDLL("kernel32.dll").NewProc("LocalFree")

func passwordCrypt(data []byte, encrypt bool) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("常用密码数据为空")
	}
	in := passwordBlob{uint32(len(data)), &data[0]}
	var out passwordBlob
	proc := passwordUnprotectProc
	if encrypt {
		proc = passwordProtectProc
	}
	ok, _, _ := proc.Call(uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0, 1, uintptr(unsafe.Pointer(&out)))
	runtime.KeepAlive(data)
	runtime.KeepAlive(in)
	if ok == 0 {
		return nil, errors.New("Windows 无法处理常用密码，请使用保存它的 Windows 账户")
	}
	defer passwordLocalFree.Call(uintptr(unsafe.Pointer(out.Data)))
	result := append([]byte(nil), unsafe.Slice(out.Data, int(out.Size))...)
	return result, nil
}
