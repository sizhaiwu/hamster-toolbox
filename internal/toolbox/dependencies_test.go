package toolbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDependenciesForEachWorkspace(t *testing.T) {
	want := map[string][]string{"format": {}, "rename": {}, "number": {}, "describe": {"FFprobe"}, "image": {"Magick"}, "exif": {"Exiftool"}, "video": {"FFmpeg", "FFprobe"}, "pdf": {"Magick", "Font"}, "thumbnail": {"FFmpeg", "FFprobe", "Magick", "Font"}}
	for tool, keys := range want {
		deps := RequiredDependencies(Config{Tool: tool})
		if len(deps) != len(keys) {
			t.Fatal(tool, deps)
		}
		for i, d := range deps {
			if d.Key != keys[i] || !strings.HasPrefix(d.DownloadURL, "https://") || d.Instructions == "" {
				t.Fatal(tool, d)
			}
		}
	}
}

func TestUnusedDependenciesAndOutputsDoNotAffectRename(t *testing.T) {
	c := DefaultConfig()
	c.Root = t.TempDir()
	c.Tool = "rename"
	c.Output = c.Root
	c.FFmpeg = "missing"
	c.Magick = "missing"
	c.Exiftool = "missing"
	c.Font = "missing"
	n, e := Normalize(c)
	if e != nil {
		t.Fatal(e)
	}
	if n.Output != "" || n.FFmpeg != "" || n.Font != "" {
		t.Fatal(n)
	}
	c.Tool = "describe"
	c.FFprobe = ""
	if _, e = Normalize(c); e == nil || !strings.Contains(e.Error(), "FFprobe") {
		t.Fatal(e)
	}
}

func TestDependencyStatusDoesNotExecuteDuringDiscovery(t *testing.T) {
	c := Config{Tool: "video", FFmpeg: filepath.Join(t.TempDir(), "missing.exe")}
	deps := CheckDependencies(context.Background(), c, false)
	for _, d := range deps {
		if d.Available || d.Message == "" {
			t.Fatal(d)
		}
	}
	font := filepath.Join(t.TempDir(), "font.ttf")
	os.WriteFile(font, []byte("test"), 0600)
	deps = CheckDependencies(context.Background(), Config{Tool: "pdf", Font: font}, false)
	if !deps[1].Available || deps[1].Verified {
		t.Fatal(deps)
	}
}

func TestAllDuplicateTargetsAreBlocked(t *testing.T) {
	root := t.TempDir()
	file(t, filepath.Join(root, "a.mp4"), []byte("a"))
	file(t, filepath.Join(root, "b.mp4"), []byte("b"))
	c := DefaultConfig()
	c.Tool = "rename"
	c.Root = root
	c.Find = "[ab]"
	c.Replace = "same"
	c.Regex = true
	c.Dirs = false
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Items) != 2 {
		t.Fatal(p)
	}
	for _, it := range p.Items {
		if it.Conflict == "" {
			t.Fatal("ambiguous target allowed")
		}
	}
	if CheckReady(p) == nil {
		t.Fatal("all-conflicting plan accepted")
	}
}

func TestDirectoryStatsAggregateOnceAndExcludeOutputs(t *testing.T) {
	root := t.TempDir()
	file(t, filepath.Join(root, "父", "子", "photo.jpg"), []byte("123"))
	file(t, filepath.Join(root, "父", "video.mp4"), []byte("12345"))
	file(t, filepath.Join(root, "父", "toolbox-output", "ignored.jpg"), []byte("ignored"))
	stats, e := measureDirectoryTree(context.Background(), root)
	if e != nil {
		t.Fatal(e)
	}
	v := stats[filepath.Join(root, "父")]
	if v.Size != 8 || v.Count != [3]int{1, 1, 0} {
		t.Fatal(v)
	}
	c := DefaultConfig()
	c.Tool = "format"
	c.Root = root
	c.Recursive = false
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Items) != 1 || !strings.HasSuffix(p.Items[0].Target, "[1P-1V-8B]") {
		t.Fatal(p.Items)
	}
}
