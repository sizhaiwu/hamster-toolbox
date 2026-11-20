package toolbox

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

type copySnapshot struct {
	rel      string
	info     os.FileInfo
	identity fileIdentity
}

// Publish a complete, verified copy before removing any source of a cross-volume move.
func BrowserCopy(ctx context.Context, source, target string, move bool, progress func(int64, int64)) error {
	src, err := safePath(source)
	if err != nil {
		return err
	}
	dst, err := safePath(target)
	if err != nil {
		return err
	}
	if pathContains(src, dst) || !validName(filepath.Base(dst)) {
		return errors.New("目标不能位于源目录中")
	}
	if _, err = os.Lstat(dst); !os.IsNotExist(err) {
		return errors.New("目标已存在，不会覆盖")
	}
	var snapshots []copySnapshot
	var total int64
	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		if unsafeEntry(path, d) {
			return errors.New("资源含链接或受保护内容，停止复制")
		}
		if _, e = safePath(path); e != nil {
			return e
		}
		st, e := d.Info()
		if e != nil {
			return e
		}
		if !st.IsDir() && !st.Mode().IsRegular() {
			return errors.New("不支持复制设备或特殊文件")
		}
		id, e := snapshotFileIdentity(path)
		if e != nil {
			return e
		}
		rel, _ := filepath.Rel(src, path)
		snapshots = append(snapshots, copySnapshot{rel, st, id})
		if st.Mode().IsRegular() {
			total += st.Size()
		}
		return nil
	})
	if err != nil {
		return err
	}
	parent, err := os.MkdirTemp(filepath.Dir(dst), ".toolbox-copy-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(parent)
	tmp := filepath.Join(parent, "resource")
	var done int64
	progress(0, total)
	for _, it := range snapshots {
		if err = ctx.Err(); err != nil {
			return err
		}
		out := tmp
		if it.rel != "." {
			out = filepath.Join(tmp, it.rel)
		}
		if it.info.IsDir() {
			if err = os.MkdirAll(out, 0755); err != nil {
				return err
			}
			continue
		}
		inPath := src
		if it.rel != "." {
			inPath = filepath.Join(src, it.rel)
		}
		in, e := BrowseOpen(inPath)
		if e != nil {
			return e
		}
		outFile, e := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			in.Close()
			return e
		}
		hash := sha256.New()
		buffer := make([]byte, 1024*1024)
		for {
			if e = ctx.Err(); e != nil {
				break
			}
			var n int
			n, e = in.Read(buffer)
			if n > 0 {
				if _, we := outFile.Write(buffer[:n]); we != nil {
					e = we
					break
				}
				hash.Write(buffer[:n])
				done += int64(n)
				progress(done, total)
			}
			if e == io.EOF {
				e = nil
				break
			}
			if e != nil {
				break
			}
		}
		if e == nil {
			e = outFile.Sync()
		}
		in.Close()
		closeErr := outFile.Close()
		if e == nil {
			e = closeErr
		}
		if e != nil {
			return e
		}
		st, e := os.Stat(inPath)
		id, ie := snapshotFileIdentity(inPath)
		if e != nil || ie != nil || !sameFileIdentity(id, it.identity) || st.Size() != it.info.Size() || !st.ModTime().Equal(it.info.ModTime()) {
			return errors.New("源文件已变化，未发布副本")
		}
		verify, e := os.Open(out)
		if e != nil {
			return e
		}
		check := sha256.New()
		_, e = io.Copy(check, contextReader{ctx, verify})
		verify.Close()
		if e != nil {
			return e
		}
		if string(check.Sum(nil)) != string(hash.Sum(nil)) {
			return errors.New("复制内容校验失败，保留源文件")
		}
		if e = os.Chmod(out, it.info.Mode().Perm()); e != nil {
			return e
		}
		if e = os.Chtimes(out, time.Now(), it.info.ModTime()); e != nil {
			return e
		}
	}
	// Check the complete source tree again, including newly added or removed entries.
	index := 0
	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if index >= len(snapshots) {
			return errors.New("源目录已变化")
		}
		it := snapshots[index]
		index++
		rel, _ := filepath.Rel(src, path)
		st, e := d.Info()
		if e != nil {
			return e
		}
		id, e := snapshotFileIdentity(path)
		if e != nil || rel != it.rel || !sameFileIdentity(id, it.identity) || !st.ModTime().Equal(it.info.ModTime()) || st.Size() != it.info.Size() {
			return errors.New("源目录内容已变化")
		}
		return ctx.Err()
	})
	if err != nil || index != len(snapshots) {
		return fmt.Errorf("源目录已变化，保留源文件：%v", err)
	}
	for i := len(snapshots) - 1; i >= 0; i-- {
		it := snapshots[i]
		if it.info.IsDir() {
			out := tmp
			if it.rel != "." {
				out = filepath.Join(tmp, it.rel)
			}
			os.Chtimes(out, time.Now(), it.info.ModTime())
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if _, err = safePath(dst); err != nil {
		return err
	}
	if err = browserMoveNoReplace(tmp, dst); err != nil {
		return err
	}
	if move {
		if err = BrowserDelete(ctx, src, true); err != nil {
			return fmt.Errorf("副本已保存，源文件未全部移除，请核对：%w", err)
		}
	}
	progress(total, total)
	return nil
}
