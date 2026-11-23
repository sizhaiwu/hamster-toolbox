package toolbox

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMetadataMismatchedImagesIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") != "1" {
		t.Skip("requires bundled ImageMagick and ExifTool")
	}
	ctx := context.Background()
	for _, tc := range []struct{ format, extension string }{{"PNG", ".jpg"}, {"JPEG", ".png"}, {"WEBP", ".png"}, {"GIF", ".jpg"}, {"TIFF", ".jpg"}, {"JPEG", ".jpg"}} {
		t.Run(tc.format+tc.extension, func(t *testing.T) {
			c := integrationConfig(t)
			c.Tool, c.Root, c.MetadataTypes = "exif", filepath.Join(t.TempDir(), "中文 空格 图集"), "images"
			if err := os.MkdirAll(c.Root, 0700); err != nil {
				t.Fatal(err)
			}
			original := filepath.Join(t.TempDir(), "fixture."+strings.ToLower(tc.format))
			if _, err := run(ctx, c.Magick, "-size", "32x24", "gradient:#224466-#88aacc", tc.format+":"+original); err != nil {
				t.Fatal(err)
			}
			if _, err := runInDirectory(ctx, filepath.Dir(original), c.Exiftool, "-XMP-dc:Description=synthetic-metadata-marker", "-overwrite_original", filepath.Base(original)); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(original)
			if err != nil {
				t.Fatal(err)
			}
			src := filepath.Join(c.Root, "测试 0021"+tc.extension)
			file(t, src, data)
			p, err := BuildPlan(ctx, c, quiet)
			if err != nil || len(p.Items) != 1 {
				t.Fatal(p, err)
			}
			logs := []string{}
			r := Execute(ctx, p, func(s string) { logs = append(logs, s) }, progress)
			if r.Success != 1 || r.Failed != 0 {
				t.Fatal(r, logs)
			}
			target := p.Items[0].Target
			if filepath.Base(target) != filepath.Base(src) {
				t.Fatal("filename changed", target)
			}
			f, err := os.Open(target)
			if err != nil {
				t.Fatal(err)
			}
			format := BrowserImageFormat(f)
			f.Close()
			if format != tc.format {
				t.Fatal("encoding changed", format)
			}
			var pixels [][]byte
			for _, path := range []string{src, target} {
				b, err := run(ctx, c.Magick, path, "-depth", "8", "RGBA:-")
				if err != nil {
					t.Fatal(err)
				}
				pixels = append(pixels, b)
			}
			if !bytes.Equal(pixels[0], pixels[1]) {
				t.Fatal("pixels changed")
			}
			// TIFF may retain unreferenced bytes after removing directory entries;
			// inspect the readable tag rather than searching its raw container bytes.
			cleaned, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			check := filepath.Join(filepath.Dir(original), "cleaned."+strings.ToLower(tc.format))
			file(t, check, cleaned)
			for _, path := range []string{original, check} {
				meta, err := runInDirectory(ctx, filepath.Dir(path), c.Exiftool, "-j", filepath.Base(path))
				if err != nil {
					t.Fatal(err)
				}
				hasMarker := bytes.Contains(meta, []byte("synthetic-metadata-marker"))
				if hasMarker != (path == original) {
					t.Fatal("unexpected readable metadata", string(meta))
				}
			}
			unchanged, err := os.ReadFile(src)
			if err != nil || !bytes.Equal(data, unchanged) {
				t.Fatal("source changed", err)
			}
			r = Execute(ctx, p, quiet, progress)
			if r.Success != 0 || r.Failed != 1 {
				t.Fatal("existing output was overwritten", r)
			}
		})
	}
}

func TestMetadataNewlinePathRejected(t *testing.T) {
	if err := runMetadataExiftool(context.Background(), "unused", "image\n.jpg"); err == nil {
		t.Fatal("argfile injection accepted")
	}
}

func TestNativeMetadataMismatchedImageUsesCLI(t *testing.T) {
	c := DefaultConfig()
	c.Tool, c.Root = "exif", t.TempDir()
	c.Output = filepath.Join(c.Root, "output")
	src := filepath.Join(c.Root, "image.jpg")
	file(t, src, []byte("\x89PNG\r\n\x1a\n"))
	_, err := NativeCommands(context.Background(), c, Item{Source: src, Target: filepath.Join(c.Output, "image.jpg")})
	if err == nil || !strings.Contains(err.Error(), "CLI") {
		t.Fatal("misleading direct ExifTool command exported", err)
	}
}
