//go:build !windows

package main

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
)

func nativePlayerEndpoint(id string) (string, string, error) {
	dir, err := os.MkdirTemp("", "hamster-mpv-")
	if err != nil {
		return "", "", err
	}
	return filepath.Join(dir, "player.sock"), dir, nil
}
func dialNativePlayer(ctx context.Context, path string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "unix", path)
}
func prepareNativePlayerCommand(cmd *exec.Cmd) {}
