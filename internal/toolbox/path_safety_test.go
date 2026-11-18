package toolbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafetyRejectsProtectedInputsBeforeScanning(t *testing.T) {
	home, _ := os.UserHomeDir()
	exe, _ := os.Executable()
	root := filepath.VolumeName(systemDirectory()) + string(filepath.Separator)
	for _, path := range []string{root, systemDirectory(), home, filepath.Dir(home), filepath.Dir(exe), os.TempDir()} {
		for _, tool := range []string{"format", "rename", "number", "describe", "image", "video", "thumbnail", "exif", "pdf", "archive", "extract"} {
			c := DefaultConfig()
			c.Tool, c.Root = tool, path
			if _, e := BuildPlan(context.Background(), c, quiet); e == nil || !strings.Contains(e.Error(), "安全拦截") {
				t.Fatalf("%s accepted protected path %q: %v", tool, path, e)
			}
		}
		if _, e := InspectNumbering(context.Background(), Config{Root: path}); e == nil {
			t.Fatalf("number inspection accepted %q", path)
		}
	}
}

func TestSafetyAllowsResourceFolderNames(t *testing.T) {
	root := t.TempDir()
	// These names are protected only at actual system locations, not in albums.
	for _, name := range []string{"Windows", "Boot", "Recovery", "Program Files"} {
		file(t, filepath.Join(root, name, "photo.jpg"), []byte("image"))
	}
	c := DefaultConfig()
	c.Tool, c.Root = "format", root
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil || len(p.Items) != 4 {
		t.Fatalf("ordinary folders rejected: %v %+v", e, p)
	}
	r := Execute(context.Background(), p, quiet, progress)
	if r.Success != 4 || r.Failed != 0 {
		t.Fatal(r)
	}
}

func TestSafetyRejectsReplacedDirectory(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "original")
	file(t, filepath.Join(source, "a.jpg"), []byte("image"))
	c := DefaultConfig()
	c.Tool, c.Root, c.Recursive = "format", root, false
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Rename(source, filepath.Join(root, "saved-original")); e != nil {
		t.Fatal(e)
	}
	file(t, filepath.Join(source, "b.jpg"), []byte("replacement"))
	r := Execute(context.Background(), p, quiet, progress)
	if r.Failed == 0 || r.Success != 0 {
		t.Fatal(r)
	}
	if _, e := os.Stat(filepath.Join(source, "b.jpg")); e != nil {
		t.Fatal("replacement was changed", e)
	}
}

func TestSafetyRejectsEscapingTargetsAndProtectedOutputs(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "a.txt")
	file(t, source, []byte("source"))
	c := DefaultConfig()
	c.Tool, c.Root, c.Prefix = "rename", root, "new-"
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil {
		t.Fatal(e)
	}
	p.Items[0].Target = filepath.Join(t.TempDir(), "escaped.txt")
	if r := Execute(context.Background(), p, quiet, progress); r.Success != 0 || r.Failed == 0 {
		t.Fatal(r)
	}
	if _, e = os.Stat(source); e != nil {
		t.Fatal(e)
	}
	c.Tool, c.Output, c.SevenZip = "archive", systemDirectory(), source
	// Use an executable to pass dependency-location validation, never run it.
	c.SevenZip, _ = os.Executable()
	if _, e = Normalize(c); e == nil || !strings.Contains(e.Error(), "安全拦截") {
		t.Fatal("system output accepted", e)
	}
	if e = publish(source, filepath.Join(systemDirectory(), "toolbox-never-create.txt")); e == nil {
		t.Fatal("publisher accepted system output")
	}
}

func TestSafetyProtectsPrivateDataAndAncestors(t *testing.T) {
	root := t.TempDir()
	private := filepath.Join(root, "private")
	ProtectDataDirectory(private)
	for _, path := range []string{root, private, filepath.Join(private, "settings.json")} {
		if _, e := safePath(path); e == nil {
			t.Fatalf("private path accepted: %s", path)
		}
	}
	if _, e := safePath(filepath.Join(root, "resources")); e != nil {
		t.Fatal(e)
	}
}

func TestSafetyRejectsReplacedOutputDirectory(t *testing.T) {
	c := archiveFixture(t)
	file(t, filepath.Join(c.Root, "resource.txt"), []byte("resource"))
	base := t.TempDir()
	c.Output = filepath.Join(base, "output")
	if e := os.Mkdir(c.Output, 0755); e != nil {
		t.Fatal(e)
	}
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Rename(c.Output, filepath.Join(base, "saved-output")); e != nil {
		t.Fatal(e)
	}
	if e = os.Mkdir(c.Output, 0755); e != nil {
		t.Fatal(e)
	}
	if r := Execute(context.Background(), p, quiet, progress); r.Success != 0 || r.Failed == 0 {
		t.Fatal(r)
	}
	entries, e := os.ReadDir(c.Output)
	if e != nil || len(entries) != 0 {
		t.Fatal("replacement output was written", e, entries)
	}
}

func TestSafetyPreservesReplacedArchiveVolume(t *testing.T) {
	root := t.TempDir()
	first, second := filepath.Join(root, "archive.7z.001"), filepath.Join(root, "archive.7z.002")
	file(t, first, []byte("first"))
	file(t, second, []byte("original"))
	volumes, e := extractVolumes(first)
	if e != nil {
		t.Fatal(e)
	}
	job := extractJob{Volumes: volumes, VolumeCount: 2, volumeIDs: map[string]fileIdentity{}}
	for _, volume := range volumes {
		id, e := snapshotFileIdentity(volume.Source)
		if e != nil {
			t.Fatal(e)
		}
		job.volumeIDs[volume.Source] = id
	}
	st, e := os.Stat(second)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Rename(second, filepath.Join(root, "saved-original")); e != nil {
		t.Fatal(e)
	}
	file(t, second, []byte("replaced"))
	if e = os.Chtimes(second, st.ModTime(), st.ModTime()); e != nil {
		t.Fatal(e)
	}
	if e = deleteExtractSources(context.Background(), Config{Root: root, ExtractDeleteMode: "permanent"}, Item{Source: first}, job, quiet); e == nil {
		t.Fatal("replaced volume accepted")
	}
	for _, path := range []string{first, second} {
		if _, e = os.Stat(path); e != nil {
			t.Fatal("source removed", e)
		}
	}
}
