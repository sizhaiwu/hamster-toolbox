//go:build !windows && !linux && !darwin

package toolbox

import (
	"errors"
	"os"
	"path/filepath"
)

func browserHandlePath(f *os.File) (string, error) { return filepath.EvalSymlinks(f.Name()) }
func browserMoveNoReplace(source, target string) error {
	if _, e := os.Lstat(target); !os.IsNotExist(e) {
		return errors.New("目标已存在")
	}
	return os.Rename(source, target)
}
