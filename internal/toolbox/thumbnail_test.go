package toolbox

import (
	"context"
	"crypto/sha256"
	"image"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestThumbnailTimeLabelCentering(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") != "1" {
		t.Skip("requires ImageMagick and font")
	}
	c := integrationConfig(t)
	fonts := []string{c.Font}
	if font := filepath.Join(os.Getenv("SystemRoot"), "Fonts", "segoeui.ttf"); regularFile(font) {
		fonts = append(fonts, font)
	}
	for _, font := range fonts {
		for _, point := range []int{10, 16, 18} {
			for _, text := range []string{"00:00:00", "01:23:45", "123:59:59"} {
				target := filepath.Join(t.TempDir(), "label.png")
				args := []string{"-font", font, "-encoding", "UTF-8"}
				args = append(args, thumbnailTimeLabelArgs(text, point)...)
				args = append(args, target)
				if _, e := runWithFont(context.Background(), c.Magick, args...); e != nil {
					t.Fatal(e)
				}
				f, e := os.Open(target)
				if e != nil {
					t.Fatal(e)
				}
				img, _, e := image.Decode(f)
				f.Close()
				if e != nil {
					t.Fatal(e)
				}
				bounds := img.Bounds()
				ink := image.Rectangle{Min: bounds.Max, Max: bounds.Min}
				for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
					for x := bounds.Min.X; x < bounds.Max.X; x++ {
						r, g, b, _ := img.At(x, y).RGBA()
						if r+g+b > 0 {
							ink.Min.X = min(ink.Min.X, x)
							ink.Min.Y = min(ink.Min.Y, y)
							ink.Max.X = max(ink.Max.X, x+1)
							ink.Max.Y = max(ink.Max.Y, y+1)
						}
					}
				}
				left, right := ink.Min.X-bounds.Min.X, bounds.Max.X-ink.Max.X
				top, bottom := ink.Min.Y-bounds.Min.Y, bounds.Max.Y-ink.Max.Y
				if ink.Empty() || math.Abs(float64(left-right)) > 1 || math.Abs(float64(top-bottom)) > 1 || left < 3 || top < 2 {
					t.Fatalf("uncentered %s at %d: margins %d,%d,%d,%d", text, point, left, right, top, bottom)
				}
			}
		}
	}
}

func TestThumbnailValidationAndWorkspaceIsolation(t *testing.T) {
	c := DefaultConfig()
	c.Tool, c.Root = "thumbnail", t.TempDir()
	exe, _ := os.Executable()
	c.FFmpeg, c.FFprobe, c.Magick, c.Font = exe, exe, exe, filepath.Join(c.Root, "font.ttf")
	file(t, c.Font, []byte("font fixture"))
	n, e := Normalize(c)
	if e != nil || n.Output != filepath.Join(c.Root, "视频预览图") || n.ThumbnailColumns != 5 || n.ThumbnailRows != 5 {
		t.Fatal(n, e)
	}
	for _, mutate := range []func(*Config){
		func(c *Config) { c.ThumbnailColumns = 11 }, func(c *Config) { c.ThumbnailRows = -1 },
		func(c *Config) { c.ThumbnailWidth = 200 }, func(c *Config) { c.ThumbnailGap = -1 },
		func(c *Config) { c.ThumbnailQuality = 101 }, func(c *Config) { c.ThumbnailFormat = "exe" },
		func(c *Config) { c.ThumbnailBackground = "#12345x" }, func(c *Config) { c.ThumbnailColumns = 10; c.ThumbnailWidth = 320 },
		func(c *Config) { c.Output = c.Root },
	} {
		bad := c
		mutate(&bad)
		if _, e := Normalize(bad); e == nil {
			t.Fatal("invalid thumbnail options accepted", bad)
		}
	}
	c.Quality, c.MinMB, c.CRF = -1, -1, 99
	if _, e := Normalize(c); e != nil {
		t.Fatal("unused compression settings block thumbnails", e)
	}
	scoped := ScopeConfig(c)
	if scoped.ThumbnailWidth != 1024 || !scoped.ThumbnailInfo || scoped.Quality != 0 || scoped.CRF != 0 || scoped.Exiftool != "" {
		t.Fatal(scoped)
	}
	c.Tool = "image"
	if got := ScopeConfig(c); got.ThumbnailWidth != 0 || got.ThumbnailInfo || got.FFmpeg != "" {
		t.Fatal("thumbnail settings leaked", got)
	}
	deps := RequiredDependencies(Config{Tool: "thumbnail"})
	keys := []string{}
	for _, d := range deps {
		keys = append(keys, d.Key)
	}
	if !reflect.DeepEqual(keys, []string{"FFmpeg", "FFprobe", "Magick", "Font"}) {
		t.Fatal(keys)
	}
}

func TestThumbnailLayoutAndSampling(t *testing.T) {
	c := DefaultConfig()
	for _, m := range []thumbnailVideo{
		{Width: 1920, Height: 1080, Duration: 3600, Aspect: 16.0 / 9, FPS: 30},
		{Width: 1080, Height: 1920, Duration: 10, Aspect: 9.0 / 16, FPS: 25},
	} {
		l, e := thumbnailDimensions(c, m, "很长的中文视频文件名 [100%] "+strings.Repeat("示例", 40)+".mp4", 100000)
		if e != nil || l.Height <= l.HeaderHeight || !strings.Contains(l.Header, "文件：") || len(strings.Split(l.Header, "\n")) < 3 {
			t.Fatal(l, e)
		}
		if math.Abs(float64(l.CellWidth)/float64(l.CellHeight)-m.Aspect) > 0.02 {
			t.Fatal("wrong aspect", l, m)
		}
		last := 0.0
		for i := 0; i < 25; i++ {
			v := thumbnailSeek(m, i, 25)
			if v <= last || v >= m.Duration {
				t.Fatal(v, last)
			}
			last = v
		}
	}
	short := thumbnailVideo{Duration: 0.04, FPS: 25}
	for i := 0; i < 25; i++ {
		if thumbnailSeek(short, i, 25) != 0 {
			t.Fatal("single-frame clip seeks beyond first frame")
		}
	}
	c.ThumbnailInfo = false
	l, e := thumbnailDimensions(c, thumbnailVideo{Aspect: 16.0 / 9}, "x", 1)
	if e != nil || l.HeaderHeight != 0 || l.Header != "" {
		t.Fatal(l, e)
	}
	if _, e = thumbnailDimensions(c, thumbnailVideo{Aspect: 0.001}, "x", 1); e == nil {
		t.Fatal("unbounded canvas accepted")
	}
}

func TestThumbnailIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") != "1" {
		t.Skip("requires FFmpeg, FFprobe, ImageMagick and CJK font")
	}
	c := integrationConfig(t)
	c.Tool = "thumbnail"
	c.Workers = 2
	c.Root = t.TempDir()
	c.ThumbnailColumns, c.ThumbnailRows, c.ThumbnailWidth = 3, 2, 640
	source := filepath.Join(c.Root, "子目录", "中文 [样片]100%.mp4")
	if e := os.MkdirAll(filepath.Dir(source), 0755); e != nil {
		t.Fatal(e)
	}
	_, e := run(context.Background(), c.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc=size=320x180:rate=12", "-t", "3", "-c:v", "libx264", "-pix_fmt", "yuv420p", source)
	if e != nil {
		t.Fatal(e)
	}
	original, _ := os.ReadFile(source)
	file(t, strings.TrimSuffix(source, ".mp4")+".mkv", original)
	file(t, filepath.Join(c.Root, "损坏.mp4"), []byte("broken video"))
	file(t, filepath.Join(c.Root, "cover.jpg"), []byte("not a video"))
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil || len(p.Items) != 3 {
		t.Fatal(p, e)
	}
	logs := []string{}
	r := Execute(context.Background(), p, func(s string) { logs = append(logs, s) }, progress)
	if r.Success != 2 || r.Skipped != 1 || r.Failed != 0 {
		t.Fatal(r, logs)
	}
	for _, it := range p.Items {
		if it.Conflict != "" {
			continue
		}
		if !strings.HasSuffix(it.Target, filepath.Base(it.Source)+".jpg") {
			t.Fatal(it.Target)
		}
		f, e := os.Open(it.Target)
		if e != nil {
			t.Fatal(e)
		}
		img, format, e := image.Decode(f)
		f.Close()
		if e != nil || format != "jpeg" || img.Bounds().Dx() != 640 {
			t.Fatal(format, e)
		}
		m, e := probeThumbnail(context.Background(), c.FFprobe, it.Source)
		if e != nil {
			t.Fatal(e)
		}
		l, e := thumbnailDimensions(c, m, filepath.Base(it.Source), it.Size)
		if e != nil || img.Bounds().Dy() != l.Height {
			t.Fatal(l, e)
		}
		// File information must produce non-background pixels in the header.
		ink := 0
		for y := 0; y < l.HeaderHeight; y++ {
			for x := 0; x < 640; x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				if r < 50000 || g < 50000 || b < 50000 {
					ink++
				}
			}
		}
		if ink < 100 {
			t.Fatal("file information was not rendered", ink)
		}
		cjkInk := 0
		for y := 8; y < 8+l.PointSize+8; y++ {
			for x := 10; x < 10+l.PointSize*2; x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				if r < 50000 || g < 50000 || b < 50000 {
					cjkInk++
				}
			}
		}
		if cjkInk < 10 {
			t.Fatal("Chinese file-information labels were not rendered")
		}
		// The changing test pattern must yield different tiles at different times.
		different := false
		for y := 10; y < l.CellHeight-25 && !different; y++ {
			for x := 10; x < l.CellWidth-10; x++ {
				if img.At(5+x, l.HeaderHeight+5+y) != img.At(5+l.CellWidth+5+x, l.HeaderHeight+5+y) {
					different = true
					break
				}
			}
		}
		if !different {
			t.Fatal("frames were duplicated")
		}
		t.Logf("generated %dx%d JPEG from %s", img.Bounds().Dx(), img.Bounds().Dy(), filepath.Base(it.Source))
	}
	after, _ := os.ReadFile(source)
	if sha256.Sum256(after) != sha256.Sum256(original) {
		t.Fatal("source video changed")
	}
	// Rotation metadata must agree with FFmpeg's automatic display orientation.
	rotated := filepath.Join(c.Root, "旋转视频.mp4")
	_, e = run(context.Background(), c.FFmpeg, "-v", "error", "-y", "-display_rotation:v:0", "90", "-i", source, "-c", "copy", rotated)
	if e != nil && (strings.Contains(e.Error(), "Unrecognized option") || strings.Contains(e.Error(), "Option not found")) {
		// Older FFmpeg versions write the display matrix through the rotate tag.
		_, e = run(context.Background(), c.FFmpeg, "-v", "error", "-y", "-i", source, "-c", "copy", "-metadata:s:v:0", "rotate=90", rotated)
	}
	if e != nil {
		t.Fatal(e)
	}
	rm, e := probeThumbnail(context.Background(), c.FFprobe, rotated)
	if e != nil || math.Abs(rm.Aspect-9.0/16) > 0.02 {
		t.Fatal("rotation metadata ignored", rm, e)
	}
	rotConfig := c
	rotConfig.ThumbnailColumns, rotConfig.ThumbnailRows = 1, 1
	rotTarget := filepath.Join(t.TempDir(), "rotated.jpg")
	rotStat, _ := os.Stat(rotated)
	if e = makeThumbnail(context.Background(), rotConfig, Item{Source: rotated, Size: rotStat.Size()}, rotTarget); e != nil {
		t.Fatal(e)
	}
	if e = os.Remove(rotated); e != nil {
		t.Fatal(e)
	}
	again, e := BuildPlan(context.Background(), c, quiet)
	if e != nil || CheckReady(again) == nil {
		t.Fatal("existing targets not blocked", again, e)
	}
	// Verify PNG output, a portrait clip, and a single-frame video.
	portrait := filepath.Join(c.Root, "竖屏单帧.mp4")
	_, e = run(context.Background(), c.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "color=c=blue:s=90x160:r=25", "-frames:v", "1", "-c:v", "libx264", "-pix_fmt", "yuv420p", portrait)
	if e != nil {
		t.Fatal(e)
	}
	c.Recursive = false
	c.ThumbnailFormat = "png"
	c.ThumbnailColumns, c.ThumbnailRows, c.ThumbnailWidth = 1, 2, 320
	c.ThumbnailInfo, c.ThumbnailTime = false, false
	pp, e := BuildPlan(context.Background(), c, quiet)
	if e != nil {
		t.Fatal(e)
	}
	r = Execute(context.Background(), pp, func(s string) { logs = append(logs, s) }, progress)
	if r.Success != 1 || r.Failed != 0 {
		t.Fatal(r, logs)
	}
	f, e := os.Open(filepath.Join(c.Root, "视频预览图", "竖屏单帧.mp4.png"))
	if e != nil {
		t.Fatal(e)
	}
	img, format, e := image.Decode(f)
	f.Close()
	if e != nil || format != "png" || img.Bounds().Dx() != 320 || img.Bounds().Dy() < 1000 {
		t.Fatal(format, e)
	}
	// Cancellation leaves neither output nor temporary image directories.
	c.Output = filepath.Join(c.Root, "cancelled")
	cp, e := BuildPlan(context.Background(), c, quiet)
	if e != nil {
		t.Fatal(e)
	}
	var active Item
	for _, it := range cp.Items {
		if it.Conflict == "" {
			active = it
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e = convert(ctx, cp.Config, active); e == nil {
		t.Fatal("cancellation ignored")
	}
	entries, e := os.ReadDir(c.Output)
	if e != nil || len(entries) != 0 {
		t.Fatal("partial output left after cancellation", entries, e)
	}
}
