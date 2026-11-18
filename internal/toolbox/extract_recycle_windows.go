//go:build windows

package toolbox

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

var recycleOle = syscall.NewLazyDLL("ole32.dll")
var recycleShell = syscall.NewLazyDLL("shell32.dll")
var recycleKernel = syscall.NewLazyDLL("kernel32.dll")
var recycleInitialize = recycleOle.NewProc("CoInitializeEx")
var recycleUninitialize = recycleOle.NewProc("CoUninitialize")
var recycleCreate = recycleOle.NewProc("CoCreateInstance")
var recycleFree = recycleOle.NewProc("CoTaskMemFree")
var recycleCreateItem = recycleShell.NewProc("SHCreateItemFromParsingName")
var recycleVolumePath = recycleKernel.NewProc("GetVolumePathNameW")
var recycleDriveType = recycleKernel.NewProc("GetDriveTypeW")

var recycleClass = syscall.GUID{Data1: 0x3ad05575, Data2: 0x8857, Data3: 0x4850, Data4: [8]byte{0x92, 0x77, 0x11, 0xb8, 0x5b, 0xdb, 0x8e, 0x09}}
var recycleInterface = syscall.GUID{Data1: 0x947aab5f, Data2: 0x0a5c, Data3: 0x4c13, Data4: [8]byte{0xb4, 0xd6, 0x4b, 0xf7, 0x83, 0x6f, 0xc9, 0xf8}}
var recycleItemInterface = syscall.GUID{Data1: 0x43826d1e, Data2: 0xe718, Data3: 0x42ee, Data4: [8]byte{0xbc, 0x55, 0xa1, 0xe2, 0x61, 0xc3, 0x7b, 0xfe}}
var recycleSinkInterface = syscall.GUID{Data1: 0x04b0f1a7, Data2: 0x9490, Data3: 0x44bc, Data4: [8]byte{0x96, 0xe1, 0x42, 0x96, 0xa3, 0x12, 0x52, 0xe2}}
var recycleUnknownInterface = syscall.GUID{Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}

type recycleObject struct{ vtbl *[23]uintptr }
type recycleSink struct {
	vtbl    *[19]uintptr
	refs    atomic.Int32
	mu      sync.Mutex
	result  recycleResult
	items   []*recycleObject
	failed  uint32
	refused bool
}

// COM may retain the sink beyond PerformOperations. Keep a Go reference until
// its actual final Release, rather than relying only on the current stack.
var recycleLiveSinks sync.Map

func recycleDropItems(this *recycleSink) {
	this.mu.Lock()
	items := this.items
	this.items = nil
	this.mu.Unlock()
	for _, item := range items {
		recycleRelease(item)
	}
}

func recycleCall(obj *recycleObject, index int, args ...uintptr) uintptr {
	parameters := append([]uintptr{uintptr(unsafe.Pointer(obj))}, args...)
	hr, _, _ := syscall.SyscallN(obj.vtbl[index], parameters...)
	runtime.KeepAlive(obj)
	return hr
}
func recycleRelease(obj *recycleObject) {
	if obj != nil {
		recycleCall(obj, 2)
	}
}
func recycleFailed(hr uintptr) bool { return int32(uint32(hr)) < 0 }
func recycleError(hr uintptr) error {
	return fmt.Errorf("Windows 无法完成回收站操作（0x%08X）；未完成的原文件已保留，不会强制永久删除", uint32(hr))
}

func recycleQuery(this *recycleSink, iid *syscall.GUID, out *unsafe.Pointer) uintptr {
	if out == nil {
		return 0x80004003
	}
	*out = nil
	if iid != nil && (*iid == recycleSinkInterface || *iid == recycleUnknownInterface) {
		*out = unsafe.Pointer(this)
		this.refs.Add(1)
		return 0
	}
	return 0x80004002
}
func recycleAddRef(this *recycleSink) uintptr { return uintptr(this.refs.Add(1)) }
func recycleReleaseRef(this *recycleSink) uintptr {
	left := this.refs.Add(-1)
	if left == 0 {
		recycleDropItems(this)
		recycleLiveSinks.Delete(this)
	}
	return uintptr(left)
}
func recycleNoop(this *recycleSink) uintptr { return 0 }
func recycleFinish(this *recycleSink, hr uint32) uintptr {
	if int32(hr) < 0 {
		this.mu.Lock()
		this.failed = hr
		this.mu.Unlock()
	}
	return 0
}
func recyclePreRename(this *recycleSink, flags uint32, item *recycleObject, name *uint16) uintptr {
	return 0
}
func recyclePostRename(this *recycleSink, flags uint32, item *recycleObject, name *uint16, hr uint32, next *recycleObject) uintptr {
	return 0
}
func recyclePreTransfer(this *recycleSink, flags uint32, item, dest *recycleObject, name *uint16) uintptr {
	return 0
}
func recyclePostTransfer(this *recycleSink, flags uint32, item, dest *recycleObject, name *uint16, hr uint32, next *recycleObject) uintptr {
	return 0
}
func recyclePreDelete(this *recycleSink, flags uint32, item *recycleObject) uintptr {
	// TSF_DELETE_RECYCLE_IF_POSSIBLE is absent for a permanent-delete fallback.
	// Return a real failing HRESULT, cancelling the operation before deletion.
	if flags&0x80 == 0 {
		this.mu.Lock()
		this.refused = true
		this.mu.Unlock()
		return 0x80004004
	}
	return 0
}
func recyclePostDelete(this *recycleSink, flags uint32, item *recycleObject, hr uint32, next *recycleObject) uintptr {
	this.mu.Lock()
	defer this.mu.Unlock()
	if int32(hr) < 0 {
		this.failed = hr
		return 0
	}
	if next == nil {
		this.failed = 0x80004005
		return 0x80004005
	}
	this.result.Count++
	// Resolve the recycled name after PerformOperations has finalized it. During
	// PostDeleteItem the shell may still be completing the destination item.
	recycleCall(next, 1)
	this.items = append(this.items, next)
	return 0
}

func recycleUTF16String(path *uint16) string {
	var text []uint16
	for i := 0; i < 32768; i++ {
		value := *(*uint16)(unsafe.Add(unsafe.Pointer(path), i*2))
		if value == 0 {
			break
		}
		text = append(text, value)
	}
	return syscall.UTF16ToString(text)
}
func recyclePostNew(this *recycleSink, flags uint32, dest *recycleObject, name, template *uint16, attributes, hr uint32, next *recycleObject) uintptr {
	return 0
}
func recycleProgress(this *recycleSink, total, done uint32) uintptr { return 0 }

var recycleSinkTable = [19]uintptr{
	syscall.NewCallback(recycleQuery), syscall.NewCallback(recycleAddRef), syscall.NewCallback(recycleReleaseRef),
	syscall.NewCallback(recycleNoop), syscall.NewCallback(recycleFinish),
	syscall.NewCallback(recyclePreRename), syscall.NewCallback(recyclePostRename),
	syscall.NewCallback(recyclePreTransfer), syscall.NewCallback(recyclePostTransfer),
	syscall.NewCallback(recyclePreTransfer), syscall.NewCallback(recyclePostTransfer),
	syscall.NewCallback(recyclePreDelete), syscall.NewCallback(recyclePostDelete),
	syscall.NewCallback(recyclePreRename), syscall.NewCallback(recyclePostNew),
	syscall.NewCallback(recycleProgress), syscall.NewCallback(recycleNoop), syscall.NewCallback(recycleNoop), syscall.NewCallback(recycleNoop),
}

func recycleLocalPath(path string) error {
	if !filepath.IsAbs(path) || strings.HasPrefix(path, `\\`) {
		return errors.New("此位置不支持回收站，原文件已保留；可另选永久删除")
	}
	p, e := syscall.UTF16PtrFromString(path)
	if e != nil {
		return e
	}
	buffer := make([]uint16, 32768)
	ok, _, _ := recycleVolumePath.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	runtime.KeepAlive(p)
	if ok == 0 {
		return errors.New("无法确认文件所在磁盘是否支持回收站，原文件已保留")
	}
	drive, _, _ := recycleDriveType.Call(uintptr(unsafe.Pointer(&buffer[0])))
	runtime.KeepAlive(buffer)
	if drive != 3 {
		return errors.New("此磁盘或网络位置不支持本工具的回收站清理，原文件已保留；可另选永久删除")
	}
	return nil
}

func recycleExtractFiles(paths []string) (recycleResult, error) {
	if len(paths) == 0 {
		return recycleResult{}, nil
	}
	for _, path := range paths {
		if e := recycleLocalPath(path); e != nil {
			return recycleResult{}, e
		}
		st, e := os.Lstat(path)
		if e != nil || !st.Mode().IsRegular() {
			return recycleResult{}, errors.New("待回收的文件不存在或不是普通文件，已停止清理")
		}
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := recycleInitialize.Call(0, 2|4)
	if recycleFailed(hr) {
		return recycleResult{}, recycleError(hr)
	}
	defer recycleUninitialize.Call()
	sink := &recycleSink{vtbl: &recycleSinkTable}
	sink.refs.Store(1)
	recycleLiveSinks.Store(sink, struct{}{})
	defer func() {
		recycleDropItems(sink)
		recycleReleaseRef(sink)
		runtime.KeepAlive(sink)
	}()
	var operation *recycleObject
	hr, _, _ = recycleCreate.Call(uintptr(unsafe.Pointer(&recycleClass)), 0, 1, uintptr(unsafe.Pointer(&recycleInterface)), uintptr(unsafe.Pointer(&operation)))
	if recycleFailed(hr) {
		return recycleResult{}, recycleError(hr)
	}
	defer recycleRelease(operation)
	// Windows 10/11: recycle and stop on errors, without modal UI or touching
	// Explorer's separate Ctrl+Z history. Items remain restorable in Recycle Bin.
	const flags = 0x00080000 | 0x00100000 | 0x0400 | 0x0010 | 0x0004 | 0x2000
	if hr = recycleCall(operation, 5, flags); recycleFailed(hr) {
		return recycleResult{}, recycleError(hr)
	}
	for _, path := range paths {
		p, e := syscall.UTF16PtrFromString(path)
		if e != nil {
			return recycleResult{}, e
		}
		var item *recycleObject
		hr, _, _ = recycleCreateItem.Call(uintptr(unsafe.Pointer(p)), 0, uintptr(unsafe.Pointer(&recycleItemInterface)), uintptr(unsafe.Pointer(&item)))
		runtime.KeepAlive(p)
		if recycleFailed(hr) {
			return recycleResult{}, recycleError(hr)
		}
		hr = recycleCall(operation, 18, uintptr(unsafe.Pointer(item)), uintptr(unsafe.Pointer(sink)))
		recycleRelease(item)
		if recycleFailed(hr) {
			return recycleResult{}, recycleError(hr)
		}
	}
	hr = recycleCall(operation, 21)
	var aborted int32
	status := recycleCall(operation, 22, uintptr(unsafe.Pointer(&aborted)))
	sink.mu.Lock()
	defer sink.mu.Unlock()
	for _, item := range sink.items {
		var path *uint16
		if result := recycleCall(item, 5, 0x80058000, uintptr(unsafe.Pointer(&path))); !recycleFailed(result) && path != nil {
			sink.result.Paths = append(sink.result.Paths, recycleUTF16String(path))
			recycleFree.Call(uintptr(unsafe.Pointer(path)))
		}
	}
	if sink.refused {
		return sink.result, errors.New("系统无法将部分文件移入回收站，未完成的原文件已保留；不会改为永久删除")
	}
	if recycleFailed(hr) {
		return sink.result, recycleError(hr)
	}
	if recycleFailed(status) {
		return sink.result, recycleError(status)
	}
	if sink.failed != 0 {
		return sink.result, recycleError(uintptr(sink.failed))
	}
	if aborted != 0 || sink.result.Count != len(paths) {
		return sink.result, errors.New("回收站操作未全部完成，未完成的原文件已保留")
	}
	return sink.result, nil
}
