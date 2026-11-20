package toolbox

import (
	"context"
	"errors"
	"os"
)

// Cleanup precedes every directory rename: a failed/cancelled cleanup must not
// write directory statistics that assume the remaining files were removed.
func executeFormat(ctx context.Context, p *Plan, log func(string), progress func(int, int)) Result {
	result := Result{}
	renames := []Item{}
	total, cleaned := len(p.Items), 0
	for _, it := range p.Items {
		if !isFormatCleanup(it) {
			renames = append(renames, it)
			continue
		}
		if ctx.Err() != nil {
			log("清理已取消，未执行后续目录改名")
			return result
		}
		e := checkItemSafety(p, it)
		if e == nil {
			var info os.FileInfo
			info, e = os.Lstat(it.Source)
			if e == nil && (!info.Mode().IsRegular() || info.Size() != it.Size || !info.ModTime().Equal(it.Modified)) {
				e = errors.New("文件已变化，请重新扫描预览")
			}
		}
		if e == nil && it.Conflict != "" {
			e = errors.New(it.Conflict)
		}
		if e == nil {
			if it.Action == "recycle" {
				_, e = recycleExtractFiles([]string{it.Source})
			} else {
				e = os.Remove(it.Source)
			}
		}
		if e != nil {
			result.Failed++
			log("清理失败：" + it.Source + " · " + e.Error() + "；停止后续清理和目录改名，请重新预览")
			progress(cleaned+1, total)
			return result
		}
		cleaned++
		result.Success++
		if it.Action == "recycle" {
			log("已移入回收站：" + it.Source)
		} else {
			log("已永久删除：" + it.Source)
		}
		progress(cleaned, total)
	}
	if ctx.Err() != nil {
		log("清理已取消，未执行后续目录改名")
		return result
	}
	e := validateFormatSnapshot(ctx, p)
	if e != nil {
		if ctx.Err() == nil {
			result.Failed++
			log("未执行目录改名：" + e.Error() + "，请重新扫描预览")
		}
		return result
	}
	remaining := *p
	remaining.Config.FormatCleanOther = false
	remaining.formatStats = nil // Already checked after cleanup, before any rename.
	remaining.Items = renames
	next := Execute(ctx, &remaining, log, func(done, _ int) { progress(cleaned+done, total) })
	result.Success += next.Success
	result.Failed += next.Failed
	result.Skipped += next.Skipped
	return result
}

func validateFormatSnapshot(ctx context.Context, p *Plan) error {
	actual, e := measureDirectoryTree(ctx, p.Config.Root)
	if e != nil {
		return e
	}
	if len(actual) != len(p.formatStats) {
		return errors.New("资源目录结构已变化")
	}
	for path, expected := range p.formatStats {
		if got, ok := actual[path]; !ok || got != expected {
			return errors.New("资源内容已变化")
		}
	}
	return nil
}
