package toolbox

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func extractFixture(t *testing.T) Config {
	t.Helper()
	if os.Getenv("TOOLBOX_INTEGRATION") == "" {
		t.Skip("set TOOLBOX_INTEGRATION=1")
	}
	c := DefaultConfig()
	c.Tool = "extract"
	c.Root = t.TempDir()
	c.SevenZip, _ = filepath.Abs(filepath.Join("..", "..", "tools", "sevenzip", "7z.exe"))
	return c
}

func writeExtractZIP(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, e := os.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	w := zip.NewWriter(f)
	for name, body := range files {
		h := &zip.FileHeader{Name: name, Method: zip.Store}
		entry, e := w.CreateHeader(h)
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
}

func TestExtractNamesAndSafety(t *testing.T) {
	for _, name := range []string{"a.7z", "a.ZIP", "a.rar", "a.7z.001", "a.zip.001", "a.part01.rar"} {
		if _, ok := extractArchiveName(name); !ok {
			t.Fatal(name)
		}
	}
	for _, name := range []string{"a.7z.002", "a.part02.rar", "a.r00", "a.z01", "a.zip.001.uploading.cfg", "a.txt"} {
		if _, ok := extractArchiveName(name); ok {
			t.Fatal(name)
		}
	}
	for _, name := range []string{"../bad", "/bad", `C:\bad`, "name:stream", `x\..\bad`, "CON.txt", "bad\nname"} {
		if _, e := safeExtractName(name); e == nil {
			t.Fatal("unsafe path accepted", name)
		}
	}
	c := DefaultConfig()
	c.Tool = "extract"
	c.ExtractPassword = "test"
	c.ArchivePassword = "unused"
	c.ArchiveExtras = "unused"
	scoped := ScopeConfig(c)
	if scoped.ExtractPassword != "test" || scoped.ArchivePassword != "" || scoped.ArchiveExtras != "" {
		t.Fatal("password scope")
	}
	if c.ExtractConflict != "rename" || c.ExtractLayout != "separate" {
		t.Fatal("defaults")
	}
}

func TestExtractConflictModesIntegration(t *testing.T) {
	for _, mode := range []string{"rename", "skip", "overwrite"} {
		t.Run(mode, func(t *testing.T) {
			c := extractFixture(t)
			c.ExtractConflict = mode
			c.ExtractLayout = "merge"
			writeExtractZIP(t, filepath.Join(c.Root, "资源!& [1].zip"), map[string]string{"same.txt": "new", "sub/中文.txt": "nested"})
			output := filepath.Join(c.Root, "解压资源")
			writeArchiveFixture(t, filepath.Join(output, "same.txt"), []byte("old"))
			writeArchiveFixture(t, filepath.Join(output, "same_1.txt"), []byte("older"))
			p, e := BuildPlan(context.Background(), c, func(string) {})
			if e != nil {
				t.Fatal(e)
			}
			if len(p.Items) != 1 || p.Items[0].Conflict != "" {
				t.Fatal(p.Items)
			}
			var logs []string
			r := Execute(context.Background(), p, func(s string) { logs = append(logs, s) }, func(int, int) {})
			if r.Success != 1 || r.Failed != 0 {
				t.Fatal(r, logs)
			}
			b, _ := os.ReadFile(filepath.Join(output, "same.txt"))
			want := "old"
			if mode == "overwrite" {
				want = "new"
			}
			if string(b) != want {
				t.Fatal("wrong overwrite behavior", string(b))
			}
			if mode == "rename" {
				b, _ = os.ReadFile(filepath.Join(output, "same_2.txt"))
				if string(b) != "new" {
					t.Fatal("new file not renamed")
				}
			}
			b, _ = os.ReadFile(filepath.Join(output, "same_1.txt"))
			if string(b) != "older" {
				t.Fatal("existing rename candidate changed")
			}
			b, _ = os.ReadFile(filepath.Join(output, "sub", "中文.txt"))
			if string(b) != "nested" {
				t.Fatal("nested content missing")
			}
			if !regularFile(filepath.Join(c.Root, "资源!& [1].zip")) {
				t.Fatal("original archive removed")
			}
			if mode == "skip" {
				r = Execute(context.Background(), p, func(string) {}, func(int, int) {})
				if r.Skipped != 1 || r.Success != 0 || r.Failed != 0 {
					t.Fatal("all-skipped archive reported as success", r)
				}
			}
		})
	}
}

func TestExtractEncryptedSplitIntegration(t *testing.T) {
	c := extractFixture(t)
	source := filepath.Join(t.TempDir(), "source.bin")
	writeArchiveFixture(t, source, bytes.Repeat([]byte("split resource"), 15000))
	base := filepath.Join(c.Root, "split.7z")
	if _, e := run(context.Background(), c.SevenZip, "a", "-t7z", "-mx=0", "-v64k", "-pTestOnly!", "-mhe=on", base, source); e != nil {
		t.Fatal(e)
	}
	c.ExtractPassword = "wrong"
	p, e := BuildPlan(context.Background(), c, func(string) {})
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Items) != 1 || p.Items[0].Conflict == "" {
		t.Fatal("bad password not rejected", p.Items)
	}
	c.ExtractPassword = "TestOnly!"
	p, e = BuildPlan(context.Background(), c, func(string) {})
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Items) != 1 || p.Items[0].Conflict != "" || len(p.extracts[0].Volumes) < 2 {
		t.Fatal("split grouping", p.Items)
	}
	var logs []string
	r := Execute(context.Background(), p, func(s string) { logs = append(logs, s) }, func(int, int) {})
	if r.Success != 1 {
		t.Fatal(r, logs)
	}
	b, _ := os.ReadFile(filepath.Join(p.Items[0].Target, "source.bin"))
	original, _ := os.ReadFile(source)
	if !bytes.Equal(b, original) {
		t.Fatal("split extraction differs")
	}
	if strings.Contains(strings.Join(logs, "\n"), c.ExtractPassword) {
		t.Fatal("password logged")
	}
	// Changed or missing volumes invalidate a previously scanned plan.
	p, e = BuildPlan(context.Background(), c, func(string) {})
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Remove(base + ".002"); e != nil {
		t.Fatal(e)
	}
	r = Execute(context.Background(), p, func(string) {}, func(int, int) {})
	if r.Failed != 1 {
		t.Fatal("missing volume not detected", r)
	}
}

func TestExtractCorruptionAndUnsafePathsIntegration(t *testing.T) {
	c := extractFixture(t)
	path := filepath.Join(c.Root, "bad.zip")
	writeExtractZIP(t, path, map[string]string{"../outside.txt": "bad"})
	p, e := BuildPlan(context.Background(), c, func(string) {})
	if e != nil {
		t.Fatal(e)
	}
	if p.Items[0].Conflict == "" {
		t.Fatal("traversal allowed")
	}
	writeExtractZIP(t, path, map[string]string{"inside.txt": "unique-corruptible-payload"})
	data, _ := os.ReadFile(path)
	at := bytes.Index(data, []byte("unique-corruptible-payload"))
	if at < 0 {
		t.Fatal("fixture")
	}
	data[at] ^= 0xff
	if e = os.WriteFile(path, data, 0600); e != nil {
		t.Fatal(e)
	}
	p, e = BuildPlan(context.Background(), c, func(string) {})
	if e != nil {
		t.Fatal(e)
	}
	r := Execute(context.Background(), p, func(string) {}, func(int, int) {})
	if r.Failed != 1 {
		t.Fatal("corrupt archive accepted", r)
	}
	if _, e = os.Stat(filepath.Join(p.Items[0].Target, "inside.txt")); !os.IsNotExist(e) {
		t.Fatal("corrupt result published")
	}
}

// A minimal, original RAR4 store fixture: no external binary test data needed.
func writeExtractRAR(t *testing.T, path string) {
	t.Helper()
	var out bytes.Buffer
	out.Write([]byte{'R', 'a', 'r', '!', 0x1a, 0x07, 0})
	header := func(kind byte, flags uint16, tail []byte) {
		b := make([]byte, 7)
		b[2] = kind
		binary.LittleEndian.PutUint16(b[3:], flags)
		binary.LittleEndian.PutUint16(b[5:], uint16(7+len(tail)))
		b = append(b, tail...)
		binary.LittleEndian.PutUint16(b, uint16(crc32.ChecksumIEEE(b[2:])))
		out.Write(b)
	}
	header(0x73, 0, make([]byte, 6))
	body := []byte("RAR resource")
	name := []byte("hello.txt")
	tail := make([]byte, 25)
	binary.LittleEndian.PutUint32(tail, uint32(len(body)))
	binary.LittleEndian.PutUint32(tail[4:], uint32(len(body)))
	tail[8] = 2
	binary.LittleEndian.PutUint32(tail[9:], crc32.ChecksumIEEE(body))
	tail[17] = 20
	tail[18] = 0x30
	binary.LittleEndian.PutUint16(tail[19:], uint16(len(name)))
	binary.LittleEndian.PutUint32(tail[21:], 0x20)
	header(0x74, 0x8000, append(tail, name...))
	out.Write(body)
	header(0x7b, 0, nil)
	if e := os.WriteFile(path, out.Bytes(), 0600); e != nil {
		t.Fatal(e)
	}
}

func TestExtractRARIntegration(t *testing.T) {
	c := extractFixture(t)
	writeExtractRAR(t, filepath.Join(c.Root, "resource.rar"))
	p, e := BuildPlan(context.Background(), c, func(string) {})
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Items) != 1 || p.Items[0].Conflict != "" {
		t.Fatal(p.Items)
	}
	var logs []string
	r := Execute(context.Background(), p, func(s string) { logs = append(logs, s) }, func(int, int) {})
	if r.Success != 1 {
		t.Fatal(r, logs)
	}
	b, _ := os.ReadFile(filepath.Join(p.Items[0].Target, "hello.txt"))
	if string(b) != "RAR resource" {
		t.Fatal("RAR content differs")
	}
}

func TestExtractDirectoryConflictsAndCancellationIntegration(t *testing.T) {
	c := extractFixture(t)
	c.ExtractLayout = "merge"
	writeExtractZIP(t, filepath.Join(c.Root, "archive.zip"), map[string]string{"folder/inside.txt": "nested"})
	writeArchiveFixture(t, filepath.Join(c.Root, "解压资源", "folder"), []byte("existing file"))
	p, e := BuildPlan(context.Background(), c, func(string) {})
	if e != nil {
		t.Fatal(e)
	}
	r := Execute(context.Background(), p, func(string) {}, func(int, int) {})
	if r.Success != 1 {
		t.Fatal(r)
	}
	b, _ := os.ReadFile(filepath.Join(c.Root, "解压资源", "folder_1", "inside.txt"))
	if string(b) != "nested" {
		t.Fatal("directory conflict not renamed")
	}
	b, _ = os.ReadFile(filepath.Join(c.Root, "解压资源", "folder"))
	if string(b) != "existing file" {
		t.Fatal("existing file changed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e = extractOne(ctx, p.Config, p.Items[0], p.extracts[0], func(string) {}); e == nil {
		t.Fatal("cancelled extraction continued")
	}
}
