package toolbox

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBrowserVideoHoverIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") != "1" {
		t.Skip("requires FFmpeg and FFprobe")
	}
	c := integrationConfig(t)
	cases := []struct {
		name     string
		args     []string
		duration string
	}{
		{"sample.mp4", []string{"-c:v", "libx264", "-c:a", "aac"}, "8"},
		{"sample.mkv", []string{"-c:v", "libx265", "-x265-params", "pools=1:frame-threads=1:log-level=error", "-c:a", "aac"}, "1"},
		{"sample.avi", []string{"-c:v", "mpeg4", "-c:a", "libmp3lame"}, "1"},
		{"sample.ts", []string{"-c:v", "libx264", "-c:a", "aac"}, "1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			path := filepath.Join(t.TempDir(), tc.name)
			args := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc2=size=640x360:rate=15", "-f", "lavfi", "-i", "sine=frequency=440", "-t", tc.duration, "-threads", "1"}
			args = append(args, tc.args...)
			args = append(args, path)
			if _, err := run(ctx, c.FFmpeg, args...); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(path)
			f, err := BrowseOpen(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			data, err := BrowserVideoHover(ctx, c.FFmpeg, f)
			if err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(t.TempDir(), "preview.mp4")
			if err = os.WriteFile(output, data, 0600); err != nil {
				t.Fatal(err)
			}
			clip, err := os.Open(output)
			if err != nil {
				t.Fatal(err)
			}
			defer clip.Close()
			info, err := ProbeBrowserMedia(ctx, c.FFprobe, clip)
			if err != nil {
				t.Fatal(err)
			}
			if info.VideoCodec != "h264" || info.AudioCodec != "" || info.Width > 480 || info.Height > 360 || info.Duration > 6.1 || info.Duration <= 0 || len(data) >= 2<<20 {
				t.Fatal("clip not bounded/silent/browser compatible", info, len(data))
			}
			after, _ := os.ReadFile(path)
			if sha256.Sum256(before) != sha256.Sum256(after) {
				t.Fatal("source changed")
			}
			cancelled, stop := context.WithCancel(ctx)
			stop()
			if _, err = BrowserVideoHover(cancelled, c.FFmpeg, f); err == nil {
				t.Fatal("cancelled hover continued")
			}
		})
	}
}
