package toolbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type recycleResult struct {
	Count int
	Paths []string
}

var errExtractDeleteSource = errors.New("解压已完成，原压缩包清理未全部完成")

func extractVolumeCount(header string) int {
	// A byte-split archive can contain another archive header. Only the outer
	// layer identifies the physical input files. Archive comments are arbitrary
	// text and must never be interpreted as volume properties.
	header = strings.SplitN(header, "\n----\n", 2)[0]
	count, seen := 1, false
	for _, line := range strings.Split(header, "\n") {
		if strings.HasPrefix(line, "Comment =") {
			if seen {
				return count
			}
			return 0
		}
		if strings.HasPrefix(line, "Volumes = ") {
			if n, e := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Volumes = "))); e == nil && n > 0 {
				count = n
				seen = true
			}
		}
	}
	return count
}

func sameExtractPath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// Delete only the snapshot of files actually consumed by 7-Zip. A regular ZIP
// can have unrelated/stale .z01 files beside it; those must never be deleted.
func deleteExtractSources(ctx context.Context, c Config, it Item, job extractJob, log func(string)) error {
	if e := checkExtractVolumes(ctx, it, job); e != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w：%s", errExtractDeleteSource, e)
	}
	var volumes []archiveEntry
	if job.VolumeCount == 1 || (job.VolumeCount == 0 && len(job.Volumes) == 1) {
		for _, v := range job.Volumes {
			if sameExtractPath(v.Source, it.Source) {
				volumes = append(volumes, v)
				break
			}
		}
	} else if job.VolumeCount > 1 && job.VolumeCount == len(job.Volumes) {
		volumes = job.Volumes
	}
	if len(volumes) == 0 {
		return fmt.Errorf("%w：无法安全确认原包及实际分卷，请手动检查", errExtractDeleteSource)
	}
	root, e := filepath.Abs(c.Root)
	if e != nil {
		return e
	}
	source, e := filepath.Abs(it.Source)
	if e != nil {
		return e
	}
	// Validate the complete set before removing anything; never remove folders,
	// links, output files, or a source that has changed since scanning.
	for _, v := range volumes {
		path, e := filepath.Abs(v.Source)
		if e != nil {
			return e
		}
		if _, e := safePath(path); e != nil {
			return fmt.Errorf("%w：%s", errExtractDeleteSource, e)
		}
		if !within(root, path) || !sameExtractPath(filepath.Dir(path), filepath.Dir(source)) || (c.Output != "" && within(c.Output, path)) {
			return fmt.Errorf("%w：原包路径不在本次允许清理的范围", errExtractDeleteSource)
		}
		st, e := os.Lstat(path)
		if e != nil || !st.Mode().IsRegular() || st.Size() != v.Size || !st.ModTime().Equal(v.Modified) {
			return fmt.Errorf("%w：原包文件已变化，请重新扫描", errExtractDeleteSource)
		}
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if c.ExtractDeleteMode == "" || c.ExtractDeleteMode == "recycle" {
		paths := make([]string, 0, len(volumes))
		for _, v := range volumes {
			paths = append(paths, v.Source)
		}
		result, e := recycleExtractFiles(paths)
		if e != nil {
			return fmt.Errorf("%w：已移入回收站 %d/%d 个文件；%s", errExtractDeleteSource, result.Count, len(paths), e)
		}
		log(fmt.Sprintf("已移入回收站：%s（共 %d 个文件/分卷）", it.Source, result.Count))
		return nil
	}
	if c.ExtractDeleteMode != "permanent" {
		return fmt.Errorf("%w：不支持的删除方式", errExtractDeleteSource)
	}
	// Once cleanup starts, finish this small file-only group even if cancellation
	// arrives. Cancellation before this commit point retains the entire group.
	for i, v := range volumes {
		if e := os.Remove(v.Source); e != nil {
			return fmt.Errorf("%w：已删除 %d/%d 个文件，删除 %s 失败：%s", errExtractDeleteSource, i, len(volumes), filepath.Base(v.Source), e)
		}
	}
	log(fmt.Sprintf("已永久删除原压缩包：%s（共 %d 个文件/分卷）", it.Source, len(volumes)))
	return nil
}
