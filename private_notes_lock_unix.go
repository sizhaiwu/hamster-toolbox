//go:build linux || darwin

package main

import (
	"golang.org/x/sys/unix"
	"os"
)

func lockNotesFile(f *os.File) error { return unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) }
