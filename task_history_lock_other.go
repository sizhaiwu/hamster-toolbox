//go:build !windows

package main

import (
	"os"
	"syscall"
)

func lockTaskRecord(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}
func unlockTaskRecord(file *os.File) { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN) }
