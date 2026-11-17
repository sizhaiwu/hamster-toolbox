package toolbox

import (
	"context"
	"io/fs"
	"os"
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
	stats := map[string]directoryStats{}
	dirs := []string{}
	e := filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.Type()&os.ModeSymlink != 0 {
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
		v := stats[parent]
		v.Size += s.Size()
		switch {
		case has(p, Images+" .pdf"):
			v.Count[0]++
		case has(p, Videos):
			v.Count[1]++
		case has(p, Audios):
			v.Count[2]++
		}
		stats[parent] = v
		return nil
	})
	if e != nil {
		return nil, e
	}
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, dir := range dirs {
		if ctx.Err() != nil {
			return nil, ctx.Err()
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
	return stats, nil
}
