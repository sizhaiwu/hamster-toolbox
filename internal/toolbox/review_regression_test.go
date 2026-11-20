package toolbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestConcurrentRenamesNeverOverwriteDestination(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "winner.txt")
	const count = 20
	for i := 0; i < count; i++ {
		file(t, filepath.Join(dir, fmt.Sprintf("source-%d.txt", i)), []byte(fmt.Sprint(i)))
	}
	start := make(chan struct{})
	results := make(chan int, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			if browserMoveNoReplace(filepath.Join(dir, fmt.Sprintf("source-%d.txt", i)), target) == nil {
				results <- i
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	winners := []int{}
	for i := range results {
		winners = append(winners, i)
	}
	if len(winners) != 1 {
		t.Fatal("destination was replaced by another rename", winners)
	}
	data, e := os.ReadFile(target)
	if e != nil || string(data) != fmt.Sprint(winners[0]) {
		t.Fatal("winning data changed", e, string(data))
	}
	for i := 0; i < count; i++ {
		if i == winners[0] {
			continue
		}
		data, e := os.ReadFile(filepath.Join(dir, fmt.Sprintf("source-%d.txt", i)))
		if e != nil || string(data) != fmt.Sprint(i) {
			t.Fatal("a losing source was lost or replaced", i, e)
		}
	}
}

func TestCompressedImageValidationPreservesDimensionsAndEncoding(t *testing.T) {
	source := imageInfo{"JPEG", 320, 240}
	if e := validateCompressedImage(Config{}, "input.jpg", source, imageInfo{"JPEG", 240, 320}); e != nil {
		t.Fatal("auto orientation should be allowed", e)
	}
	for _, output := range []imageInfo{{"PNG", 320, 240}, {"JPEG", 160, 120}, {"JPEG", 320, 239}} {
		if e := validateCompressedImage(Config{}, "input.jpg", source, output); e == nil {
			t.Fatal("invalid output was accepted", output)
		}
	}
	if e := validateCompressedImage(Config{PngMode: "lossless"}, "input.png", imageInfo{"PNG", 320, 240}, imageInfo{"PNG", 240, 320}); e == nil {
		t.Fatal("PNG dimensions changed")
	}
}

func TestAllCompressedImageFormatsIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") != "1" {
		t.Skip("requires ImageMagick")
	}
	c := integrationConfig(t)
	ctx := context.Background()
	for _, ext := range []string{"jpg", "jpeg", "jpe", "jfif", "webp", "png"} {
		t.Run(ext, func(t *testing.T) {
			dir := t.TempDir()
			source, target := filepath.Join(dir, "source."+ext), filepath.Join(dir, "result."+ext)
			fixture := source
			if ext == "jfif" {
				fixture = "JPEG:" + source
			}
			if _, e := run(ctx, c.Magick, "-size", "320x240", "gradient:#224466-#88aacc", "-quality", "100", fixture); e != nil {
				t.Fatal(e)
			}
			if e := compressImage(ctx, c, source, target); e != nil {
				t.Fatal(e)
			}
			if _, e := os.Stat(source); e != nil {
				t.Fatal("source removed", e)
			}
		})
	}
}

func TestFormatRejectsChangedStatisticsBeforeRenaming(t *testing.T) {
	root := t.TempDir()
	album := filepath.Join(root, "album")
	file(t, filepath.Join(album, "a.jpg"), []byte("image"))
	c := DefaultConfig()
	c.Tool, c.Root = "format", root
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil || len(p.Items) != 1 {
		t.Fatal(e, p)
	}
	file(t, filepath.Join(album, "new.mp4"), []byte("new resource"))
	r := Execute(context.Background(), p, quiet, progress)
	if r.Success != 0 || r.Failed != 1 {
		t.Fatal("stale statistics were used", r)
	}
	if _, e := os.Stat(album); e != nil {
		t.Fatal("original directory renamed", e)
	}
	if _, e := os.Stat(p.Items[0].Target); !os.IsNotExist(e) {
		t.Fatal("stale target created", e)
	}
}
