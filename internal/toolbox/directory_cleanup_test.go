package toolbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func formatCleanupFixture(t *testing.T) Config {
	t.Helper()
	c := DefaultConfig()
	c.Tool, c.Root, c.Recursive = "format", t.TempDir(), false
	file(t, filepath.Join(c.Root, "相册 [35P]", "a.jpg"), []byte("a"))
	file(t, filepath.Join(c.Root, "相册 [35P]", "v.mp4"), []byte("bc"))
	file(t, filepath.Join(c.Root, "相册 [35P]", "info.txt"), []byte("def"))
	file(t, filepath.Join(c.Root, "相册 [35P]", "nested", "link.url"), []byte("ghij"))
	file(t, filepath.Join(c.Root, "root-note.txt"), []byte("keep root file"))
	return c
}

func TestFormatCleanupDefaultsScopeAndKnownMedia(t *testing.T) {
	c := DefaultConfig()
	if c.FormatCleanOther || c.FormatDeleteMode != "recycle" {
		t.Fatal("unsafe cleanup default")
	}
	c.FormatCleanOther, c.Tool = true, "format"
	if !ScopeConfig(c).FormatCleanOther {
		t.Fatal("cleanup option dropped")
	}
	c.Tool = "rename"
	if ScopeConfig(c).FormatCleanOther || ScopeConfig(c).FormatDeleteMode != "" {
		t.Fatal("cleanup leaked to rename")
	}
	for _, name := range []string{"image.JPG", "page.pdf", "clip.mov", "clip.ts", "clip.avi", "clip.m2ts", "song.flac", "list.m3u", "image.avif", "camera.dng", "camera.cr3", "clip.hevc", "book.m4b"} {
		if formatMediaKind(name) < 0 {
			t.Fatal("known media would be deleted", name)
		}
	}
	for _, name := range []string{"info.txt", "site.url", "archive.zip", "subtitle.srt", "unknown", "script.bat"} {
		if formatMediaKind(name) >= 0 {
			t.Fatal("unexpected media", name)
		}
	}
}

func TestFormatCleanupPreviewAndExecute(t *testing.T) {
	for _, recursive := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "recursive"}[recursive], func(t *testing.T) {
			c := formatCleanupFixture(t)
			c.Recursive = recursive
			p, e := BuildPlan(context.Background(), c, quiet)
			if e != nil {
				t.Fatal(e)
			}
			for _, it := range p.Items {
				if isFormatCleanup(it) {
					t.Fatal("cleanup enabled by default")
				}
			}
			if !strings.Contains(p.Items[0].Target+p.Items[len(p.Items)-1].Target, "1P-1V-10B") {
				t.Fatal(p.Items)
			}
			c.FormatCleanOther, c.FormatDeleteMode = true, "permanent"
			p, e = BuildPlan(context.Background(), c, quiet)
			if e != nil {
				t.Fatal(e)
			}
			cleanups := 0
			for _, it := range p.Items {
				if isFormatCleanup(it) {
					cleanups++
					if it.Action != "delete" {
						t.Fatal(it)
					}
					if _, e := os.Stat(it.Source); e != nil {
						t.Fatal("preview changed source", e)
					}
				}
				if it.Conflict != "" {
					t.Fatal(it)
				}
			}
			if cleanups != 2 {
				t.Fatal(cleanups, p.Items)
			}
			r := Execute(context.Background(), p, quiet, progress)
			if r.Failed != 0 || r.Success != len(p.Items) {
				t.Fatal(r)
			}
			target := filepath.Join(c.Root, "相册 [1P-1V-3B]")
			if _, e := os.Stat(filepath.Join(target, "a.jpg")); e != nil {
				t.Fatal(e)
			}
			if _, e := os.Stat(filepath.Join(target, "info.txt")); !os.IsNotExist(e) {
				t.Fatal("unknown retained", e)
			}
			if _, e := os.Stat(filepath.Join(c.Root, "root-note.txt")); e != nil {
				t.Fatal("root file changed", e)
			}
			p, e = BuildPlan(context.Background(), c, quiet)
			if e != nil || len(p.Items) != 0 {
				t.Fatal("not idempotent", e, p)
			}
		})
	}
}

func TestFormatCleanupPreservesUnpreviewedFiles(t *testing.T) {
	c := formatCleanupFixture(t)
	c.FormatCleanOther, c.FormatDeleteMode = true, "permanent"
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil {
		t.Fatal(e)
	}
	added := filepath.Join(c.Root, "相册 [35P]", "new.txt")
	r := Execute(context.Background(), p, quiet, func(done, _ int) {
		if done == 1 {
			file(t, added, []byte("arrived after preview"))
		}
	})
	if r.Failed == 0 {
		t.Fatal("stale statistics were applied", r)
	}
	if _, e := os.Stat(added); e != nil {
		t.Fatal("unpreviewed file removed or folder renamed", e)
	}
}

func TestFormatCleanupRejectsChangedAndForgedItems(t *testing.T) {
	for _, mode := range []string{"changed", "media", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			c := formatCleanupFixture(t)
			c.FormatCleanOther, c.FormatDeleteMode = true, "permanent"
			p, e := BuildPlan(context.Background(), c, quiet)
			if e != nil {
				t.Fatal(e)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			original := p.Items[0].Source
			switch mode {
			case "changed":
				file(t, original, []byte("changed since preview"))
			case "media":
				p.Items[0].Source = filepath.Join(c.Root, "相册 [35P]", "a.jpg")
				p.Items[0].Target = p.Items[0].Source
			case "cancelled":
				cancel()
			}
			r := Execute(ctx, p, quiet, progress)
			if mode != "cancelled" && r.Failed == 0 {
				t.Fatal("invalid cleanup succeeded", r)
			}
			if r.Success != 0 {
				t.Fatal("cleanup started before full validation", r)
			}
			if _, e := os.Stat(original); e != nil {
				t.Fatal(e)
			}
			if _, e := os.Stat(filepath.Join(c.Root, "相册 [35P]", "a.jpg")); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestFormatCleanupPreservesExcludedDirectories(t *testing.T) {
	c := formatCleanupFixture(t)
	c.FormatCleanOther, c.FormatDeleteMode = true, "permanent"
	for _, name := range []string{"toolbox-output", "0utput_7z", ".git", ".extract-owned", ".packing-owned", ".toolbox-owned"} {
		file(t, filepath.Join(c.Root, "相册 [35P]", name, "keep.zip"), []byte("keep generated or private file"))
	}
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil {
		t.Fatal(e)
	}
	for _, it := range p.Items {
		if filepath.Base(it.Source) == "keep.zip" {
			t.Fatal("excluded file scheduled for deletion", it)
		}
	}
	r := Execute(context.Background(), p, quiet, progress)
	if r.Failed != 0 {
		t.Fatal(r)
	}
	for _, name := range []string{"toolbox-output", "0utput_7z", ".git", ".extract-owned", ".packing-owned", ".toolbox-owned"} {
		if _, e := os.Stat(filepath.Join(c.Root, "相册 [1P-1V-3B]", name, "keep.zip")); e != nil {
			t.Fatal("excluded file lost", e)
		}
	}
}
