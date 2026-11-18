//go:build !windows

package toolbox

import "errors"

func recycleExtractFiles(paths []string) (recycleResult, error) {
	return recycleResult{}, errors.New("当前系统暂不支持回收站清理，原文件已保留；不会改为永久删除")
}
