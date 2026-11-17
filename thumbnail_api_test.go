package main

import (
	"context"
	"encoding/json"
	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestThumbnailAPIIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") != "1" {
		t.Skip("requires local media dependencies")
	}
	c := toolbox.DefaultConfig()
	for relative, field := range map[string]*string{
		"tools/ffmpeg/bin/ffmpeg.exe":           &c.FFmpeg,
		"tools/ffmpeg/bin/ffprobe.exe":          &c.FFprobe,
		"tools/imagemagick/magick.exe":          &c.Magick,
		"tools/exiftool/exiftool.exe":           &c.Exiftool,
		"tools/fonts/NotoSansCJKsc-Regular.otf": &c.Font,
	} {
		path, e := filepath.Abs(filepath.FromSlash(relative))
		if e != nil {
			t.Fatal(e)
		}
		if st, e := os.Stat(path); e == nil && st.Mode().IsRegular() {
			*field = path
		}
	}
	c.Tool, c.Root = "thumbnail", t.TempDir()
	c.ThumbnailColumns, c.ThumbnailRows, c.ThumbnailWidth = 2, 2, 480
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc=size=320x180:rate=12", "-t", "2", "-c:v", "libx264", "-pix_fmt", "yuv420p", filepath.Join(c.Root, "sample.mp4"))
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatal(e, string(out))
	}
	s := &server{token: "test-session", dataDir: t.TempDir()}
	t.Cleanup(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.cancel != nil {
			s.cancel()
		}
	})
	if w := requestAPI(t, s, "/api/save-settings", c); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w := requestAPI(t, s, "/api/dependencies", map[string]any{"Config": c, "Verify": true})
	var deps []toolbox.Dependency
	if e := json.Unmarshal(w.Body.Bytes(), &deps); e != nil || len(deps) != 4 {
		t.Fatal(e, w.Body.String())
	}
	for _, d := range deps {
		if !d.Available {
			t.Fatal(d)
		}
	}
	if w := requestAPI(t, s, "/api/plan", c); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	wait := func(active string) state {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for {
			w := requestAPI(t, s, "/api/state", map[string]bool{"Summary": true})
			var result state
			if e := json.Unmarshal(w.Body.Bytes(), &result); e != nil {
				t.Fatal(e)
			}
			if result.Status != active {
				return result
			}
			if time.Now().After(deadline) {
				t.Fatal("API task did not finish", result)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	ready := wait("scanning")
	if ready.Status != "ready" || ready.Actionable != 1 || ready.Total != 1 {
		t.Fatal(ready)
	}
	w = requestAPI(t, s, "/api/preview", previewRequest{PlanID: ready.PlanID, Limit: 100})
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := requestAPI(t, s, "/api/execute", map[string]string{"PlanID": ready.PlanID}); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	finished := wait("running")
	if finished.Status != "finished" || finished.Result.Success != 1 || finished.Result.Failed != 0 {
		t.Fatal(finished)
	}
	if st, e := os.Stat(filepath.Join(c.Root, "视频预览图", "sample.mp4.jpg")); e != nil || st.Size() == 0 {
		t.Fatal("thumbnail was not published", e)
	}
	w = requestAPI(t, s, "/api/settings", map[string]any{})
	var p preferences
	if e := json.Unmarshal(w.Body.Bytes(), &p); e != nil || p.LastTool != "thumbnail" || p.Workspaces["thumbnail"].ThumbnailWidth != 480 {
		t.Fatal(p, e)
	}
}
