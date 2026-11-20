//go:build !windows

package main

import (
	"errors"
	"os"
	"syscall"
)

func consoleAddressInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE)
}

func enableConsoleColor() (bool, func()) {
	info, err := os.Stdout.Stat()
	color := err == nil && info.Mode()&os.ModeCharDevice != 0 && os.Getenv("TERM") != "" && consoleColorAllowed()
	return color, func() {}
}
