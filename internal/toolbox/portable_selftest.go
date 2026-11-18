package toolbox

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

// SelfTestPortable exercises the published binary using only its bundled tools.
// Every input and output lives in a temporary directory, never in user settings.
func SelfTestPortable(ctx context.Context, log func(string)) error {
	root, e := os.MkdirTemp("", "hamster-selftest-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(root)
	c := DefaultConfig()
	base := executableDirectory()
	c.FFmpeg, c.FFprobe, c.Magick = bundledDependency(base, "FFmpeg"), bundledDependency(base, "FFprobe"), bundledDependency(base, "Magick")
	c.Exiftool, c.Font = bundledDependency(base, "Exiftool"), bundledDependency(base, "Font")
	c.SevenZip = bundledDependency(base, "SevenZip")
	c.Recursive = false
	c.MinMB = 0
	c.MinKbps = 0
	img := image.NewNRGBA(image.Rect(0, 0, 320, 180))
	for y := 0; y < 180; y++ {
		for x := 0; x < 320; x++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(x), uint8(y), 80, 200})
		}
	}
	writeImage := func(path string) error {
		if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
			return e
		}
		f, e := os.Create(path)
		if e != nil {
			return e
		}
		if strings.EqualFold(filepath.Ext(path), ".jpg") {
			e = jpeg.Encode(f, img, &jpeg.Options{Quality: 100})
		} else {
			enc := png.Encoder{CompressionLevel: png.NoCompression}
			e = enc.Encode(f, img)
		}
		ce := f.Close()
		if e != nil {
			return e
		}
		return ce
	}
	video := filepath.Join(root, "source.mp4")
	if _, e = run(ctx, c.FFmpeg, "-v", "error", "-nostdin", "-f", "lavfi", "-i", "testsrc=size=320x180:rate=12", "-t", "2", "-c:v", "libx264", "-crf", "1", "-pix_fmt", "yuv420p", video); e != nil {
		return fmt.Errorf("test video: %w", e)
	}
	for _, tool := range []string{"format", "rename", "number", "describe", "exif", "pdf", "image", "video", "thumbnail", "archive", "extract"} {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		active := c
		active.Tool = tool
		active.Root = filepath.Join(root, tool)
		active.Output = ""
		if e = os.MkdirAll(active.Root, 0755); e != nil {
			return e
		}
		switch tool {
		case "extract":
			active.ExtractPassword = "SelfTestOnly!"
			input := filepath.Join(root, "extract-input.txt")
			e = os.WriteFile(input, []byte("extraction test"), 0600)
			if e == nil {
				_, e = run(ctx, active.SevenZip, "a", "-t7z", "-p"+active.ExtractPassword, "-mhe=on", filepath.Join(active.Root, "演示.7z"), input)
			}
		case "archive":
			active.ArchivePassword = "SelfTestOnly!"
			e = os.WriteFile(filepath.Join(active.Root, "中文资源!.txt"), []byte(strings.Repeat("archive test\n", 2000)), 0600)
		case "format", "pdf":
			e = writeImage(filepath.Join(active.Root, "中文演示", "cover.jpg"))
		case "rename":
			active.Prefix = "new-"
			active.Dirs = false
			e = os.WriteFile(filepath.Join(active.Root, "old.txt"), []byte("test"), 0600)
		case "number":
			active.NumberName = "Example"
			e = os.Mkdir(filepath.Join(active.Root, "album"), 0755)
		case "describe", "video", "thumbnail":
			e = publishContext(ctx, video, filepath.Join(active.Root, "中文 [100%].mp4"))
			active.ThumbnailColumns, active.ThumbnailRows, active.ThumbnailWidth = 2, 2, 480
			active.CRF = 28
		case "image":
			active.ImageFormats = []string{".png"}
			active.PngMode = "webp-png"
			e = writeImage(filepath.Join(active.Root, "透明测试.png"))
		case "exif":
			source := filepath.Join(active.Root, "metadata.jpg")
			e = writeImage(source)
			if e == nil {
				_, e = run(ctx, active.Exiftool, "-overwrite_original", "-Artist=HamsterSelfTest", source)
			}
		}
		if e != nil {
			return fmt.Errorf("%s fixture: %w", tool, e)
		}
		p, pe := BuildPlan(ctx, active, func(string) {})
		if pe != nil {
			return fmt.Errorf("%s preview: %w", tool, pe)
		}
		if len(p.Items) != 1 || p.Items[0].Conflict != "" {
			return fmt.Errorf("%s unexpected plan: %+v", tool, p.Items)
		}
		messages := []string{}
		r := Execute(ctx, p, func(s string) { messages = append(messages, s) }, func(int, int) {})
		if r.Success != 1 || r.Failed != 0 {
			return fmt.Errorf("%s execution: %+v %s", tool, r, strings.Join(messages, "; "))
		}
		switch tool {
		case "extract":
			b, ie := os.ReadFile(filepath.Join(p.Items[0].Target, "extract-input.txt"))
			if ie != nil || string(b) != "extraction test" {
				return fmt.Errorf("extraction content check failed: %v", ie)
			}
		case "image":
			info, ie := inspectImage(ctx, active.Magick, p.Items[0].Target)
			if ie != nil || info.format != "WEBP" || info.width != 320 || info.height != 180 {
				return fmt.Errorf("WebP output check failed: %+v %v", info, ie)
			}
		case "exif":
			out, ie := run(ctx, active.Exiftool, "-j", "-Artist", p.Items[0].Target)
			if ie != nil || strings.Contains(string(out), "HamsterSelfTest") {
				return fmt.Errorf("metadata cleanup check failed: %v", ie)
			}
		case "pdf":
			f, ie := os.Open(p.Config.Output)
			if ie != nil {
				return ie
			}
			header := make([]byte, 5)
			_, ie = f.Read(header)
			f.Close()
			if ie != nil || string(header) != "%PDF-" {
				return fmt.Errorf("PDF output check failed")
			}
		case "thumbnail":
			f, ie := os.Open(p.Items[0].Target)
			if ie != nil {
				return ie
			}
			out, _, ie := image.Decode(f)
			f.Close()
			if ie != nil || out.Bounds().Dx() != 480 {
				return fmt.Errorf("thumbnail output check failed: %v", ie)
			}
			// Chinese glyphs at the start of the header must be visible.
			ink := 0
			for y := 8; y < 32; y++ {
				for x := 10; x < 34; x++ {
					r, g, b, _ := out.At(x, y).RGBA()
					if r < 50000 || g < 50000 || b < 50000 {
						ink++
					}
				}
			}
			if ink < 10 {
				return fmt.Errorf("Chinese font rendering check failed")
			}
		}
		log("PASS: " + tool)
	}
	return nil
}
