//go:build !windows && !darwin

package main

import (
	"context"
	"errors"
)

func openNativeFileWith(ctx context.Context, file, program string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if program == "" {
		return errors.New("请选择本机程序，例如 /usr/bin/vlc 或 /usr/bin/gedit")
	}
	return startExternalFile(program, file)
}
