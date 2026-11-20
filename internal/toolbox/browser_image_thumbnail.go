package toolbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/jpeg"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// BrowserImageThumbnail uses a pinned file as stdin and an explicit decoder.
// JPEG decoder scaling avoids allocating a full-size image for large photos.
// No user filename, delegate format or URL is passed to ImageMagick.
func BrowserImageThumbnail(ctx context.Context, program string, input *os.File, format string) ([]byte, error) {
	return browserImageRaster(ctx, program, input, format, 640, 480, 78)
}
func BrowserImagePreview(ctx context.Context, program string, input *os.File, format string) ([]byte, error) {
	return browserImageRaster(ctx, program, input, format, 2560, 2560, 90)
}
func BrowserImageFormat(input *os.File) string {
	var head [512]byte
	n, _ := input.ReadAt(head[:], 0)
	data := head[:n]
	if format := map[string]string{"image/jpeg": "JPEG", "image/png": "PNG", "image/webp": "WEBP", "image/gif": "GIF", "image/bmp": "BMP", "image/tiff": "TIFF"}[http.DetectContentType(data)]; format != "" {
		return format
	}
	if bytes.HasPrefix(data, []byte("II*\x00")) || bytes.HasPrefix(data, []byte("MM\x00*")) {
		return "TIFF"
	}
	if len(data) >= 16 && string(data[4:8]) == "ftyp" {
		for offset := 8; offset+4 <= len(data) && offset < 64; offset += 4 {
			brand := string(data[offset : offset+4])
			if brand == "avif" || brand == "avis" {
				return "AVIF"
			}
		}
		for offset := 8; offset+4 <= len(data) && offset < 64; offset += 4 {
			switch string(data[offset : offset+4]) {
			case "heic", "heix", "hevc", "hevx", "mif1", "msf1":
				return "HEIC"
			}
		}
	}
	return ""
}
func browserImageRaster(ctx context.Context, program string, input *os.File, format string, maxWidth, maxHeight, quality int) ([]byte, error) {
	if program == "" {
		return nil, errors.New("生成大图缩略图需要 ImageMagick")
	}
	switch format {
	case "JPEG", "PNG", "WEBP", "GIF", "BMP", "TIFF", "AVIF", "HEIC":
	default:
		return nil, errors.New("此格式不使用外部缩略图解码器")
	}
	if _, err := input.Seek(0, 0); err != nil {
		return nil, err
	}
	memory := "256MiB"
	if maxWidth > 640 {
		memory = "384MiB"
	}
	args := []string{"-limit", "thread", "1", "-limit", "memory", memory, "-limit", "map", "128MiB", "-limit", "disk", "0", "-limit", "width", "32768", "-limit", "height", "32768", "-limit", "time", "10"}
	if format == "JPEG" {
		cfg, err := jpeg.DecodeConfig(input)
		if err != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 120_000_000 {
			return nil, errors.New("JPEG 尺寸无效或过大")
		}
		if cfg.Width > maxWidth || cfg.Height > maxHeight {
			ratio := min(float64(maxWidth)/float64(cfg.Width), float64(maxHeight)/float64(cfg.Height))
			args = append(args, "-define", fmt.Sprintf("jpeg:size=%dx%d", max(1, int(float64(cfg.Width)*ratio)), max(1, int(float64(cfg.Height)*ratio))))
		}
		if _, err := input.Seek(0, 0); err != nil {
			return nil, err
		}
	}
	args = append(args, format+":-[0]", "-auto-orient", "-thumbnail", fmt.Sprintf("%dx%d>", maxWidth, maxHeight), "-background", "white", "-alpha", "remove", "-alpha", "off", "-strip", "-quality", fmt.Sprint(quality), "JPEG:-")
	cmd := exec.CommandContext(ctx, program, args...)
	hideWindow(cmd)
	cmd.Stdin = input
	out, stderr := &thumbnailBuffer{limit: 8 << 20}, &thumbnailBuffer{limit: 4096}
	cmd.Stdout, cmd.Stderr = out, stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("生成图片缩略图失败：%w %s", err, strings.TrimSpace(stderr.String()))
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(out.Bytes()))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > maxWidth || cfg.Height > maxHeight {
		return nil, errors.New("图片缩略图输出无效")
	}
	return out.Bytes(), nil
}
