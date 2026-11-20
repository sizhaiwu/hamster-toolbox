package toolbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// BrowserVideoHover makes a short silent clip, never a full-video conversion.
// The decoder reads the verified open file and cannot follow media URLs.
func BrowserVideoHover(ctx context.Context, program string, input *os.File) ([]byte, error) {
	if _, err := input.Seek(0, 0); err != nil {
		return nil, err
	}
	args := append(browserSafeInputArgs(), "-i", "fd:", "-map", "0:v:0", "-an", "-sn", "-dn", "-t", "6", "-map_metadata", "-1", "-map_chapters", "-1", "-vf", "fps=12,scale=w='min(480,iw)':h='min(360,ih)':force_original_aspect_ratio=decrease:force_divisible_by=2", "-c:v", "libx264", "-preset", "veryfast", "-crf", "28", "-maxrate", "600k", "-bufsize", "1200k", "-pix_fmt", "yuv420p", "-threads", "1", "-g", "24", "-movflags", "frag_keyframe+empty_moov+default_base_moof", "-f", "mp4", "pipe:1")
	cmd := exec.CommandContext(ctx, program, args...)
	hideWindow(cmd)
	cmd.Stdin = input
	out, stderr := &thumbnailBuffer{limit: 2 << 20}, &thumbnailBuffer{limit: 4096}
	cmd.Stdout, cmd.Stderr = out, stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("无法生成悬停预览：%w %s", err, stderr.String())
	}
	if out.Len() < 32 || out.Len() >= out.limit {
		return nil, errors.New("悬停预览为空或过大，请点击打开视频")
	}
	return out.Bytes(), nil
}
