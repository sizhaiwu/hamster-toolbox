//go:build windows

package main

import (
	"context"
	"net"
	"os/exec"
	"syscall"

	"github.com/Microsoft/go-winio"
)

func nativePlayerEndpoint(id string) (string, string, error) {
	return `\\.\pipe\hamster-mpv-` + id, "", nil
}
func dialNativePlayer(ctx context.Context, path string) (net.Conn, error) {
	return winio.DialPipeContext(ctx, path)
}
func prepareNativePlayerCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
}
