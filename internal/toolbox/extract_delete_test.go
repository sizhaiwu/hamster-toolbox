package toolbox

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractDeleteDefaultAndScope(t *testing.T) {
	c := DefaultConfig()
	if c.ExtractDeleteSource || c.ExtractDeleteMode != "recycle" {
		t.Fatal("destructive option enabled by default")
	}
	c.Tool = "extract"
	c.ExtractDeleteSource = true
	if !ScopeConfig(c).ExtractDeleteSource {
		t.Fatal("extraction option dropped")
	}
	c.Tool = "archive"
	if ScopeConfig(c).ExtractDeleteSource {
		t.Fatal("delete option leaked to another tool")
	}
}

func TestExtractVolumeCountIgnoresCommentsAndInnerArchive(t *testing.T) {
	if n := extractVolumeCount("Type = Split\nVolumes = 6\n----\nType = Rar5\nVolumes = 1"); n != 6 {
		t.Fatal(n)
	}
	if n := extractVolumeCount("Type = zip\nComment = untrusted\nVolumes = 2"); n != 0 {
		t.Fatal("comment interpreted as volume metadata", n)
	}
}

func TestExtractDeleteDoesNotTrustZIPCommentIntegration(t *testing.T) {
	c := extractFixture(t)
	c.ExtractDeleteMode = "permanent"
	c.ExtractDeleteSource = true
	source := filepath.Join(c.Root, "archive.zip")
	f, e := os.Create(source)
	if e != nil {
		t.Fatal(e)
	}
	w := zip.NewWriter(f)
	if e = w.SetComment("untrusted comment\nVolumes = 2"); e != nil {
		t.Fatal(e)
	}
	entry, e := w.Create("file.txt")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = entry.Write([]byte("contents")); e != nil {
		t.Fatal(e)
	}
	if e = w.Close(); e != nil {
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
	other := filepath.Join(c.Root, "archive.z01")
	writeArchiveFixture(t, other, []byte("unrelated stale volume"))
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil || p.Items[0].Conflict != "" {
		t.Fatal(p, e)
	}
	Execute(context.Background(), p, quiet, progress)
	if !regularFile(source) || !regularFile(other) {
		t.Fatal("comment caused an unsafe cleanup")
	}
}

func TestExtractDeleteSingleIntegration(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "default-preserve", true: "delete-enabled"}[enabled], func(t *testing.T) {
			c := extractFixture(t)
			c.ExtractDeleteMode = "permanent"
			c.ExtractDeleteSource = enabled
			source := filepath.Join(c.Root, "resource.zip")
			writeExtractZIP(t, source, map[string]string{"file.txt": "contents"})
			stale := filepath.Join(c.Root, "resource.z01")
			writeArchiveFixture(t, stale, []byte("unrelated old volume"))
			sidecar := source + ".uploading.cfg"
			writeArchiveFixture(t, sidecar, []byte("upload state"))
			p, e := BuildPlan(context.Background(), c, quiet)
			if e != nil || len(p.Items) != 1 || p.Items[0].Conflict != "" {
				t.Fatal(p, e)
			}
			if p.extracts[0].VolumeCount != 1 {
				t.Fatal("single ZIP incorrectly grouped", p.extracts[0].VolumeCount)
			}
			r := Execute(context.Background(), p, quiet, progress)
			if r.Success != 1 {
				t.Fatal(r)
			}
			_, e = os.Stat(source)
			if enabled && !os.IsNotExist(e) {
				t.Fatal("source not deleted", e)
			}
			if !enabled && e != nil {
				t.Fatal("default removed source", e)
			}
			b, _ := os.ReadFile(filepath.Join(p.Items[0].Target, "file.txt"))
			if string(b) != "contents" {
				t.Fatal("extracted content missing")
			}
			b, _ = os.ReadFile(stale)
			if string(b) != "unrelated old volume" {
				t.Fatal("unused same-name volume deleted")
			}
			if !regularFile(sidecar) {
				t.Fatal("unrelated sidecar deleted")
			}
		})
	}
}

func TestExtractDeleteSplitIntegration(t *testing.T) {
	c := extractFixture(t)
	c.ExtractDeleteMode = "permanent"
	c.ExtractDeleteSource = true
	input := filepath.Join(t.TempDir(), "file.bin")
	data := bytes.Repeat([]byte("contents"), 25000)
	writeArchiveFixture(t, input, data)
	base := filepath.Join(c.Root, "parts.7z")
	if _, e := run(context.Background(), c.SevenZip, "a", "-t7z", "-mx=0", "-v32k", base, input); e != nil {
		t.Fatal(e)
	}
	writeArchiveFixture(t, base+".002.uploading.cfg", []byte("keep"))
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil || p.Items[0].Conflict != "" {
		t.Fatal(p, e)
	}
	if p.extracts[0].VolumeCount < 2 || p.extracts[0].VolumeCount != len(p.extracts[0].Volumes) {
		t.Fatal("incorrect consumed volume count")
	}
	r := Execute(context.Background(), p, quiet, progress)
	if r.Success != 1 {
		t.Fatal(r)
	}
	for _, part := range p.extracts[0].Volumes {
		if _, e = os.Stat(part.Source); !os.IsNotExist(e) {
			t.Fatal("part not deleted", part.Source, e)
		}
	}
	b, _ := os.ReadFile(filepath.Join(p.Items[0].Target, "file.bin"))
	if !bytes.Equal(b, data) {
		t.Fatal("published output differs")
	}
	if !regularFile(base + ".002.uploading.cfg") {
		t.Fatal("upload record deleted")
	}
}

func TestExtractDeleteRetainsUnsafeOrIncompleteResultsIntegration(t *testing.T) {
	for _, mode := range []string{"partial-skip", "all-skip", "cancel", "publish-failure", "source-changed", "wrong-password", "corrupt"} {
		t.Run(mode, func(t *testing.T) {
			c := extractFixture(t)
			c.ExtractDeleteMode = "permanent"
			c.ExtractDeleteSource = true
			source := filepath.Join(c.Root, "archive.zip")
			writeExtractZIP(t, source, map[string]string{"a.txt": "aaa", "b.txt": "bbb"})
			if mode == "partial-skip" || mode == "all-skip" {
				c.ExtractConflict = "skip"
				writeArchiveFixture(t, filepath.Join(c.Root, "解压资源", "archive", "a.txt"), []byte("old"))
				if mode == "all-skip" {
					writeArchiveFixture(t, filepath.Join(c.Root, "解压资源", "archive", "b.txt"), []byte("old"))
				}
			}
			if mode == "publish-failure" {
				c.ExtractConflict = "overwrite"
				writeArchiveFixture(t, filepath.Join(c.Root, "解压资源", "archive", "a.txt", "keep.txt"), []byte("keep"))
			}
			if mode == "wrong-password" {
				if e := os.Remove(source); e != nil {
					t.Fatal(e)
				}
				input := filepath.Join(t.TempDir(), "file.txt")
				writeArchiveFixture(t, input, []byte("source"))
				if _, e := run(context.Background(), c.SevenZip, "a", "-tzip", "-pCorrect!", source, input); e != nil {
					t.Fatal(e)
				}
				c.ExtractPassword = "wrong"
			}
			if mode == "corrupt" {
				b, _ := os.ReadFile(source)
				at := bytes.Index(b, []byte("aaa"))
				if at < 0 {
					t.Fatal("fixture")
				}
				b[at] ^= 0xff
				if e := os.WriteFile(source, b, 0600); e != nil {
					t.Fatal(e)
				}
			}
			p, e := BuildPlan(context.Background(), c, quiet)
			if e != nil {
				t.Fatal(e)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			Execute(ctx, p, func(s string) {
				if strings.HasPrefix(s, "解压完成：") {
					if mode == "cancel" {
						cancel()
					}
					if mode == "source-changed" {
						writeArchiveFixture(t, source, []byte("replaced source after extraction"))
					}
				}
			}, progress)
			if !regularFile(source) {
				t.Fatal("source removed despite incomplete/skipped/cancelled/changed task")
			}
		})
	}
}

func TestExtractDeleteRetainsInternallySkippedEntriesIntegration(t *testing.T) {
	c := extractFixture(t)
	c.ExtractDeleteMode = "permanent"
	c.ExtractDeleteSource = true
	c.ExtractConflict = "skip"
	source := filepath.Join(c.Root, "duplicates.zip")
	f, e := os.Create(source)
	if e != nil {
		t.Fatal(e)
	}
	w := zip.NewWriter(f)
	for _, body := range []string{"first version", "second version"} {
		entry, e := w.Create("same.txt")
		if e != nil {
			t.Fatal(e)
		}
		if _, e = entry.Write([]byte(body)); e != nil {
			t.Fatal(e)
		}
	}
	if e = w.Close(); e != nil {
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil {
		t.Fatal(e)
	}
	var logs []string
	r := Execute(context.Background(), p, func(s string) { logs = append(logs, s) }, progress)
	if r.Success != 1 || !regularFile(source) || !strings.Contains(strings.Join(logs, "\n"), "原压缩包已保留") {
		t.Fatal("internally skipped versions lost", r, logs)
	}
}

func TestExtractDeleteRejectsOutOfRoot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archive.zip")
	writeArchiveFixture(t, path, []byte("source"))
	volumes, e := extractVolumes(path)
	if e != nil {
		t.Fatal(e)
	}
	e = deleteExtractSources(context.Background(), Config{Root: t.TempDir()}, Item{Source: path}, extractJob{Volumes: volumes, VolumeCount: 1}, quiet)
	if !errors.Is(e, errExtractDeleteSource) || !regularFile(path) {
		t.Fatal("out-of-root cleanup allowed", e)
	}
}
