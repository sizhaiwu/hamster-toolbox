package toolbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Result struct{ Success, Failed, Skipped int }

var errNotSmaller = errors.New("压缩结果未变小")
var errImageSkipped = errors.New("图片已跳过")

func Execute(ctx context.Context, p *Plan, log func(string), progress func(int, int)) Result {
	r := Result{}
	total := len(p.Items)
	if p.Config.Tool == "pdf" {
		if e := makePDF(ctx, p, log, progress); e != nil {
			if ctx.Err() != nil {
				log("PDF 生成已取消")
				return r
			}
			log("失败：" + e.Error())
			r.Failed = total
		} else {
			r.Success = total
			log("PDF 已生成：" + p.Config.Output)
		}
		return r
	}
	for i, it := range p.Items {
		if ctx.Err() != nil {
			log("任务已取消")
			break
		}
		if it.Conflict != "" {
			r.Skipped++
			log("跳过：" + it.Source + " · " + it.Conflict)
			progress(i+1, total)
			continue
		}
		s, e := os.Stat(it.Source)
		if e == nil && !s.IsDir() && (s.Size() != it.Size || !s.ModTime().Equal(it.Modified)) {
			e = errors.New("文件已变化，请重新预览")
		}
		if e == nil {
			if _, te := os.Lstat(it.Target); te == nil {
				e = errors.New("目标已存在")
			} else if !os.IsNotExist(te) {
				e = te
			}
		}
		if e == nil {
			if p.Config.Tool == "rename" || p.Config.Tool == "format" || p.Config.Tool == "describe" || p.Config.Tool == "number" {
				e = os.Rename(it.Source, it.Target)
			} else {
				if p.Config.Tool == "thumbnail" {
					log("正在生成缩略图：" + it.Source)
				}
				e = convert(ctx, p.Config, it)
			}
		}
		if e != nil {
			if ctx.Err() != nil {
				log("当前处理已取消：" + it.Source)
				break
			}
			if errors.Is(e, errNotSmaller) || errors.Is(e, errImageSkipped) {
				r.Skipped++
				log("跳过：" + it.Source + " · " + e.Error())
			} else {
				r.Failed++
				log("失败：" + it.Source + " · " + e.Error())
			}
		} else {
			r.Success++
			message := "完成：" + it.Target
			if (p.Config.Tool == "image" || p.Config.Tool == "video") && it.Size > 0 {
				if out, e := os.Stat(it.Target); e == nil {
					message += fmt.Sprintf("（%s → %s，减少 %.1f%%）", Human(it.Size), Human(out.Size()), 100*(1-float64(out.Size())/float64(it.Size)))
				}
			}
			log(message)
		}
		progress(i+1, total)
	}
	return r
}

// Reserve output with O_EXCL and publish only after successful conversion.
func publish(tmp, target string) error {
	return publishContext(context.Background(), tmp, target)
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	return r.reader.Read(p)
}

func publishContext(ctx context.Context, tmp, target string) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	in, e := os.Open(tmp)
	if e != nil {
		return e
	}
	defer in.Close()
	return publishReader(ctx, in, target)
}

func publishReader(ctx context.Context, in io.Reader, target string) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	out, e := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if e != nil {
		return e
	}
	_, e = io.Copy(out, contextReader{ctx, in})
	if e == nil {
		e = ctx.Err()
	}
	ce := out.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		os.Remove(target)
	}
	return e
}
func convert(ctx context.Context, c Config, it Item) error {
	if e := os.MkdirAll(filepath.Dir(it.Target), 0755); e != nil {
		return e
	}
	dir, e := os.MkdirTemp(filepath.Dir(it.Target), ".toolbox-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(dir)
	tmp := filepath.Join(dir, "result"+filepath.Ext(it.Target))
	switch c.Tool {
	case "thumbnail":
		e = makeThumbnail(ctx, c, it, tmp)
	case "image":
		e = compressImage(ctx, c, it.Source, tmp)
	case "exif":
		// Copy first, then remove metadata; ExifTool preserves image pixel data.
		e = publishContext(ctx, it.Source, tmp)
		if e == nil {
			_, e = run(ctx, c.Exiftool, "-all=", "-overwrite_original", tmp)
		}
	case "video":
		_, e = run(ctx, c.FFmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-n", "-i", it.Source, "-map", "0:v:0", "-map", "0:a?", "-c:v", "libx264", "-crf", strconv.Itoa(c.CRF), "-preset", "medium", "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "128k", "-movflags", "+faststart", tmp)
	}
	if e != nil {
		return e
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	s, e := os.Stat(tmp)
	if e != nil {
		return e
	}
	if s.Size() == 0 {
		return errors.New("输出为空")
	}
	if (c.Tool == "image" || c.Tool == "video") && s.Size() >= it.Size {
		return fmt.Errorf("%w（%s → %s），保留原文件", errNotSmaller, Human(it.Size), Human(s.Size()))
	}
	if c.Tool == "video" {
		if _, e = probe(ctx, c.FFprobe, tmp); e != nil {
			return e
		}
	}
	return publishContext(ctx, tmp, it.Target)
}
func CheckReady(p *Plan) error {
	if p == nil {
		return errors.New("请先预览")
	}
	if len(p.Items) == 0 {
		return errors.New("没有可执行项目")
	}
	if p.Config.Tool == "pdf" {
		for _, it := range p.Items {
			if it.Conflict != "" {
				return errors.New(it.Conflict)
			}
		}
	}
	for _, it := range p.Items {
		if it.Conflict == "" {
			return nil
		}
	}
	return errors.New("所有项目均有冲突，请调整设置后重新预览")
}
func CleanText(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.ReplaceAll(s, "\n", " ")
}
