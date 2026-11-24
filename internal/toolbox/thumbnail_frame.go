package toolbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type thumbnailFrameExtractor struct {
	program, source string
	compatibility   bool
	log             func(string)
	run             func(context.Context, string, ...string) ([]byte, error)
}
type thumbnailFrameSeek struct {
	anchor, offset float64
	fast           bool
}

func thumbnailFrameSeeks(seek float64, compatible bool) []thumbnailFrameSeek {
	var attempts []thumbnailFrameSeek
	if !compatible {
		attempts = append(attempts, thumbnailFrameSeek{anchor: seek, fast: true})
	}
	// Decode a short section before the requested timestamp when an unindexed
	// transport stream cannot seek directly. The output seek keeps the timestamp.
	anchor := math.Max(0, seek-15)
	attempts = append(attempts, thumbnailFrameSeek{anchor: anchor, offset: seek - anchor})
	if anchor > 0 {
		attempts = append(attempts, thumbnailFrameSeek{offset: seek})
	}
	return attempts
}

// Bound the pipe output even if a configured executable does not honor the
// single-frame filter. No temporary frame file is opened, reused or deleted.
func runThumbnailFrame(ctx context.Context, program string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, program, args...)
	hideWindow(cmd)
	out, stderr := &thumbnailBuffer{limit: 80 << 20}, &thumbnailBuffer{limit: 16 << 10}
	cmd.Stdout, cmd.Stderr = out, stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s: %w：%s", filepath.Base(program), err, strings.TrimSpace(stderr.String()))
	}
	if out.Len() >= out.limit {
		return nil, errors.New("截帧数据超过安全大小限制")
	}
	return out.Bytes(), nil
}

func (r *thumbnailFrameExtractor) frame(ctx context.Context, filter string, seek float64) (image.Image, error) {
	invoke := r.run
	if invoke == nil {
		invoke = runThumbnailFrame
	}
	var last error
	for _, attempt := range thumbnailFrameSeeks(seek, r.compatibility) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y"}
		if attempt.fast || attempt.anchor > 0 {
			args = append(args, "-ss", strconv.FormatFloat(attempt.anchor, 'f', 6, 64))
		}
		args = append(args, "-threads", "2", "-i", r.source)
		if attempt.offset > 0 {
			args = append(args, "-ss", strconv.FormatFloat(attempt.offset, 'f', 6, 64))
		}
		args = append(args, "-map", "0:V:0", "-an", "-sn", "-dn", "-frames:v", "1", "-vf", filter, "-threads", "1", "-c:v", "png", "-pix_fmt", "rgba", "-f", "image2pipe", "pipe:1")
		data, err := invoke(ctx, r.program, args...)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err == nil && len(data) == 0 {
			last = errors.New("目标时间点未输出视频画面")
		} else if err == nil {
			cfg, decodeErr := png.DecodeConfig(bytes.NewReader(data))
			if decodeErr == nil && (cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 4096 || cfg.Height > 4096) {
				decodeErr = errors.New("截帧尺寸超过安全限制")
			}
			if decodeErr == nil {
				var img image.Image
				img, decodeErr = png.Decode(bytes.NewReader(data))
				if decodeErr == nil {
					return img, nil
				}
			}
			last = fmt.Errorf("截取画面无法解码：%w", decodeErr)
		} else {
			last = err
		}
		if attempt.fast && !r.compatibility {
			r.compatibility = true
			if r.log != nil {
				r.log("快速定位未取得画面，自动切换兼容定位（保持原截取时间）")
			}
		}
	}
	return nil, fmt.Errorf("%s 处截帧失败，兼容定位仍未取得画面，请检查视频是否损坏：%w", thumbnailTimestamp(seek), last)
}
