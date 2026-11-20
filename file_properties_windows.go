//go:build windows

package main

import (
	"os"
	"time"

	"golang.org/x/sys/windows"
)

func addNativeFileProperties(f *os.File, properties map[string]any) {
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &info) != nil {
		return
	}
	properties["Created"] = time.Unix(0, info.CreationTime.Nanoseconds())
	properties["Accessed"] = time.Unix(0, info.LastAccessTime.Nanoseconds())
	properties["ReadOnly"] = info.FileAttributes&windows.FILE_ATTRIBUTE_READONLY != 0
	properties["Hidden"] = info.FileAttributes&windows.FILE_ATTRIBUTE_HIDDEN != 0
}
