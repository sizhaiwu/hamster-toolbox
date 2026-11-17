package toolbox

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func quiet(string)      {}
func progress(int, int) {}
func file(t *testing.T, p string, b []byte) {
	t.Helper()
	if e := os.MkdirAll(filepath.Dir(p), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(p, b, 0644); e != nil {
		t.Fatal(e)
	}
}
func TestNestedRename(t *testing.T) {
	root := t.TempDir()
	file(t, filepath.Join(root, "old父", "old子", "old图.jpg"), []byte("x"))
	c := DefaultConfig()
	c.Root = root
	c.Tool = "rename"
	c.Find = "old"
	c.Replace = "new"
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Items) != 3 {
		t.Fatal(len(p.Items))
	}
	r := Execute(context.Background(), p, quiet, progress)
	if r.Failed != 0 || r.Success != 3 {
		t.Fatal(r)
	}
	if _, e = os.Stat(filepath.Join(root, "new父", "new子", "new图.jpg")); e != nil {
		t.Fatal(e)
	}
}
func TestConflictAndTraversal(t *testing.T) {
	root := t.TempDir()
	file(t, filepath.Join(root, "a.jpg"), []byte("a"))
	file(t, filepath.Join(root, "b.jpg"), []byte("b"))
	c := DefaultConfig()
	c.Tool = "rename"
	c.Root = root
	c.Dirs = false
	c.Find = "a"
	c.Replace = "b"
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Items) != 1 || p.Items[0].Conflict == "" {
		t.Fatal(p)
	}
	c.Find = ""
	c.Prefix = "../escape/"
	p, e = BuildPlan(context.Background(), c, quiet)
	if e != nil {
		t.Fatal(e)
	}
	for _, it := range p.Items {
		if it.Conflict == "" {
			t.Fatal("invalid path allowed")
		}
	}
}
func TestStalePlanAndOutputExclusion(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "a.jpg")
	file(t, src, []byte("a"))
	file(t, filepath.Join(root, "toolbox-output", "nested.jpg"), []byte("a"))
	c := DefaultConfig()
	c.Tool = "rename"
	c.Root = root
	c.Prefix = "x"
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Items) != 1 {
		t.Fatal(len(p.Items))
	}
	file(t, src, []byte("changed"))
	r := Execute(context.Background(), p, quiet, progress)
	if r.Failed != 1 {
		t.Fatal(r)
	}
}
func TestFormatIdempotent(t *testing.T) {
	root := t.TempDir()
	file(t, filepath.Join(root, "相册", "a.jpg"), []byte("123"))
	c := DefaultConfig()
	c.Tool = "format"
	c.Root = root
	c.Recursive = false
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(p.Items[0].Target, "[1P-3B]") {
		t.Fatal(p.Items)
	}
	r := Execute(context.Background(), p, quiet, progress)
	if r.Failed > 0 {
		t.Fatal(r)
	}
	p, e = BuildPlan(context.Background(), c, quiet)
	if e != nil || len(p.Items) != 0 {
		t.Fatal(p, e)
	}
}
func TestCancellationAndNoOverwrite(t *testing.T) {
	root := t.TempDir()
	file(t, filepath.Join(root, "a.jpg"), []byte("a"))
	c := DefaultConfig()
	c.Tool = "rename"
	c.Root = root
	c.Prefix = "x"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := BuildPlan(ctx, c, quiet); e == nil {
		t.Fatal("cancel ignored")
	}
	src := filepath.Join(root, "tmp")
	dst := filepath.Join(root, "out")
	file(t, src, []byte("new"))
	file(t, dst, []byte("old"))
	if publish(src, dst) == nil {
		t.Fatal("overwrote file")
	}
	b, _ := os.ReadFile(dst)
	if string(b) != "old" {
		t.Fatal(string(b))
	}
}
func TestMediaIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") != "1" {
		t.Skip("set TOOLBOX_INTEGRATION=1 for real media tools")
	}
	root := t.TempDir()
	c := integrationConfig(t)
	c.Root = root
	c.MinMB = 0
	c.MinKbps = 0
	imagePath := filepath.Join(root, "中文相册", "测试.jpg")
	os.MkdirAll(filepath.Dir(imagePath), 0755)
	img := image.NewRGBA(image.Rect(0, 0, 800, 600))
	for y := 0; y < 600; y++ {
		for x := 0; x < 800; x++ {
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), uint8((x + y) % 256), 255})
		}
	}
	f, _ := os.Create(imagePath)
	jpeg.Encode(f, img, &jpeg.Options{Quality: 100})
	f.Close()
	_, e := run(context.Background(), c.Exiftool, "-overwrite_original", "-Artist=ToolboxTest", imagePath)
	if e != nil {
		t.Fatal(e)
	}
	videoPath := filepath.Join(root, "样片.mp4")
	_, e = run(context.Background(), c.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc=size=320x240:rate=24", "-t", "2", "-c:v", "libx264", "-crf", "1", "-pix_fmt", "yuv420p", videoPath)
	if e != nil {
		t.Fatal(e)
	}
	// One damaged file must be reported without blocking the healthy media.
	file(t, filepath.Join(root, "损坏视频.mp4"), []byte("not a valid video"))
	for _, tool := range []string{"describe", "exif", "image", "video", "pdf"} {
		c.Tool = tool
		c.Output = ""
		for _, d := range CheckDependencies(context.Background(), c, true) {
			if !d.Available {
				t.Fatal(tool, d.Name, d.Message)
			}
		}
		p, e := BuildPlan(context.Background(), c, quiet)
		if e != nil {
			t.Fatal(tool, e)
		}
		if len(p.Items) == 0 {
			t.Fatal(tool, "empty")
		}
		if tool == "describe" || tool == "video" {
			bad := 0
			for _, it := range p.Items {
				if it.Conflict != "" {
					bad++
				}
			}
			if bad != 1 {
				t.Fatal(tool, "expected one damaged file", p.Items)
			}
		}
		logs := []string{}
		r := Execute(context.Background(), p, func(s string) { logs = append(logs, s) }, progress)
		if r.Failed > 0 {
			t.Fatal(tool, r, logs)
		}
		t.Log(tool, r, logs)
		if tool == "exif" {
			out, e := run(context.Background(), c.Exiftool, "-j", "-Artist", p.Items[0].Target)
			if e != nil {
				t.Fatal(e)
			}
			var rows []map[string]interface{}
			json.Unmarshal(out, &rows)
			if _, ok := rows[0]["Artist"]; ok {
				t.Fatal("metadata retained")
			}
		}
		if tool == "pdf" {
			b, _ := os.ReadFile(p.Config.Output)
			if !strings.HasPrefix(string(b), "%PDF-1.4") || !strings.Contains(string(b), "/Count 1") {
				t.Fatal("bad PDF")
			}
			if outDir := os.Getenv("TOOLBOX_QA"); outDir != "" {
				os.MkdirAll(outDir, 0755)
				os.WriteFile(filepath.Join(outDir, "目录样例.pdf"), b, 0644)
				fmt.Println("QA PDF", outDir)
			}
		}
	}
}
