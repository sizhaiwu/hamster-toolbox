//go:build linux

package toolbox

import (
	"golang.org/x/sys/unix"
	"os"
	"strconv"
)

func browserHandlePath(f *os.File) (string, error) {
	return os.Readlink("/proc/self/fd/" + strconv.FormatUint(uint64(f.Fd()), 10))
}
func browserMoveNoReplace(source, target string) error {
	return unix.Renameat2(unix.AT_FDCWD, source, unix.AT_FDCWD, target, unix.RENAME_NOREPLACE)
}
