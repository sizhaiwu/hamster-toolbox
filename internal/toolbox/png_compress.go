package toolbox

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func normalizePNGOptions(c *Config) error {
	if c.PngMode == "" {
		c.PngMode = "lossless"
	}
	if c.PngQuality == 0 {
		c.PngQuality = 90
	}
	selected := false
	for _, ext := range c.ImageFormats {
		if ext == ".png" {
			selected = true
		}
	}
	if !selected {
		return nil
	}
	if c.PngMode != "lossless" && c.PngMode != "webp-png" && c.PngMode != "webp" {
		return fmt.Errorf("请选择有效的 PNG 压缩方案")
	}
	if c.PngMode != "lossless" && (c.PngQuality < 1 || c.PngQuality > 100) {
		return fmt.Errorf("PNG 的 WebP 画质须为 1–100")
	}
	return nil
}

// APNG and animated WebP are skipped to avoid changing animation frames.
// A .png filename containing WebP remains eligible for deliberate recompression.
func imageCompressionSkipReason(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	header := make([]byte, 21)
	n, e := io.ReadFull(f, header)
	if e != nil && e != io.ErrUnexpectedEOF && e != io.EOF {
		return "", e
	}
	// Extended WebP begins with VP8X; bit 1 of its feature flags marks animation.
	if n >= 21 && string(header[:4]) == "RIFF" && string(header[8:12]) == "WEBP" && string(header[12:16]) == "VP8X" && header[20]&2 != 0 {
		return "动画 WebP 暂不压缩，避免改变动画帧", nil
	}
	if n < 8 || !bytes.Equal(header[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10}) {
		return "", nil
	}
	if _, e = f.Seek(8, io.SeekStart); e != nil {
		return "", e
	}
	for {
		chunk := make([]byte, 8)
		if _, e = io.ReadFull(f, chunk); e != nil {
			return "", fmt.Errorf("PNG 数据不完整")
		}
		kind := string(chunk[4:8])
		if kind == "acTL" {
			return "动画 PNG 暂不转换，避免丢失动画帧", nil
		}
		if kind == "IDAT" || kind == "IEND" {
			return "", nil
		}
		length := int64(binary.BigEndian.Uint32(chunk[:4]))
		if _, e = f.Seek(length+4, io.SeekCurrent); e != nil {
			return "", e
		}
	}
}

type imageInfo struct {
	format        string
	width, height int
}

func inspectImage(ctx context.Context, exe, path string) (imageInfo, error) {
	out, e := run(ctx, exe, "identify", "-ping", "-format", "%m|%w|%h|%n\n", path)
	if e != nil {
		return imageInfo{}, e
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 1 {
		return imageInfo{}, fmt.Errorf("多帧图片暂不转换，避免丢失帧")
	}
	parts := strings.Split(lines[0], "|")
	if len(parts) != 4 {
		return imageInfo{}, fmt.Errorf("图片尺寸读取失败")
	}
	w, we := strconv.Atoi(parts[1])
	h, he := strconv.Atoi(parts[2])
	if we != nil || he != nil || w <= 0 || h <= 0 || strings.TrimSpace(parts[3]) != "1" {
		return imageInfo{}, fmt.Errorf("图片尺寸无效或含有多帧")
	}
	return imageInfo{parts[0], w, h}, nil
}

func compressImage(ctx context.Context, c Config, source, target string) error {
	reason, e := imageCompressionSkipReason(source)
	if e != nil {
		return e
	}
	if reason != "" {
		return fmt.Errorf("%w：%s", errImageSkipped, reason)
	}

	original, e := inspectImage(ctx, c.Magick, source)
	if e != nil {
		return fmt.Errorf("输入图片校验失败：%w", e)
	}
	if _, e = run(ctx, c.Magick, imageCompressionArgs(c, source, target)...); e != nil {
		return e
	}
	output, e := inspectImage(ctx, c.Magick, target)
	if e != nil {
		return fmt.Errorf("输出图片校验失败：%w", e)
	}
	return validateCompressedImage(c, source, original, output)
}

func validateCompressedImage(c Config, source string, original, output imageInfo) error {
	expected := "JPEG"
	ext := strings.ToLower(filepath.Ext(source))
	if ext == ".webp" {
		expected = "WEBP"
	}
	if ext == ".png" {
		expected = "PNG"
		if c.PngMode == "webp" || c.PngMode == "webp-png" {
			expected = "WEBP"
		}
	}
	dimensions := original.width == output.width && original.height == output.height
	// JPEG/WebP apply auto-orient; a 90-degree orientation swaps stored axes.
	if ext != ".png" {
		dimensions = dimensions || original.width == output.height && original.height == output.width
	}
	if output.format != expected || !dimensions {
		return fmt.Errorf("输出图片校验失败：编码格式或分辨率不符合预期，保留原图")
	}
	return nil
}
