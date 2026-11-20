//go:build !windows

package main

import "errors"

func readDirectoryShortcut(string) (string, error) {
	return "", errors.New("Windows .lnk 目录快捷方式需要在 Windows 主机上打开；当前系统请直接访问目标目录")
}
