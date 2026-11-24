//go:build windows

package toolbox

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestThumbnailLockedLegacyFrameIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") != "1" {
		t.Skip("requires media runtime")
	}
	c := integrationConfig(t)
	c.Tool = "thumbnail"
	c.ThumbnailColumns = 2
	c.ThumbnailRows = 2
	c.ThumbnailWidth = 640
	c.ThumbnailInfo = false
	c.ThumbnailTime = false
	dir := t.TempDir()
	source := filepath.Join(dir, "中文 视频.mp4")
	if _, err := run(context.Background(), c.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=128x72:rate=12", "-t", "3", "-c:v", "libx264", "-pix_fmt", "yuv420p", source); err != nil {
		t.Fatal(err)
	}
	frame := filepath.Join(dir, "frame.png")
	writeThumbnailFrame(t, frame, color.RGBA{B: 255, A: 255})
	before, _ := os.ReadFile(frame)
	path, err := windows.UTF16PtrFromString(frame)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	if err := os.Remove(frame); err == nil {
		t.Fatal("test did not reproduce a locked temporary frame")
	} else {
		t.Logf("reproduced old frame cleanup failure: %v", err)
	}
	st, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	sourceBefore := thumbnailSourceDigest(t, source)
	target := filepath.Join(dir, "preview.jpg")
	if err := makeThumbnail(context.Background(), c, Item{Source: source, Size: st.Size()}, target); err != nil {
		t.Fatal("locked legacy frame blocked pipe extraction", err)
	}
	f, err := os.Open(target)
	if err != nil {
		t.Fatal(err)
	}
	cfg, kind, err := image.DecodeConfig(f)
	f.Close()
	if err != nil || kind != "jpeg" || cfg.Width != 640 {
		t.Fatal(cfg, kind, err)
	}
	after, err := os.ReadFile(frame)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("legacy frame was reused or modified", err)
	}
	if thumbnailSourceDigest(t, source) != sourceBefore {
		t.Fatal("source video was modified")
	}
}
