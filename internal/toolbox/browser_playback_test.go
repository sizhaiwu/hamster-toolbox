package toolbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBrowserPlaybackFormatsIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") != "1" {
		t.Skip("requires FFmpeg and FFprobe")
	}
	c := integrationConfig(t)
	cases := []struct {
		name                  string
		args                  []string
		copyVideo, fragmented bool
	}{
		{"normal.mp4", []string{"-c:v", "libx264", "-c:a", "aac"}, true, false},
		{"fragmented.mp4", []string{"-c:v", "libx264", "-c:a", "aac", "-movflags", "frag_keyframe+empty_moov+default_base_moof", "-frag_duration", "200000"}, true, true},
		{"movie.mkv", []string{"-c:v", "libx264", "-c:a", "aac"}, true, false},
		{"movie.avi", []string{"-c:v", "mpeg4", "-c:a", "libmp3lame"}, false, false},
		{"movie.ts", []string{"-c:v", "libx264", "-c:a", "aac"}, true, false},
		{"movie.mov", []string{"-c:v", "libx264", "-c:a", "pcm_s16le"}, true, false},
		{"movie.webm", []string{"-c:v", "libvpx-vp9", "-c:a", "libopus"}, false, false},
		{"hevc.mkv", []string{"-c:v", "libx265", "-pix_fmt", "yuv420p10le", "-x265-params", "log-level=error:pools=1:frame-threads=1", "-c:a", "aac"}, false, false},
		{"movie.wmv", []string{"-c:v", "wmv2", "-c:a", "wmav2"}, false, false},
		{"movie.flv", []string{"-c:v", "flv1", "-c:a", "libmp3lame"}, false, false},
		{"movie.mpg", []string{"-r", "25", "-c:v", "mpeg2video", "-c:a", "mp2"}, false, false},
		{"movie.3gp", []string{"-c:v", "mpeg4", "-c:a", "aac"}, false, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			path := filepath.Join(t.TempDir(), test.name)
			args := []string{"-v", "error", "-nostdin", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=12", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "1.5", "-threads", "1"}
			args = append(args, test.args...)
			args = append(args, path)
			if _, err := run(ctx, c.FFmpeg, args...); err != nil {
				t.Fatal(err)
			}
			original, _ := os.ReadFile(path)
			f, err := BrowseOpen(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			info, err := ProbeBrowserMedia(ctx, c.FFprobe, f)
			if err != nil || info.CopyVideo() != test.copyVideo || info.Fragmented != test.fragmented {
				t.Fatal(info, err)
			}
			thumb, err := BrowserVideoThumbnail(ctx, c.FFmpeg, f)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = jpeg.DecodeConfig(bytes.NewReader(thumb)); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			if err := StreamBrowserMedia(ctx, c.FFmpeg, f, info, 0, false, &output); err != nil {
				t.Fatal(err)
			}
			compatible := filepath.Join(t.TempDir(), "compatible.mp4")
			if err := os.WriteFile(compatible, output.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			converted, err := BrowseOpen(compatible)
			if err != nil {
				t.Fatal(err)
			}
			defer converted.Close()
			result, err := ProbeBrowserMedia(ctx, c.FFprobe, converted)
			if err != nil || result.VideoCodec != "h264" || result.AudioCodec != "aac" || result.Width > 1280 || result.Height > 720 {
				t.Fatal(result, err)
			}
			after, _ := os.ReadFile(path)
			if sha256.Sum256(after) != sha256.Sum256(original) {
				t.Fatal("source changed")
			}
		})
	}
}

type cancellingPlaybackWriter struct {
	cancel context.CancelFunc
	bytes  int
}

func (w *cancellingPlaybackWriter) Write(data []byte) (int, error) {
	w.bytes += len(data)
	w.cancel()
	return len(data), nil
}
func TestBrowserPlaybackCancellationIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") != "1" {
		t.Skip("requires FFmpeg")
	}
	c := integrationConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "long.mp4")
	if _, err := run(ctx, c.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "color=size=320x180:rate=24", "-t", "10", "-c:v", "libx264", "-threads", "1", path); err != nil {
		t.Fatal(err)
	}
	f, err := BrowseOpen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	info, err := ProbeBrowserMedia(ctx, c.FFprobe, f)
	if err != nil {
		t.Fatal(err)
	}
	streamCtx, stop := context.WithCancel(ctx)
	defer stop()
	writer := &cancellingPlaybackWriter{cancel: stop}
	if err := StreamBrowserMedia(streamCtx, c.FFmpeg, f, info, 0, false, writer); err == nil || writer.bytes == 0 {
		t.Fatal("stream did not stop after client cancellation", err)
	}
}
