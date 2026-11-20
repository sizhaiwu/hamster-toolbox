package toolbox

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func BrowserUploadPart(part, target string) (*os.File, error) {
	dst, err := safePath(target)
	if err != nil {
		return nil, err
	}
	tmp, err := safePath(part)
	if err != nil {
		return nil, err
	}
	if filepath.Dir(tmp) != filepath.Dir(dst) || !strings.HasPrefix(filepath.Base(tmp), ".toolbox-upload-") {
		return nil, errors.New("上传临时文件位置无效")
	}
	if st, e := os.Lstat(tmp); e == nil && !st.Mode().IsRegular() {
		return nil, errors.New("上传临时文件不可用")
	}
	f, err := os.OpenFile(tmp, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	actual, err := browserHandlePath(f)
	if err != nil || !sameExtractPath(actual, tmp) {
		f.Close()
		return nil, errors.New("上传目录已变化")
	}
	return f, nil
}
