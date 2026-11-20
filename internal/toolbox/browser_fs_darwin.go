//go:build darwin

package toolbox

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
)

func browserHandlePath(f *os.File) (string, error) {
	p, e := filepath.EvalSymlinks(f.Name())
	if e != nil {
		return "", e
	}
	a, e := f.Stat()
	if e != nil {
		return "", e
	}
	b, e := os.Stat(p)
	if e != nil || !os.SameFile(a, b) {
		return "", errors.New("文件路径已变化")
	}
	return p, nil
}
func browserMoveNoReplace(source, target string) error {
	return unix.RenamexNp(source, target, unix.RENAME_EXCL)
}
