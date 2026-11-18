package toolbox

import (
	"context"
	"io/fs"
	"path/filepath"
	"sort"
)

type directoryStats struct {
	Count [3]int
	Size  int64
}

// Read each entry once, then aggregate children into parents. Nested folders
// no longer trigger a fresh recursive scan for every directory being renamed.
func measureDirectoryTree(ctx context.Context, root string) (map[string]directoryStats, error) {
	stats, _, e := measureDirectoryTreeForFormat(ctx, root, false, "recycle")
	return stats, e
}

func measureDirectoryTreeForFormat(ctx context.Context, root string, clean bool, mode string) (map[string]directoryStats, []Item, error) {
	stats := map[string]directoryStats{}
	cleanup := []Item{}
	dirs := []string{}
	e := filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if unsafeEntry(p, d) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if p != root && excluded(d.Name()) {
				return filepath.SkipDir
			}
			stats[p] = directoryStats{}
			dirs = append(dirs, p)
			return nil
		}
		s, e := d.Info()
		if e != nil {
			return e
		}
		parent := filepath.Dir(p)
		kind := formatMediaKind(p)
		if clean && parent != root && kind < 0 && s.Mode().IsRegular() {
			action, note := "recycle", "非图片、视频、音频文件 · 移入回收站"
			if mode == "permanent" {
				action, note = "delete", "非图片、视频、音频文件 · 永久删除（不进回收站）"
			}
			cleanup = append(cleanup, Item{Action: action, Source: p, Target: p, Note: note, Size: s.Size(), Modified: s.ModTime()})
			return nil
		}
		v := stats[parent]
		v.Size += s.Size()
		if kind >= 0 {
			v.Count[kind]++
		}
		stats[parent] = v
		return nil
	})
	if e != nil {
		return nil, nil, e
	}
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, dir := range dirs {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		if dir == root {
			continue
		}
		v := stats[dir]
		parent := filepath.Dir(dir)
		pv := stats[parent]
		pv.Size += v.Size
		for i, n := range v.Count {
			pv.Count[i] += n
		}
		stats[parent] = pv
	}
	return stats, cleanup, nil
}
