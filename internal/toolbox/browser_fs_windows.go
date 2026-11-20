//go:build windows

package toolbox

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

func browserHandlePath(f *os.File) (string, error) {
	b := make([]uint16, 32768)
	n, _, e := finalPathProc.Call(f.Fd(), uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), 0)
	if n == 0 {
		return "", e
	}
	if n >= uintptr(len(b)) {
		return "", syscall.ENAMETOOLONG
	}
	p := syscall.UTF16ToString(b[:n])
	if strings.HasPrefix(p, `\\?\UNC\`) {
		p = `\\` + strings.TrimPrefix(p, `\\?\UNC\`)
	} else {
		p = strings.TrimPrefix(p, `\\?\`)
	}
	return filepath.Clean(p), nil
}
func browserMoveNoReplace(source, target string) error {
	s, e := syscall.UTF16PtrFromString(source)
	if e != nil {
		return e
	}
	d, e := syscall.UTF16PtrFromString(target)
	if e != nil {
		return e
	}
	return syscall.MoveFile(s, d)
}
func browserRecycle(paths []string) error { _, e := recycleWindowsPaths(paths, true); return e }
