package toolbox

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

// FileOutcome describes a completed independent file operation. The callback
// can run concurrently when workers are enabled; it must return promptly.
type FileOutcome struct {
	Source, Target, Status, Message string
}

type executionObserverKey struct{}
type remainingScanKey struct{}
type remainingScanFilter struct{ sources, directories map[string]bool }

func ExecuteObserved(ctx context.Context, p *Plan, log func(string), progress func(int, int), observe func(FileOutcome)) Result {
	return Execute(context.WithValue(ctx, executionObserverKey{}, observe), p, log, progress)
}

func observeFile(ctx context.Context, it Item, status, message string) {
	if callback, ok := ctx.Value(executionObserverKey{}).(func(FileOutcome)); ok && callback != nil {
		callback(FileOutcome{Source: it.Source, Target: it.Target, Status: status, Message: message})
	}
}

func IndependentFileTask(tool string) bool {
	return tool == "image" || tool == "video" || tool == "exif" || tool == "thumbnail"
}

func sourceKey(source string) string {
	source = filepath.Clean(source)
	if runtime.GOOS == "windows" {
		return strings.ToLower(source)
	}
	return source
}

// Rebuild the plan from current files and settings, never deserialize an old
// executable plan. Existing output and path identity checks stay in effect.
func BuildRemainingPlan(ctx context.Context, c Config, sources []string, log func(string)) (*Plan, error) {
	if !IndependentFileTask(c.Tool) {
		return nil, errors.New("此工具需要重新核对完整目录，请载入设置后重新预览")
	}
	if len(sources) == 0 {
		return nil, errors.New("没有失败或未完成的文件")
	}
	wanted := make(map[string]bool, len(sources))
	directories := map[string]bool{}
	root, err := filepath.Abs(c.Root)
	if err != nil {
		return nil, err
	}
	for _, source := range sources {
		wanted[sourceKey(source)] = true
		for parent := filepath.Dir(source); within(root, parent); parent = filepath.Dir(parent) {
			directories[sourceKey(parent)] = true
			if sourceKey(parent) == sourceKey(root) {
				break
			}
		}
	}
	ctx = context.WithValue(ctx, remainingScanKey{}, remainingScanFilter{sources: wanted, directories: directories})
	p, err := BuildPlan(ctx, c, log)
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, min(len(p.Items), len(wanted)))
	for _, it := range p.Items {
		if wanted[sourceKey(it.Source)] {
			items = append(items, it)
			delete(wanted, sourceKey(it.Source))
		}
	}
	p.Items = items
	if len(wanted) > 0 {
		log("部分历史文件未出现在当前扫描中，可能已移动、删除或被过滤；请核对预览")
	}
	log(fmt.Sprintf("本次重扫预览：%d 项；成功及已跳过项不重复处理", len(items)))
	return p, nil
}
