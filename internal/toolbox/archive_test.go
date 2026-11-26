package toolbox

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func archiveFixture(t *testing.T) Config {
	t.Helper()
	c := DefaultConfig()
	c.Tool = "archive"
	c.Root = t.TempDir()
	c.SevenZip = ""
	// Dependency validation only needs a real executable; integration below uses 7-Zip.
	c.SevenZip, _ = os.Executable()
	return c
}

func writeArchiveFixture(t *testing.T, path string, data []byte) {
	t.Helper()
	if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, data, 0600); e != nil {
		t.Fatal(e)
	}
}

func TestArchiveIndependentStartingNumbers(t *testing.T) {
	for _, tc := range []struct {
		name                                   string
		dirStart, fileStart, wantDir, wantFile int
	}{
		{"legacy", 0, 0, 29, 29},
		{"independent", 99, 3, 99, 3},
		{"directory override", 99, 0, 99, 29},
		{"file override", 0, 3, 29, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := archiveFixture(t)
			c.ArchiveStart, c.ArchiveDirStart, c.ArchiveFileStart = 29, tc.dirStart, tc.fileStart
			for _, name := range []string{"album2/item.txt", "album10/item.txt", "file2.txt", "file10.txt"} {
				writeArchiveFixture(t, filepath.Join(c.Root, name), []byte("fixture"))
			}
			p, err := BuildPlan(context.Background(), ScopeConfig(c), func(string) {})
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]string{
				"album2":     fmt.Sprintf("D_%03d.7z.001", tc.wantDir),
				"album10":    fmt.Sprintf("D_%03d.7z.001", tc.wantDir+1),
				"file2.txt":  fmt.Sprintf("F_%03d.7z.001", tc.wantFile),
				"file10.txt": fmt.Sprintf("F_%03d.7z.001", tc.wantFile+1),
			}
			if len(p.Items) != len(want) {
				t.Fatal(p.Items)
			}
			for _, item := range p.Items {
				if item.Conflict != "" || filepath.Base(item.Target) != want[filepath.Base(item.Source)] {
					t.Fatal(item)
				}
			}
		})
	}
	for _, field := range []string{"dir", "file"} {
		for _, value := range []int{-1, 1000000000} {
			c := archiveFixture(t)
			if field == "dir" {
				c.ArchiveDirStart = value
			} else {
				c.ArchiveFileStart = value
			}
			if _, err := BuildPlan(context.Background(), c, func(string) {}); err == nil {
				t.Fatalf("accepted invalid %s start %d", field, value)
			}
		}
	}
}

func TestArchiveIndependentStartingNumbersIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") == "" {
		t.Skip("set TOOLBOX_INTEGRATION=1")
	}
	c := archiveFixture(t)
	c.SevenZip, _ = filepath.Abs(filepath.Join("..", "..", "tools", "sevenzip", "7za.exe"))
	if _, err := os.Stat(c.SevenZip); err != nil {
		t.Skip("bundled 7-Zip unavailable")
	}
	c.ArchiveDirStart, c.ArchiveFileStart, c.ArchiveSplit = 99, 3, "none"
	writeArchiveFixture(t, filepath.Join(c.Root, "album", "inside.txt"), []byte("directory fixture"))
	writeArchiveFixture(t, filepath.Join(c.Root, "item.txt"), []byte("file fixture"))
	p, err := BuildPlan(context.Background(), c, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	result := Execute(context.Background(), p, func(message string) { t.Log(message) }, func(int, int) {})
	if result.Success != 2 || result.Failed != 0 {
		t.Fatal(result)
	}
	for _, name := range []string{"D_099.7z", "F_003.7z"} {
		if _, err := run(context.Background(), c.SevenZip, "t", filepath.Join(p.Config.Output, name)); err != nil {
			t.Fatal(err)
		}
	}
	for name, want := range map[string]string{"album/inside.txt": "directory fixture", "item.txt": "file fixture"} {
		data, err := os.ReadFile(filepath.Join(c.Root, name))
		if err != nil || string(data) != want {
			t.Fatal("source changed", name, err)
		}
	}
}

func TestArchivePlanNumberingAndConflicts(t *testing.T) {
	c := archiveFixture(t)
	c.ArchiveStart = 99
	for _, name := range []string{"目录10/资源.txt", "目录2/资源.txt", "file10.txt", "file2.txt", "update.bat", "old.7z.001", "0utput_7z/old.txt", "toolbox-output/other/result.txt"} {
		writeArchiveFixture(t, filepath.Join(c.Root, name), []byte("test"))
	}
	p, e := BuildPlan(context.Background(), c, func(string) {})
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Items) != 4 {
		t.Fatalf("unexpected items: %+v", p.Items)
	}
	if p.Config.Output != filepath.Join(c.Root, "0utput_7z") {
		t.Fatalf("legacy default output changed: %s", p.Config.Output)
	}
	got := map[string]string{}
	for _, it := range p.Items {
		got[filepath.Base(it.Source)] = filepath.Base(it.Target)
		if it.Conflict != "" {
			t.Fatal(it)
		}
	}
	for source, want := range map[string]string{"目录2": "D_099.7z.001", "目录10": "D_100.7z.001", "file2.txt": "F_099.7z.001", "file10.txt": "F_100.7z.001"} {
		if got[source] != want {
			t.Fatalf("%s: %s", source, got[source])
		}
	}
	writeArchiveFixture(t, filepath.Join(p.Config.Output, "D_099.7z.002"), []byte("existing later volume"))
	p, e = BuildPlan(context.Background(), c, func(string) {})
	if e != nil {
		t.Fatal(e)
	}
	for _, it := range p.Items {
		if filepath.Base(it.Source) == "目录2" && it.Conflict == "" {
			t.Fatal("orphan later volume must block")
		}
	}
	c.ArchiveFilePrefix = c.ArchiveDirPrefix
	p, e = BuildPlan(context.Background(), c, func(string) {})
	if e != nil {
		t.Fatal(e)
	}
	for _, it := range p.Items {
		if it.Conflict == "" {
			t.Fatal("both duplicate targets must conflict")
		}
	}
}

func TestArchiveExtrasAndStaleSource(t *testing.T) {
	c := archiveFixture(t)
	c.ArchiveMode = "extras"
	source := filepath.Join(c.Root, "资源!& [1]/empty")
	if e := os.MkdirAll(source, 0755); e != nil {
		t.Fatal(e)
	}
	extra := filepath.Join(t.TempDir(), "说明.txt")
	writeArchiveFixture(t, extra, []byte("instructions"))
	c.ArchiveExtras = extra
	p, e := BuildPlan(context.Background(), c, func(string) {})
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Items) != 1 || p.Items[0].Conflict != "" {
		t.Fatal(p.Items)
	}
	if e = checkArchiveSources(context.Background(), p.Items[0], p.archives[0]); e != nil {
		t.Fatal(e)
	}
	writeArchiveFixture(t, filepath.Join(source, "new.txt"), []byte("new after preview"))
	if e = checkArchiveSources(context.Background(), p.Items[0], p.archives[0]); e == nil {
		t.Fatal("new resource member must invalidate preview")
	}
	c.Output = filepath.Join(source, "out")
	p, e = BuildPlan(context.Background(), c, func(string) {})
	if e != nil {
		t.Fatal(e)
	}
	if p.Items[0].Conflict == "" {
		t.Fatal("output inside resource must block")
	}
	c.Output = ""
	extra2 := filepath.Join(t.TempDir(), "说明.txt")
	writeArchiveFixture(t, extra2, []byte("other"))
	c.ArchiveExtras += "\n" + extra2
	if _, e = BuildPlan(context.Background(), c, func(string) {}); e == nil {
		t.Fatal("duplicate extra names must block")
	}
}

func TestArchiveReadOnlyExtrasAllowProtectedLocations(t *testing.T) {
	c := archiveFixture(t)
	c.ArchiveMode = "extras"
	writeArchiveFixture(t, filepath.Join(c.Root, "resource.txt"), []byte("resource"))
	private := filepath.Join(t.TempDir(), "read-only-inputs")
	extra := filepath.Join(private, "instructions.html")
	writeArchiveFixture(t, extra, []byte("<p>read only</p>"))
	ProtectDataDirectory(private)
	if _, e := safePath(extra); e == nil {
		t.Fatal("write protection was removed")
	}
	c.ArchiveExtras = extra
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil || len(p.Items) != 1 || p.Items[0].Conflict != "" {
		t.Fatalf("explicit read-only extra rejected: %v %+v", e, p)
	}
	if e := checkArchiveSources(context.Background(), p.Items[0], p.archives[0]); e != nil {
		t.Fatal(e)
	}
	payload := t.TempDir()
	if e := stageArchive(context.Background(), payload, p.archives[0]); e != nil {
		t.Fatal(e)
	}
	staged := filepath.Join(payload, filepath.Base(extra))
	originalInfo, _ := os.Stat(extra)
	stagedInfo, _ := os.Stat(staged)
	if os.SameFile(originalInfo, stagedInfo) {
		t.Fatal("read-only extra was hard-linked instead of copied")
	}
	writeArchiveFixture(t, staged, []byte("stage changed"))
	if original, e := os.ReadFile(extra); e != nil || string(original) != "<p>read only</p>" {
		t.Fatal("extra source changed", e)
	}
	if _, e := Normalize(Config{Tool: "rename", Root: private, Files: true, Prefix: "new-"}); e == nil {
		t.Fatal("mutation protection relaxed for extra directory")
	}
}

func TestArchiveExtrasRejectSameMetadataReplacement(t *testing.T) {
	c := archiveFixture(t)
	c.ArchiveMode = "extras"
	writeArchiveFixture(t, filepath.Join(c.Root, "resource.txt"), []byte("resource"))
	extra := filepath.Join(t.TempDir(), "info.txt")
	writeArchiveFixture(t, extra, []byte("original"))
	c.ArchiveExtras = extra
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil {
		t.Fatal(e)
	}
	info, e := os.Stat(extra)
	if e != nil {
		t.Fatal(e)
	}
	if e := os.Rename(extra, filepath.Join(filepath.Dir(extra), "saved.txt")); e != nil {
		t.Fatal(e)
	}
	writeArchiveFixture(t, extra, []byte("replaced"))
	if e := os.Chtimes(extra, info.ModTime(), info.ModTime()); e != nil {
		t.Fatal(e)
	}
	if e := checkArchiveSources(context.Background(), p.Items[0], p.archives[0]); e == nil {
		t.Fatal("same-size/time replacement accepted")
	}
}

func TestArchiveExtrasStillRejectDirectories(t *testing.T) {
	c := archiveFixture(t)
	c.ArchiveMode, c.ArchiveExtras = "extras", t.TempDir()
	if _, e := archiveExtraPaths(c); e == nil {
		t.Fatal("directory accepted as extra")
	}
}

func TestArchiveArgsAndScope(t *testing.T) {
	c := archiveFixture(t)
	c.ArchivePassword = "a b!&中文"
	c.ArchiveMode = "extras"
	c.ArchiveMarker = true
	c.ArchiveExtras = "info.txt"
	args := archiveArgs(c, "archive.7z", false)
	joined := strings.Join(args, "|")
	for _, want := range []string{"-p" + c.ArchivePassword, "-mhe=on", "-v10k|-v3g", "--|archive.7z|."} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q", want)
		}
	}
	c.Tool = "format"
	if got := ScopeConfig(c); got.ArchivePassword != "" || got.SevenZip != "" || got.ArchiveMarker {
		t.Fatal("archive options leaked")
	}
	c.Tool = "archive"
	c.ArchiveSplit = "none"
	if strings.Contains(strings.Join(archiveArgs(c, "archive.7z", false), "|"), "-v") {
		t.Fatal("unexpected volume option")
	}
}

func TestArchiveStagingPreservesMetadataAndReadonlyOriginal(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "资源")
	file := filepath.Join(source, "photo.bin")
	empty := filepath.Join(source, "空目录")
	writeArchiveFixture(t, file, []byte("original"))
	if e := os.Mkdir(empty, 0755); e != nil {
		t.Fatal(e)
	}
	modified := time.Date(2020, 2, 3, 4, 5, 6, 0, time.Local)
	for _, p := range []string{file, empty, source} {
		if e := os.Chtimes(p, modified, modified); e != nil {
			t.Fatal(e)
		}
	}
	// Force the cross-volume copy path directly, including timestamp preservation.
	st, e := os.Stat(file)
	if e != nil {
		t.Fatal(e)
	}
	copyPath := filepath.Join(t.TempDir(), "copied.bin")
	if e = copyArchiveFile(context.Background(), archiveEntry{Source: file, Size: st.Size(), Modified: st.ModTime()}, copyPath); e != nil {
		t.Fatal(e)
	}
	st, e = os.Stat(copyPath)
	if e != nil || !st.ModTime().Equal(modified) {
		t.Fatal("copy changed timestamp", e)
	}
	if e = os.Chmod(file, 0444); e != nil {
		t.Fatal(e)
	}
	defer os.Chmod(file, 0600)
	entries, e := archiveSnapshot(context.Background(), source)
	if e != nil {
		t.Fatal(e)
	}
	payload := t.TempDir()
	if e = stageArchive(context.Background(), payload, archiveJob{Entries: entries}); e != nil {
		t.Fatal(e)
	}
	for _, a := range entries {
		st, e = os.Stat(filepath.Join(payload, a.Name))
		if e != nil || !st.ModTime().Equal(a.Modified) {
			t.Fatal("staged modification time differs", a.Name, e)
		}
	}
	if runtime.GOOS == "windows" {
		a, _ := os.Stat(file)
		b, _ := os.Stat(filepath.Join(payload, "资源", "photo.bin"))
		if os.SameFile(a, b) {
			t.Fatal("readonly source was hard-linked")
		}
	}
	if e = os.RemoveAll(payload); e != nil {
		t.Fatal(e)
	}
	st, e = os.Stat(file)
	if e != nil || st.Mode().Perm()&0200 != 0 {
		t.Fatal("cleanup changed readonly original", e)
	}
}

func archiveTestListing(raw []byte) map[string]map[string]string {
	result := map[string]map[string]string{}
	for _, block := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n\n") {
		fields := map[string]string{}
		for _, line := range strings.Split(block, "\n") {
			if k, v, ok := strings.Cut(line, " = "); ok {
				fields[k] = v
			}
		}
		if _, ok := fields["CRC"]; ok && fields["Path"] != "" {
			result[fields["Path"]] = fields
		}
	}
	return result
}

func TestArchiveIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") == "" {
		t.Skip("set TOOLBOX_INTEGRATION=1")
	}
	exe, e := filepath.Abs(filepath.Join("..", "..", "tools", "sevenzip", "7za.exe"))
	if e != nil {
		t.Fatal(e)
	}
	if !regularFile(exe) {
		t.Fatal("bundled 7-Zip missing")
	}
	for _, split := range []string{"script", "uniform", "none"} {
		t.Run(split, func(t *testing.T) {
			c := archiveFixture(t)
			c.SevenZip = exe
			c.ArchiveMode = "extras"
			c.ArchiveMarker = true
			c.ArchivePassword = "test !& 中文"
			c.ArchiveSplit = split
			c.ArchiveVolume = "16k"
			data := make([]byte, 70000)
			if _, e = rand.Read(data); e != nil {
				t.Fatal(e)
			}
			resource := filepath.Join(c.Root, "资源!& [1]", "-picture.bin")
			writeArchiveFixture(t, resource, data)
			if e = os.Mkdir(filepath.Join(filepath.Dir(resource), "空目录"), 0755); e != nil {
				t.Fatal(e)
			}
			extra := filepath.Join(t.TempDir(), "说明.txt")
			writeArchiveFixture(t, extra, []byte("extra contents"))
			c.ArchiveExtras = extra
			writeArchiveFixture(t, filepath.Join(c.Root, "单个!&.txt"), []byte("single"))
			p, e := BuildPlan(context.Background(), c, func(string) {})
			if e != nil {
				t.Fatal(e)
			}
			if len(p.Items) != 2 {
				t.Fatal(p.Items)
			}
			var logs []string
			r := Execute(context.Background(), p, func(s string) { logs = append(logs, s) }, func(int, int) {})
			if r.Success != 2 || r.Failed != 0 {
				t.Fatalf("%+v\n%s", r, strings.Join(logs, "\n"))
			}
			if strings.Contains(strings.Join(logs, "\n"), c.ArchivePassword) {
				t.Fatal("password in logs")
			}
			var marker []byte
			for _, it := range p.Items {
				b, e := run(context.Background(), exe, "x", "-so", "-p"+c.ArchivePassword, it.Target, ".packing_date")
				if e != nil {
					t.Fatal(e)
				}
				if !strings.HasPrefix(string(b), "压缩时间：") || !strings.HasSuffix(string(b), "\r\n") {
					t.Fatal("unexpected marker format")
				}
				if marker != nil && string(marker) != string(b) {
					t.Fatal("batch marker differs between archives")
				}
				marker = b
				listing, e := run(context.Background(), exe, "l", "-slt", "-sccUTF-8", "-p"+c.ArchivePassword, it.Target)
				if e != nil {
					t.Fatal(e)
				}
				if runtime.GOOS == "windows" && archiveTestListing(listing)[".packing_date"]["Attributes"] != "HSA" {
					t.Fatal("marker is not hidden+system")
				}
			}
			var dirItem Item
			for _, it := range p.Items {
				if strings.HasPrefix(filepath.Base(it.Target), "D_") {
					dirItem = it
				}
			}
			if split == "script" {
				st, e := os.Stat(dirItem.Target)
				if e != nil || st.Size() != 10240 {
					t.Fatalf("first volume size: %v %v", st, e)
				}
				st, e = os.Stat(strings.TrimSuffix(dirItem.Target, "001") + "002")
				if e != nil || st.Size() != 16384 {
					t.Fatalf("second volume size: %v %v", st, e)
				}
			}
			if _, e = run(context.Background(), exe, "t", "-pwrong", "-bso0", dirItem.Target); e == nil {
				t.Fatal("wrong password accepted")
			}
			listing, e := run(context.Background(), exe, "l", "-slt", "-sccUTF-8", "-p"+c.ArchivePassword, dirItem.Target)
			if e != nil {
				t.Fatal(e)
			}
			for _, want := range []string{"-picture.bin", "说明.txt", ".packing_date", "空目录"} {
				if !strings.Contains(string(listing), want) {
					t.Fatalf("missing %s:\n%s", want, listing)
				}
			}
			dest := t.TempDir()
			if _, e = run(context.Background(), exe, "x", "-y", "-p"+c.ArchivePassword, "-o"+dest, dirItem.Target); e != nil {
				t.Fatal(e)
			}
			out, e := os.ReadFile(filepath.Join(dest, "资源!& [1]", "-picture.bin"))
			if e != nil || string(out) != string(data) {
				t.Fatal("resource content changed", e)
			}
			out, e = os.ReadFile(resource)
			if e != nil || string(out) != string(data) {
				t.Fatal("original changed", e)
			}
			files, e := os.ReadDir(p.Config.Output)
			if e != nil {
				t.Fatal(e)
			}
			maps := 0
			for _, d := range files {
				if strings.HasPrefix(d.Name(), ".packing-") {
					t.Fatal("temporary stage left")
				}
				if d.Name() == "目录.txt" {
					maps++
					b, _ := os.ReadFile(filepath.Join(p.Config.Output, d.Name()))
					if !strings.Contains(string(b), "D_001.7z 目录：资源!& [1]\r\n") || !strings.Contains(string(b), "F_001.7z 文件：单个!&.txt\r\n") || strings.Contains(string(b), c.ArchivePassword) {
						t.Fatal("bad mapping")
					}
				}
			}
			if maps != 1 {
				t.Fatal("mapping missing")
			}
			p, e = BuildPlan(context.Background(), c, func(string) {})
			if e != nil {
				t.Fatal(e)
			}
			for _, it := range p.Items {
				if it.Conflict == "" {
					t.Fatal("existing archives must not be updated")
				}
			}
		})
	}
}

func TestArchiveLegacyManifestAppendIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") == "" {
		t.Skip("set TOOLBOX_INTEGRATION=1")
	}
	c := archiveFixture(t)
	c.SevenZip, _ = filepath.Abs(filepath.Join("..", "..", "tools", "sevenzip", "7za.exe"))
	writeArchiveFixture(t, filepath.Join(c.Root, "新资源.txt"), []byte("resource"))
	index := filepath.Join(c.Root, "0utput_7z", "目录.txt")
	want := "D_001.7z 目录：原有资源\r\n"
	writeArchiveFixture(t, index, []byte(want))
	for _, start := range []int{1, 2} {
		c.ArchiveStart = start
		p, e := BuildPlan(context.Background(), c, func(string) {})
		if e != nil {
			t.Fatal(e)
		}
		if len(p.Items) != 1 || p.Items[0].Conflict != "" {
			t.Fatal(p.Items)
		}
		r := Execute(context.Background(), p, func(string) {}, func(int, int) {})
		if r.Success != 1 {
			t.Fatal(r)
		}
		want += fmt.Sprintf("F_%03d.7z 文件：新资源.txt\r\n", start)
		b, e := os.ReadFile(index)
		if e != nil || string(b) != want {
			t.Fatalf("manifest was not appended: %q (%v)", b, e)
		}
	}
	// A failed update must retain the previous fixed-name manifest unchanged.
	c.ArchiveStart = 3
	p, e := BuildPlan(context.Background(), c, func(string) {})
	if e != nil {
		t.Fatal(e)
	}
	p.Config.SevenZip = filepath.Join(t.TempDir(), "missing.exe")
	r := Execute(context.Background(), p, func(string) {}, func(int, int) {})
	if r.Success != 0 || r.Failed != 1 {
		t.Fatal(r)
	}
	b, e := os.ReadFile(index)
	if e != nil || string(b) != want {
		t.Fatalf("failure changed prior manifest: %q (%v)", b, e)
	}
}

func TestArchiveFailureAndCancellationIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") == "" {
		t.Skip("set TOOLBOX_INTEGRATION=1")
	}
	exe, _ := filepath.Abs(filepath.Join("..", "..", "tools", "sevenzip", "7za.exe"))
	for _, mode := range []string{"pure", "cancel", "collision", "stale", "bad-program"} {
		t.Run(mode, func(t *testing.T) {
			c := archiveFixture(t)
			c.SevenZip = exe
			c.ArchiveSplit = "none"
			// Pure mode must ignore accidentally retained extra-file and marker options.
			c.ArchiveExtras = "this file does not exist"
			c.ArchiveMarker = true
			path := filepath.Join(c.Root, "资源.txt")
			writeArchiveFixture(t, path, []byte("original"))
			p, e := BuildPlan(context.Background(), c, func(string) {})
			if e != nil {
				t.Fatal(e)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "stale" {
				writeArchiveFixture(t, path, []byte("changed after preview"))
			}
			if mode == "bad-program" {
				p.Config.SevenZip = filepath.Join(t.TempDir(), "missing.exe")
			}
			r := Execute(ctx, p, func(s string) {
				if strings.HasPrefix(s, "正在校验：") {
					if mode == "cancel" {
						cancel()
					}
					if mode == "collision" {
						writeArchiveFixture(t, p.Items[0].Target, []byte("existing"))
					}
				}
			}, func(int, int) {})
			if mode == "pure" {
				if r.Success != 1 {
					t.Fatal(r)
				}
				out, e := run(ctx, exe, "l", "-slt", "-sccUTF-8", p.Items[0].Target)
				if e != nil {
					t.Fatal(e)
				}
				if strings.Contains(string(out), ".packing_date") {
					t.Fatal("pure archive contains marker")
				}
			} else {
				if r.Success != 0 {
					t.Fatal("failed task reported success", r)
				}
				if mode == "collision" {
					b, e := os.ReadFile(p.Items[0].Target)
					if e != nil || string(b) != "existing" {
						t.Fatal("existing output changed", e)
					}
				} else if _, e := os.Stat(p.Items[0].Target); !os.IsNotExist(e) {
					t.Fatal("failed archive published", e)
				}
				entries, e := os.ReadDir(p.Config.Output)
				if e != nil {
					t.Fatal(e)
				}
				for _, d := range entries {
					if strings.HasPrefix(d.Name(), ".packing-") || d.Name() == "目录.txt" {
						t.Fatal("failed task left temporary output", d.Name())
					}
				}
			}
		})
	}
}
