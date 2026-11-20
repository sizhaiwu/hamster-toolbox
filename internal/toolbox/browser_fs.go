package toolbox

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func BrowserVideoThumbnail(ctx context.Context, program string, input *os.File) ([]byte, error) {
	return browserVideoFrame(ctx, program, input)
}

// BrowserScope pins a shared directory's identity. Remote requests always use
// relative paths inside this root; they cannot select arbitrary disk paths.
type BrowserScope struct {
	Root string
	id   fileIdentity
}

type BrowserItemRef struct {
	path     string
	id       fileIdentity
	size     int64
	modified time.Time
}

func CaptureBrowserItem(path string) (*BrowserItemRef, error) {
	p, e := resolveNonLinkPath(path)
	if e != nil {
		return nil, e
	}
	st, e := os.Stat(p)
	if e != nil {
		return nil, e
	}
	id, e := snapshotFileIdentity(p)
	if e != nil {
		return nil, e
	}
	return &BrowserItemRef{p, id, st.Size(), st.ModTime()}, nil
}
func (ref *BrowserItemRef) Check(path string) error {
	p, e := resolveNonLinkPath(path)
	if e != nil || !sameExtractPath(p, ref.path) {
		return errors.New("文件路径已变化，请刷新后重新选择")
	}
	st, e := os.Stat(p)
	if e != nil {
		return e
	}
	id, e := snapshotFileIdentity(p)
	if e != nil || !sameFileIdentity(id, ref.id) || st.Size() != ref.size || !st.ModTime().Equal(ref.modified) {
		return errors.New("文件已变化，请刷新后重新选择")
	}
	return nil
}

func NewBrowserScope(root string) (*BrowserScope, error) {
	p, e := resolveOrdinaryPath(root, true)
	if e != nil {
		return nil, e
	}
	if filepath.Dir(p) != p {
		if _, e := safePath(p); e != nil {
			return nil, e
		}
	}
	st, e := os.Stat(p)
	if e != nil || !st.IsDir() {
		return nil, errors.New("请选择存在的普通资源文件夹")
	}
	id, e := snapshotFileIdentity(p)
	return &BrowserScope{p, id}, e
}
func (s *BrowserScope) Resolve(relative string, write bool) (string, error) {
	if filepath.IsAbs(relative) || strings.ContainsAny(relative, ":\x00") {
		return "", errors.New("不允许绝对路径或特殊路径")
	}
	relative = filepath.Clean(strings.ReplaceAll(relative, "\\", string(filepath.Separator)))
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("不能访问共享目录之外")
	}
	id, e := snapshotFileIdentity(s.Root)
	if e != nil || !sameFileIdentity(id, s.id) {
		return "", errors.New("共享目录已变化，请在本机重新开启共享")
	}
	p := filepath.Join(s.Root, relative)
	if write && (relative == "." || relative == "") {
		return "", errors.New("不能修改共享根目录本身")
	}
	if write {
		p, e = safePath(p)
	} else {
		p, e = resolveOrdinaryPath(p, true)
		if e == nil && !sameExtractPath(p, s.Root) && protectedPathReasonOptions(p, false, filepath.Dir(s.Root) != s.Root) != "" {
			return "", errors.New("共享范围内的系统、配置或凭据目录不可访问")
		}
	}
	if e != nil || !pathContains(s.Root, p) {
		return "", errors.New("路径不可用或超出共享范围")
	}
	return p, nil
}

// BrowseOpen verifies the opened handle as well as the path. The file handle
// remains pinned if a directory is renamed while a download is in progress.
func BrowseOpen(path string) (*os.File, error) {
	p, e := resolveOrdinaryPath(path, true)
	if e != nil {
		return nil, e
	}
	f, e := os.Open(p)
	if e != nil {
		return nil, e
	}
	actual, e := browserHandlePath(f)
	if e != nil || !sameExtractPath(actual, p) {
		f.Close()
		return nil, errors.New("文件路径已变化，请刷新")
	}
	return f, nil
}
func BrowserName(name string) bool          { return validName(name) }
func BrowserReadableChild(path string) bool { return !pathRedirects(path) }

func BrowserMkdir(path string) error {
	p, e := safePath(path)
	if e != nil {
		return e
	}
	if !validName(filepath.Base(p)) {
		return errors.New("文件夹名称不合法")
	}
	return os.Mkdir(p, 0755)
}
func BrowserMove(source, target string) error {
	src, e := safePath(source)
	if e != nil {
		return e
	}
	dst, e := safePath(target)
	if e != nil {
		return e
	}
	if !validName(filepath.Base(dst)) || pathContains(src, dst) {
		return errors.New("目标名称不合法，或目标在源目录内部")
	}
	if !strings.EqualFold(filepath.VolumeName(src), filepath.VolumeName(dst)) {
		return errors.New("暂不支持跨磁盘移动，请先复制资源并核对")
	}
	if _, e = os.Lstat(dst); !os.IsNotExist(e) {
		return errors.New("目标已存在或不可访问，不会覆盖")
	}
	return browserMoveNoReplace(src, dst)
}
func BrowserUpload(ctx context.Context, path string, input io.Reader) error {
	p, e := safePath(path)
	if e != nil {
		return e
	}
	if !validName(filepath.Base(p)) {
		return errors.New("文件名不合法")
	}
	if _, e := os.Lstat(p); !os.IsNotExist(e) {
		return errors.New("目标已存在或不可访问，不会覆盖")
	}
	parent := filepath.Dir(p)
	f, e := os.CreateTemp(parent, ".toolbox-upload-*")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if actual, e := browserHandlePath(f); e != nil || !sameExtractPath(actual, tmp) {
		f.Close()
		return errors.New("上传目录已变化")
	}
	_, e = io.Copy(f, &contextReader{ctx: ctx, reader: input})
	if ce := f.Close(); e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	if checked, e := safePath(p); e != nil || !sameExtractPath(checked, p) {
		return errors.New("输出路径已变化")
	}
	return browserMoveNoReplace(tmp, p)
}
func BrowserDelete(ctx context.Context, path string, permanent bool) error {
	p, e := safePath(path)
	if e != nil {
		return e
	}
	type snapshot struct {
		path string
		id   fileIdentity
	}
	var items []snapshot
	e = filepath.WalkDir(p, func(current string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		checked, err := safePath(current)
		if err != nil {
			return err
		}
		if !pathContains(p, checked) || unsafeEntry(current, d) {
			return errors.New("目录含链接或受保护内容，停止删除")
		}
		id, err := snapshotFileIdentity(current)
		if err != nil {
			return err
		}
		items = append(items, snapshot{current, id})
		return nil
	})
	if e != nil {
		return e
	}
	check := func(it snapshot) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if _, e := safePath(it.path); e != nil {
			return e
		}
		id, e := snapshotFileIdentity(it.path)
		if e != nil || !sameFileIdentity(id, it.id) {
			return errors.New("文件已变化，停止删除")
		}
		return nil
	}
	if !permanent {
		for _, it := range items {
			if e := check(it); e != nil {
				return e
			}
		}
		return browserRecycle([]string{p})
	}
	for i := len(items) - 1; i >= 0; i-- {
		if e := check(items[i]); e != nil {
			return e
		}
		if e := os.Remove(items[i].path); e != nil {
			return e
		}
	}
	return nil
}
