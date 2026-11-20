//go:build windows

package main

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func openNativeDirectory(path string) error {
	verb, err := windows.UTF16PtrFromString("explore")
	if err != nil {
		return err
	}
	folder, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	// ShellExecute receives the folder separately from parameters. This avoids
	// Explorer command-line parsing of commas and other characters in folder names.
	if err = windows.ShellExecute(0, verb, folder, nil, nil, windows.SW_SHOWNORMAL); err != nil {
		return fmt.Errorf("打开系统文件管理器失败：%w", err)
	}
	return nil
}
