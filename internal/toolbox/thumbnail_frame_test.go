package toolbox

import (
	"context"
	"crypto/sha256"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeThumbnailFrame(t *testing.T, path string, ink color.RGBA) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, ink)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestThumbnailEmptyFastSeekFallsBackWithoutStaleFrame(t *testing.T) {
	frame := filepath.Join(t.TempDir(), "frame.png")
	writeThumbnailFrame(t, frame, color.RGBA{B: 255, A: 255})
	var calls [][]string
	var messages []string
	r := thumbnailFrameExtractor{program: "synthetic-ffmpeg", source: "synthetic.mp4", log: func(message string) { messages = append(messages, message) }}
	r.run = func(ctx context.Context, exe string, args ...string) ([]byte, error) {
		if _, err := os.Stat(frame); !os.IsNotExist(err) {
			t.Fatal("previous frame reused", err)
		}
		calls = append(calls, append([]string{}, args...))
		if len(calls) < 3 {
			return nil, nil
		} // FFmpeg can succeed without an output frame.
		writeThumbnailFrame(t, frame, color.RGBA{R: 255, A: 255})
		return nil, nil
	}
	img, err := r.frame(context.Background(), frame, "scale=2:2", 60)
	if err != nil || img == nil || len(calls) != 3 || !r.compatibility || len(messages) != 1 {
		t.Fatal(len(calls), messages, err)
	}
	red, _, blue, _ := img.At(0, 0).RGBA()
	if red == 0 || blue != 0 {
		t.Fatal("stale blue frame was accepted")
	}
	seeks := func(args []string) (before, after []string) {
		input := false
		for i, arg := range args {
			if arg == "-i" {
				input = true
			}
			if arg == "-ss" && i+1 < len(args) {
				if input {
					after = append(after, args[i+1])
				} else {
					before = append(before, args[i+1])
				}
			}
		}
		return
	}
	a, b := seeks(calls[0])
	if len(a) != 1 || a[0] != "60.000000" || len(b) != 0 {
		t.Fatal("normal fast path changed", a, b)
	}
	a, b = seeks(calls[1])
	if len(a) != 1 || a[0] != "45.000000" || len(b) != 1 || b[0] != "15.000000" {
		t.Fatal("hybrid seek shifted target timestamp", a, b)
	}
	a, b = seeks(calls[2])
	if len(a) != 0 || len(b) != 1 || b[0] != "60.000000" {
		t.Fatal("accurate fallback shifted timestamp", a, b)
	}
	if _, err = r.frame(context.Background(), frame, "scale=2:2", 90); err != nil {
		t.Fatal(err)
	}
	a, b = seeks(calls[3])
	if len(a) != 1 || a[0] != "75.000000" || len(b) != 1 || b[0] != "15.000000" {
		t.Fatal("known broken fast seek was repeated", a, b)
	}
}
func TestThumbnailRetryHonorsCancellationAndExplainsEmptyFrame(t *testing.T) {
	frame := filepath.Join(t.TempDir(), "frame.png")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	r := thumbnailFrameExtractor{program: "synthetic", source: "synthetic.mp4", run: func(context.Context, string, ...string) ([]byte, error) { calls++; cancel(); return nil, nil }}
	if _, err := r.frame(ctx, frame, "scale=2:2", 60); err != context.Canceled || calls != 1 {
		t.Fatal("cancellation caused more decodes", calls, err)
	}
	r.run = func(context.Context, string, ...string) ([]byte, error) { return nil, nil }
	r.compatibility = false
	if _, err := r.frame(context.Background(), frame, "scale=2:2", 60); err == nil || !strings.Contains(err.Error(), "兼容定位仍未取得画面") || strings.Contains(err.Error(), "cannot find the file") {
		t.Fatal("missing frame misreported as filesystem failure", err)
	}
}
func TestThumbnailTransportStreamNamedMP4Integration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") != "1" {
		t.Skip("requires media runtime")
	}
	c := integrationConfig(t)
	c.Tool = "thumbnail"
	c.ThumbnailColumns = 5
	c.ThumbnailRows = 5
	c.ThumbnailWidth = 640
	source := filepath.Join(t.TempDir(), "long-gop-transport.mp4")
	if _, err := run(context.Background(), c.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=128x72:rate=12", "-t", "12", "-c:v", "libx264", "-g", "240", "-sc_threshold", "0", "-pix_fmt", "yuv420p", "-f", "mpegts", source); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "preview.jpg")
	if err = makeThumbnail(context.Background(), c, Item{Source: source, Size: st.Size()}, output); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	config, kind, err := image.DecodeConfig(f)
	if err != nil || kind != "jpeg" || config.Width != 640 {
		t.Fatal(config, kind, err)
	}
}
func thumbnailSourceDigest(t *testing.T, path string) [32]byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, f); err != nil {
		t.Fatal(err)
	}
	var result [32]byte
	copy(result[:], hash.Sum(nil))
	return result
}
func TestThumbnailSelectedRealSamplesIntegration(t *testing.T) {
	pattern := os.Getenv("TOOLBOX_THUMBNAIL_SAMPLE_GLOB")
	if pattern == "" {
		t.Skip("set a read-only video glob for local acceptance; outputs use temporary folders")
	}
	files, err := filepath.Glob(pattern)
	if err != nil || len(files) == 0 {
		t.Fatal("no selected video samples", err)
	}
	for _, source := range files {
		source := source
		t.Run(filepath.Base(source), func(t *testing.T) {
			t.Parallel()
			c := integrationConfig(t)
			c.Tool = "thumbnail"
			c.ThumbnailColumns = 5
			c.ThumbnailRows = 5
			c.ThumbnailWidth = 1024
			st, err := os.Stat(source)
			if err != nil {
				t.Fatal(err)
			}
			if !st.Mode().IsRegular() {
				t.Fatal("sample must be a regular file")
			}
			before := thumbnailSourceDigest(t, source)
			output := filepath.Join(t.TempDir(), "preview.jpg")
			var messages []string
			if err = makeThumbnailWithLog(context.Background(), c, Item{Source: source, Size: st.Size()}, output, func(message string) { messages = append(messages, message) }); err != nil {
				t.Fatal(err)
			}
			f, err := os.Open(output)
			if err != nil {
				t.Fatal(err)
			}
			config, kind, err := image.DecodeConfig(f)
			f.Close()
			if err != nil || kind != "jpeg" || config.Width != 1024 {
				t.Fatal(config, kind, err)
			}
			if before != thumbnailSourceDigest(t, source) {
				t.Fatal("original video changed")
			}
			t.Logf("PASS: 25-frame preview, width %d, compatibility switches %d, source unchanged", config.Width, len(messages))
		})
	}
}
