package toolbox

import (
	"errors"
	"os"
	"path/filepath"
)

// IsExtractArchiveName classifies candidate names without opening media files.
// Numeric/disguised names are still checked against their header during scan.
func IsExtractArchiveName(name string) bool { _, ok := extractArchiveName(name); return ok }

// Selecting one archive permits a read-only parent scope, including a disk
// root. It never permits scanning that parent or mutating the root itself.
// The selected file and every output retain normal write/delete protection.
func configRootPath(c Config) (string, error) {
	if c.Tool != "extract" || c.ExtractSource == "" {
		return safePath(c.Root)
	}
	source, err := safePath(c.ExtractSource)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(source)
	if err != nil || !st.Mode().IsRegular() {
		return "", errors.New("所选压缩包不存在或不是普通文件")
	}
	root, err := resolveOrdinaryPath(c.Root, true)
	if err != nil {
		return "", err
	}
	if !sameExtractPath(filepath.Dir(source), root) {
		return "", errors.New("所选压缩包必须位于输入目录中，不能跨目录处理")
	}
	return root, nil
}
