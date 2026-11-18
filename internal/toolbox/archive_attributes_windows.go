//go:build windows

package toolbox

import "syscall"

func archiveCanLink(source string) bool {
	s, e := syscall.UTF16PtrFromString(source)
	if e != nil {
		return false
	}
	attrs, e := syscall.GetFileAttributes(s)
	// Windows cleanup may clear READONLY on a link, changing the original too.
	return e == nil && attrs&(syscall.FILE_ATTRIBUTE_READONLY|syscall.FILE_ATTRIBUTE_HIDDEN|syscall.FILE_ATTRIBUTE_SYSTEM) == 0
}

func copyArchiveAttributes(source, target string) error {
	s, e := syscall.UTF16PtrFromString(source)
	if e != nil {
		return e
	}
	attrs, e := syscall.GetFileAttributes(s)
	if e != nil {
		return e
	}
	attrs &= syscall.FILE_ATTRIBUTE_READONLY | syscall.FILE_ATTRIBUTE_HIDDEN | syscall.FILE_ATTRIBUTE_SYSTEM | syscall.FILE_ATTRIBUTE_ARCHIVE
	if attrs == 0 {
		attrs = syscall.FILE_ATTRIBUTE_NORMAL
	}
	t, e := syscall.UTF16PtrFromString(target)
	if e != nil {
		return e
	}
	return syscall.SetFileAttributes(t, attrs)
}

func setArchiveMarkerAttributes(path string) error {
	p, e := syscall.UTF16PtrFromString(path)
	if e != nil {
		return e
	}
	return syscall.SetFileAttributes(p, syscall.FILE_ATTRIBUTE_HIDDEN|syscall.FILE_ATTRIBUTE_SYSTEM|syscall.FILE_ATTRIBUTE_ARCHIVE)
}

func clearArchiveMarkerAttributes(path string) {
	p, e := syscall.UTF16PtrFromString(path)
	if e == nil {
		_ = syscall.SetFileAttributes(p, syscall.FILE_ATTRIBUTE_NORMAL)
	}
}
