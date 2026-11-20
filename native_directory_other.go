//go:build !windows

package main

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

func openNativeDirectory(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	program, args := "xdg-open", []string{path}
	if runtime.GOOS == "darwin" {
		program, args = "open", []string{"-a", "Finder", "--", path}
	}
	if err := exec.CommandContext(ctx, program, args...).Run(); err != nil {
		return fmt.Errorf("无法打开系统文件管理器，请确认本机有桌面环境：%w", err)
	}
	return nil
}
