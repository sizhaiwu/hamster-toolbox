package toolbox

import (
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"os/exec"
)

// Read the pinned file's header, never a user-supplied pathname or delegate URL.
// The fallback is an explicit raster decoder with bounded output and resources.
func BrowserImageDimensions(ctx context.Context, program string, input *os.File) (width, height int, format string, err error) {
	if _, err = input.Seek(0, 0); err != nil {
		return
	}
	config, decoded, decodeErr := image.DecodeConfig(io.LimitReader(input, 16<<20))
	if decodeErr == nil && config.Width > 0 && config.Height > 0 {
		return config.Width, config.Height, decoded, nil
	}
	format = BrowserImageFormat(input)
	if format == "" || program == "" {
		return 0, 0, format, errors.New("无法读取图片尺寸，请检查图片或 ImageMagick 组件")
	}
	if _, err = input.Seek(0, 0); err != nil {
		return
	}
	args := []string{"-limit", "thread", "1", "-limit", "memory", "64MiB", "-limit", "map", "64MiB", "-limit", "disk", "0", "-limit", "width", "32768", "-limit", "height", "32768", "-limit", "time", "8", "-ping", format + ":-[0]", "-format", "%w %h", "info:"}
	cmd := exec.CommandContext(ctx, program, args...)
	hideWindow(cmd)
	cmd.Stdin = input
	out, stderr := &thumbnailBuffer{limit: 128}, &thumbnailBuffer{limit: 1024}
	cmd.Stdout, cmd.Stderr = out, stderr
	if err = cmd.Run(); err != nil {
		return 0, 0, format, errors.New("无法读取图片尺寸，请检查图片或 ImageMagick 组件")
	}
	if _, err = fmt.Sscanf(out.String(), "%d %d", &width, &height); err != nil || width < 1 || height < 1 || width > 32768 || height > 32768 {
		return 0, 0, format, errors.New("图片尺寸无效")
	}
	return
}
