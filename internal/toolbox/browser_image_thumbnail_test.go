package toolbox

import (
	"bytes"
	"context"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBrowserImageThumbnailRejectsOtherDecoders(t *testing.T) {
	_, err := BrowserImageThumbnail(context.Background(), "not-a-program", nil, "PDF")
	if err == nil || !strings.Contains(err.Error(), "格式") {
		t.Fatal("unexpected formats must be rejected before starting any process", err)
	}
}

func TestBrowserImageThumbnailPinnedInputIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") != "1" {
		t.Skip("requires ImageMagick")
	}
	c := integrationConfig(t)
	for _, format := range []string{"JPEG", "PNG", "WEBP", "GIF", "BMP", "TIFF", "AVIF", "HEIC"} {
		t.Run(format, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			path := filepath.Join(t.TempDir(), "缩略图 sample [literal].jpg")
			if format == "HEIC" {
				fixture := os.Getenv("TOOLBOX_HEIC_FIXTURE")
				if fixture == "" {
					t.Skip("HEIC encoder is optional; set TOOLBOX_HEIC_FIXTURE to test a real HEIC")
				}
				data, err := os.ReadFile(fixture)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			} else if _, err := run(ctx, c.Magick, "-size", "120x80", "xc:skyblue", format+":"+path); err != nil {
				t.Fatal(err)
			}
			f, err := BrowseOpen(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if found := BrowserImageFormat(f); found != format {
				t.Fatalf("real image format: got %s, want %s", found, format)
			}
			f.Seek(12, 0) // The thumbnail helper must rewind the existing handle.
			width, height, actualFormat, err := BrowserImageDimensions(ctx, c.Magick, f)
			if err != nil || width < 1 || height < 1 || format != "HEIC" && (width != 120 || height != 80) || !strings.EqualFold(actualFormat, format) {
				t.Fatal("incorrect image properties", width, height, actualFormat, err)
			}
			data, err := BrowserImageThumbnail(ctx, c.Magick, f, format)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
			if err != nil || cfg.Width > 640 || cfg.Height > 480 || format != "HEIC" && (cfg.Width != 120 || cfg.Height != 80) {
				t.Fatal("invalid thumbnail or enlarged image", cfg, err)
			}
			preview, err := BrowserImagePreview(ctx, c.Magick, f, format)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := jpeg.DecodeConfig(bytes.NewReader(preview)); err != nil {
				t.Fatal(err)
			}
			cancelled, stop := context.WithCancel(context.Background())
			stop()
			if _, err := BrowserImageThumbnail(cancelled, c.Magick, f, format); err == nil {
				t.Fatal("cancelled thumbnail generation succeeded")
			}
		})
	}
}
