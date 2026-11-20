package toolbox

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type BrowserTransferResult struct {
	Copied, Skipped int
	Backup          string
}

// Explicit conflict handling is separate from ordinary no-overwrite copying.
// Replaced originals are retained in a sibling backup directory. A move only
// removes each verified source file, and removes source folders if empty.
func BrowserMerge(ctx context.Context, source, target string, move bool, policy string, progress func(int64, int64)) (BrowserTransferResult, error) {
	result := BrowserTransferResult{}
	if policy != "overwrite" && policy != "skip" && policy != "rename" {
		return result, errors.New("请选择同名文件处理方式")
	}
	src, err := safePath(source)
	if err != nil {
		return result, err
	}
	dst, err := safePath(target)
	if err != nil {
		return result, err
	}
	if sameExtractPath(src, dst) || pathContains(src, dst) {
		return result, errors.New("目标不能是源目录或位于源目录内部")
	}
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return result, err
	}
	targetInfo, err := os.Lstat(dst)
	if os.IsNotExist(err) {
		err = BrowserCopy(ctx, src, dst, move, progress)
		return result, err
	}
	if err != nil {
		return result, err
	}
	if policy == "rename" {
		base, extension := filepath.Base(dst), ""
		if !sourceInfo.IsDir() {
			extension = filepath.Ext(base)
			base = base[:len(base)-len(extension)]
		}
		for i := 2; i < 10000; i++ {
			candidate := filepath.Join(filepath.Dir(dst), fmt.Sprintf("%s (%d)%s", base, i, extension))
			if _, err = os.Lstat(candidate); os.IsNotExist(err) {
				return result, BrowserCopy(ctx, src, candidate, move, progress)
			}
		}
		return result, errors.New("无法分配新的名称")
	}
	if !BrowserReadableChild(dst) || targetInfo.Mode()&os.ModeSymlink != 0 {
		return result, errors.New("目标是链接，不能合并或替换")
	}
	if sourceInfo.IsDir() != targetInfo.IsDir() {
		return result, errors.New("同名文件与文件夹类型不同，请选择保留两份")
	}
	if !sourceInfo.IsDir() && policy == "skip" {
		result.Skipped = 1
		return result, nil
	}
	var items []copySnapshot
	var total int64
	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if unsafeEntry(path, d) {
			return errors.New("源资源含链接或受保护内容，停止合并")
		}
		if _, err := safePath(path); err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return errors.New("不能合并设备或特殊文件")
		}
		identity, err := snapshotFileIdentity(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		out := dst
		if rel != "." {
			out = filepath.Join(dst, rel)
		}
		if pathContains(src, out) {
			return errors.New("合并目标会写入源目录内部，请先把处理结果移到原目录外")
		}
		items = append(items, copySnapshot{rel, info, identity})
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	// Pin existing destination directories. Recheck their identities before
	// every publication so a replaced directory cannot redirect later writes.
	directories := map[string]fileIdentity{}
	var done int64
	progress(0, total)
	for _, item := range items {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		in, out := src, dst
		if item.rel != "." {
			in = filepath.Join(src, item.rel)
			out = filepath.Join(dst, item.rel)
		}
		if _, err = safePath(out); err != nil {
			return result, err
		}
		for dir := filepath.Dir(out); pathContains(dst, dir); dir = filepath.Dir(dir) {
			if id, ok := directories[dir]; ok {
				current, e := snapshotFileIdentity(dir)
				if e != nil || !sameFileIdentity(current, id) {
					return result, errors.New("目标目录已变化，停止合并")
				}
			}
			if sameExtractPath(dir, dst) {
				break
			}
		}
		if item.info.IsDir() {
			if err = os.Mkdir(out, item.info.Mode().Perm()); err != nil && !os.IsExist(err) {
				return result, err
			}
			st, e := os.Lstat(out)
			if e != nil || !st.IsDir() || !BrowserReadableChild(out) {
				return result, errors.New("目标目录存在类型冲突或链接")
			}
			directories[out], err = snapshotFileIdentity(out)
			if err != nil {
				return result, err
			}
			continue
		}
		current, e := os.Stat(in)
		identity, identityErr := snapshotFileIdentity(in)
		if e != nil || identityErr != nil || !sameFileIdentity(identity, item.identity) || current.Size() != item.info.Size() || !current.ModTime().Equal(item.info.ModTime()) {
			return result, errors.New("源文件已变化，停止合并")
		}
		var existing *BrowserItemRef
		if _, e = os.Lstat(out); e == nil {
			st, e := os.Lstat(out)
			if e != nil || !st.Mode().IsRegular() || !BrowserReadableChild(out) {
				return result, errors.New("同名目标不是普通文件，请选择保留两份")
			}
			if policy == "skip" {
				result.Skipped++
				done += item.info.Size()
				progress(done, total)
				continue
			}
			existing, e = CaptureBrowserItem(out)
			if e != nil {
				return result, e
			}
		} else if !os.IsNotExist(e) {
			return result, e
		}
		// Copy and hash-verify first. No existing destination is touched until
		// the replacement is ready, and no source is removed before publication.
		stage, e := os.MkdirTemp(filepath.Dir(out), ".toolbox-merge-")
		if e != nil {
			return result, e
		}
		staged := filepath.Join(stage, "verified")
		err = BrowserCopy(ctx, in, staged, false, func(n, _ int64) { progress(done+n, total) })
		if err != nil {
			os.RemoveAll(stage)
			return result, err
		}
		backup := ""
		if existing != nil {
			if err = existing.Check(out); err == nil {
				_, err = safePath(out)
			}
			if err == nil && result.Backup == "" {
				result.Backup, err = os.MkdirTemp(filepath.Dir(dst), ".toolbox-replaced-")
			}
			if err == nil {
				backup = filepath.Join(result.Backup, item.rel)
				if item.rel == "." {
					backup = filepath.Join(result.Backup, filepath.Base(dst))
				}
				err = os.MkdirAll(filepath.Dir(backup), 0700)
			}
			if err == nil {
				err = browserMoveNoReplace(out, backup)
			}
		}
		if err == nil {
			err = ctx.Err()
		}
		if err == nil {
			_, err = safePath(out)
		}
		if err == nil {
			err = browserMoveNoReplace(staged, out)
		}
		if err != nil && backup != "" {
			if _, backupErr := os.Lstat(backup); backupErr == nil {
				if restoreErr := browserMoveNoReplace(backup, out); restoreErr != nil {
					err = fmt.Errorf("替换失败，原文件保留在备份目录：%w", err)
				}
			}
		}
		os.RemoveAll(stage)
		if err != nil {
			return result, err
		}
		result.Copied++
		done += item.info.Size()
		progress(done, total)
		if move {
			current, e = os.Stat(in)
			identity, identityErr = snapshotFileIdentity(in)
			if e != nil || identityErr != nil || !sameFileIdentity(identity, item.identity) || current.Size() != item.info.Size() || !current.ModTime().Equal(item.info.ModTime()) {
				return result, errors.New("副本已发布，源文件发生变化未删除")
			}
			if err = BrowserDelete(ctx, in, true); err != nil {
				return result, err
			}
		}
	}
	if move && sourceInfo.IsDir() {
		for i := len(items) - 1; i >= 0; i-- {
			item := items[i]
			if !item.info.IsDir() {
				continue
			}
			p := src
			if item.rel != "." {
				p = filepath.Join(src, item.rel)
			}
			id, e := snapshotFileIdentity(p)
			if e == nil && sameFileIdentity(id, item.identity) {
				os.Remove(p)
			}
		}
	}
	return result, nil
}
