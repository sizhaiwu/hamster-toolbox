//go:build windows

package toolbox

import (
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var pathKernel = syscall.NewLazyDLL("kernel32.dll")
var finalPathProc = pathKernel.NewProc("GetFinalPathNameByHandleW")
var windowsDirectoryProc = pathKernel.NewProc("GetWindowsDirectoryW")

type fileIdentity struct{ volume, high, low uint32 }

func snapshotFileIdentity(path string) (fileIdentity, error) {
	p, e := syscall.UTF16PtrFromString(path)
	if e != nil {
		return fileIdentity{}, e
	}
	h, e := syscall.CreateFile(p, 0, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil, syscall.OPEN_EXISTING, 0x02000000, 0)
	if e != nil {
		return fileIdentity{}, e
	}
	defer syscall.CloseHandle(h)
	var info syscall.ByHandleFileInformation
	if e := syscall.GetFileInformationByHandle(h, &info); e != nil {
		return fileIdentity{}, e
	}
	return fileIdentity{info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow}, nil
}
func sameFileIdentity(a, b fileIdentity) bool { return a == b }

func systemDirectory() string {
	b := make([]uint16, 32768)
	n, _, _ := windowsDirectoryProc.Call(uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)))
	if n == 0 || n >= uintptr(len(b)) {
		return `C:\Windows`
	}
	return syscall.UTF16ToString(b[:n])
}

// The handle resolves junctions and 8.3 aliases as well as normal symlinks.
func physicalExistingPath(path string) (string, error) {
	p, e := syscall.UTF16PtrFromString(path)
	if e != nil {
		return "", e
	}
	h, e := syscall.CreateFile(p, 0, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil, syscall.OPEN_EXISTING, 0x02000000, 0)
	if e != nil {
		return "", e
	}
	defer syscall.CloseHandle(h)
	b := make([]uint16, 32768)
	n, _, callErr := finalPathProc.Call(uintptr(h), uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), 0)
	if n == 0 {
		return "", callErr
	}
	if n >= uintptr(len(b)) {
		return "", syscall.ENAMETOOLONG
	}
	v := syscall.UTF16ToString(b[:n])
	if strings.HasPrefix(v, `\\?\UNC\`) {
		v = `\\` + strings.TrimPrefix(v, `\\?\UNC\`)
	} else {
		v = strings.TrimPrefix(v, `\\?\`)
	}
	return filepath.Clean(v), nil
}

func pathRedirects(path string) bool {
	p, e := syscall.UTF16PtrFromString(path)
	if e != nil {
		return true
	}
	var d syscall.Win32finddata
	h, e := syscall.FindFirstFile(p, &d)
	if e != nil {
		return false
	} // Access errors are handled by canonicalization/stat.
	syscall.FindClose(h)
	// Name-surrogate tags include junctions/symlinks, but not OneDrive files.
	return d.FileAttributes&0x400 != 0 && d.Reserved0&0x20000000 != 0
}
