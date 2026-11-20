package toolbox

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
)

type DirectorySize struct {
	Size                        int64
	Files, Directories, Skipped int64
	Complete                    bool
	Note                        string
}

// BrowserDirectorySize reads metadata in bounded batches, never file contents.
// The original guest scope is retained; links and protected paths are excluded.
func BrowserDirectorySize(ctx context.Context, path string, scope *BrowserScope) (DirectorySize, error) {
	result := DirectorySize{Complete: true}
	// Compare physical paths consistently, including Windows 8.3 aliases and
	// macOS's temporary-directory aliases, without accepting directory links.
	var err error
	path, err = resolveOrdinaryPath(path, true)
	if err != nil {
		return result, err
	}
	if scope == nil {
		scope, err = NewBrowserScope(path)
		if err != nil {
			return result, err
		}
	}
	var visited int64
	var walk func(string, int) error
	walk = func(dir string, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if depth > 128 {
			result.Skipped++
			result.Complete = false
			return nil
		}
		rel, err := filepath.Rel(scope.Root, dir)
		if err != nil {
			return err
		}
		if _, err = scope.Resolve(rel, false); err != nil {
			return err
		}
		f, err := BrowseOpen(dir)
		if err != nil {
			return err
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil {
			return err
		}
		if !st.IsDir() {
			return errors.New("请选择文件夹计算大小")
		}
		for {
			entries, readErr := f.ReadDir(128)
			for _, entry := range entries {
				if err := ctx.Err(); err != nil {
					return err
				}
				visited++
				if visited > 2000000 {
					return errors.New("目录项目过多，已停止统计")
				}
				child := filepath.Join(dir, entry.Name())
				if entry.Type()&os.ModeSymlink != 0 || !BrowserReadableChild(child) {
					result.Skipped++
					continue
				}
				rel, err := filepath.Rel(scope.Root, child)
				if err == nil {
					_, err = scope.Resolve(rel, false)
				}
				if err != nil {
					result.Skipped++
					continue
				}
				if entry.IsDir() {
					result.Directories++
					if err = walk(child, depth+1); err != nil {
						if ctx.Err() != nil || visited > 2000000 {
							return err
						}
						result.Skipped++
						result.Complete = false
					}
				} else {
					info, err := entry.Info()
					if err != nil || !info.Mode().IsRegular() {
						result.Skipped++
						continue
					}
					result.Files++
					result.Size += info.Size()
				}
			}
			if readErr == io.EOF {
				return nil
			}
			if readErr != nil {
				return readErr
			}
		}
	}
	err = walk(path, 0)
	if err != nil {
		result.Complete = false
		if ctx.Err() != nil {
			result.Note = "统计超时或已取消，显示已读部分"
		} else {
			result.Note = "部分目录无法统计，显示已读部分"
		}
	}
	if result.Skipped > 0 {
		result.Complete = false
		if result.Note == "" {
			result.Note = "部分链接、受保护或不可读取的项目已跳过"
		}
	}
	// Descendant read failures produce a clearly marked partial result. Failure
	// to open the requested root, and explicit cancellation, remain errors.
	if err != nil && (visited == 0 || errors.Is(err, context.Canceled)) {
		return result, err
	}
	return result, nil
}
