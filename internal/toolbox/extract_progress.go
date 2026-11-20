package toolbox

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// 7-Zip rewrites progress using CR/backspace, not necessarily newline. Keep
// only a bounded numeric prefix; filenames must never become progress events.
type sevenZipProgressWriter struct {
	notify  func(int)
	prefix  [16]byte
	used    int
	ignored bool
}

func (w *sevenZipProgressWriter) Write(data []byte) (int, error) {
	for _, b := range data {
		if b == '\r' || b == '\n' || b == '\b' {
			w.used = 0
			w.ignored = false
			continue
		}
		if w.ignored {
			continue
		}
		if b == '%' {
			n, err := strconv.Atoi(strings.TrimSpace(string(w.prefix[:w.used])))
			if err == nil && n >= 0 && n <= 100 && w.notify != nil {
				w.notify(n)
			}
			w.ignored = true
			continue
		}
		if (b < '0' || b > '9') && b != ' ' && b != '\t' || w.used == len(w.prefix) {
			w.ignored = true
			continue
		}
		w.prefix[w.used] = b
		w.used++
	}
	return len(data), nil
}

type extractPositionKey struct{}
type extractPosition struct{ number, total int }
type extractProgressReporter struct {
	ctx               context.Context
	value             OperationProgress
	lastEmit, lastLog time.Time
	log               func(string)
}

func newExtractProgress(ctx context.Context, source string, log func(string)) *extractProgressReporter {
	position, _ := ctx.Value(extractPositionKey{}).(extractPosition)
	return &extractProgressReporter{ctx: ctx, log: log, value: OperationProgress{Tool: "extract", Source: source, ItemNumber: position.number, ItemTotal: position.total, Started: time.Now().UTC()}}
}
func (p *extractProgressReporter) phase(name string, files int, bytes int64) {
	p.value.Phase = name
	p.value.PhaseStarted = time.Now().UTC()
	p.value.Percent = 0
	p.value.PercentKnown = false
	p.value.FilesDone = 0
	p.value.FilesTotal = files
	p.value.BytesDone = 0
	p.value.BytesTotal = bytes
	p.emit(true)
}
func (p *extractProgressReporter) percent(n int) {
	percent := math.Min(99.9, float64(n))
	if p.value.PercentKnown && percent < p.value.Percent {
		return
	}
	force := !p.value.PercentKnown || (n == 100 && p.value.Percent < 99.9)
	p.value.Percent = percent
	p.value.PercentKnown = true
	p.emit(force)
}
func (p *extractProgressReporter) copyBytes(n int64) {
	p.value.BytesDone += n
	p.writingPercent()
	p.emit(false)
}
func (p *extractProgressReporter) writingPercent() {
	p.value.PercentKnown = p.value.BytesTotal > 0 || p.value.FilesTotal > 0
	if p.value.BytesTotal > 0 {
		p.value.Percent = math.Min(99.9, 100*float64(p.value.BytesDone)/float64(p.value.BytesTotal))
	} else if p.value.FilesTotal > 0 {
		p.value.Percent = math.Min(99.9, 100*float64(p.value.FilesDone)/float64(p.value.FilesTotal))
	}
}
func (p *extractProgressReporter) emit(force bool) {
	now := time.Now()
	if !force && now.Sub(p.lastEmit) < time.Second {
		return
	}
	p.lastEmit = now
	reportOperationProgress(p.ctx, p.value)
	if !force && now.Sub(p.lastLog) < 10*time.Second {
		return
	}
	p.lastLog = now
	labels := map[string]string{"extract": "正在解压", "validate": "检查解压结果", "publish": "写入输出文件", "cleanup": "清理原压缩包", "complete": "本包处理完成"}
	message := "解压进度：" + labels[p.value.Phase]
	if p.value.PercentKnown {
		message += fmt.Sprintf(" %.1f%%", p.value.Percent)
	}
	if p.value.Phase == "publish" {
		message += fmt.Sprintf(" · 已处理 %d/%d 个文件", p.value.FilesDone, p.value.FilesTotal)
	}
	message += " · 已用 " + time.Since(p.value.Started).Round(time.Second).String()
	if p.log != nil {
		p.log(message)
	}
}
func addExtractBytes(total, size int64) int64 {
	if size > 0 && total <= math.MaxInt64-size {
		return total + size
	}
	return total
}
