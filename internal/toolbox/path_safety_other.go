//go:build !windows

package toolbox

import (
	"os"
	"path/filepath"
)

type fileIdentity struct{ info os.FileInfo }

func snapshotFileIdentity(path string) (fileIdentity, error) {
	info, e := os.Stat(path)
	return fileIdentity{info}, e
}
func sameFileIdentity(a, b fileIdentity) bool { return os.SameFile(a.info, b.info) }

func systemDirectory() string                          { return "/etc" }
func physicalExistingPath(path string) (string, error) { return filepath.EvalSymlinks(path) }
func pathRedirects(path string) bool {
	s, e := os.Lstat(path)
	return e == nil && s.Mode()&os.ModeSymlink != 0
}
