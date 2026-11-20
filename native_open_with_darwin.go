//go:build darwin

package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func openNativeFileWith(ctx context.Context, file, program string) error {
	if program != "" {
		return startExternalFile(program, file)
	}
	pickerCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	// User data never becomes AppleScript source. Only the chooser is scripted.
	script := "set chosenApp to choose application with prompt \"选择查看或编辑此文件的应用\" as alias\nreturn POSIX path of chosenApp"
	output, err := exec.CommandContext(pickerCtx, "/usr/bin/osascript", "-e", script).Output()
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok && strings.Contains(string(e.Stderr), "(-128)") {
			return errOpenWithCancelled
		}
		return fmt.Errorf("无法显示应用选择窗口，请确认本机有桌面环境：%w", err)
	}
	application := strings.TrimSpace(string(output))
	if err := validateExternalProgram(application); err != nil {
		return err
	}
	return startExternalFile(application, file)
}
