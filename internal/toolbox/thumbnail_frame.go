package toolbox

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"strconv"
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
func (r *thumbnailFrameExtractor) frame(ctx context.Context, frame, filter string, seek float64) (image.Image, error) {
	invoke := r.run
	if invoke == nil {
		invoke = run
	}
	var last error
	for _, attempt := range thumbnailFrameSeeks(seek, r.compatibility) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Never accept a previous frame, including a partial failed PNG.
		if err := os.Remove(frame); err != nil && !os.IsNotExist(err) {
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
		args = append(args, "-map", "0:V:0", "-an", "-sn", "-dn", "-frames:v", "1", "-vf", filter, "-threads", "1", "-f", "image2", "-update", "1", frame)
		_, err := invoke(ctx, r.program, args...)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err == nil {
			f, openErr := os.Open(frame)
			if openErr == nil {
				img, decodeErr := png.Decode(f)
				f.Close()
				if decodeErr == nil {
					return img, nil
				}
				last = fmt.Errorf("截取画面无法解码：%w", decodeErr)
			} else if !os.IsNotExist(openErr) {
				return nil, openErr
			} else {
				last = errors.New("目标时间点未输出视频画面")
			}
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
