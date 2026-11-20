package toolbox

import (
	"bytes"
	"context"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBrowserVideoThumbnailSeekableAndShortIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") != "1" {
		t.Skip("requires FFmpeg")
	}
	c := integrationConfig(t)
	t.Setenv("http_proxy", "http://127.0.0.1:1")
	for _, duration := range []string{"0.15", "2"} {
		t.Run(duration, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			path := filepath.Join(t.TempDir(), "视频 with spaces.mp4")
			if _, e := run(ctx, c.FFmpeg, "-v", "error", "-nostdin", "-f", "lavfi", "-i", "testsrc=size=640x360:rate=24", "-t", duration, "-c:v", "libx264", "-threads", "1", "-pix_fmt", "yuv420p", path); e != nil {
				t.Fatal(e)
			}
			f, e := BrowseOpen(path)
			if e != nil {
				t.Fatal(e)
			}
			defer f.Close()
			f.Seek(7, 0)
			data, e := BrowserVideoThumbnail(ctx, c.FFmpeg, f)
			if e != nil {
				t.Fatal(e)
			}
			cfg, e := jpeg.DecodeConfig(bytes.NewReader(data))
			if e != nil || cfg.Width < 1 || cfg.Height < 1 {
				t.Fatal("invalid thumbnail", e)
			}
		})
	}
}
