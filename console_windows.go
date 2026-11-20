//go:build windows

package main

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func consoleAddressInUse(err error) bool {
	return errors.Is(err, windows.WSAEADDRINUSE)
}

func enableConsoleColor() (bool, func()) {
	if !consoleColorAllowed() {
		return false, func() {}
	}
	handle := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if windows.GetConsoleMode(handle, &mode) != nil {
		return false, func() {} // Redirection and older consoles get plain text.
	}
	if windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) != nil {
		return false, func() {}
	}
	return true, func() { _ = windows.SetConsoleMode(handle, mode) }
}
