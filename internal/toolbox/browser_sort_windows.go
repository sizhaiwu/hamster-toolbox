//go:build windows

package toolbox

import (
	"golang.org/x/sys/windows"
	"unsafe"
)

var browserLogicalCompare = windows.NewLazySystemDLL("shlwapi.dll").NewProc("StrCmpLogicalW")

// Use the Windows logical comparison for Explorer-style numeric and locale ordering.
func BrowserNameLess(a, b string) bool {
	aa, err := windows.UTF16PtrFromString(a)
	if err != nil {
		return naturalLess(a, b)
	}
	bb, err := windows.UTF16PtrFromString(b)
	if err != nil {
		return naturalLess(a, b)
	}
	if browserLogicalCompare.Find() != nil {
		return naturalLess(a, b)
	}
	result, _, _ := browserLogicalCompare.Call(uintptr(unsafe.Pointer(aa)), uintptr(unsafe.Pointer(bb)))
	if int32(result) != 0 {
		return int32(result) < 0
	}
	return a < b
}
