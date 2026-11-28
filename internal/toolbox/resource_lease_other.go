//go:build !windows

package toolbox

import (
	"os"
	"syscall"
)

func lockResourceFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}
