package toolbox

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestExpandedExtractNames(t *testing.T) {
	for _, name := range []string{"a.TAR", "a.tar.gz", "a.TBZ2", "a.tar.xz", "a.tzst", "a.gz", "a.bz2", "a.zst", "a.cab", "a.iso", "a.wim", "a.deb", "a.rpm", "a.zipx"} {
		base, ok := extractArchiveName(name)
		if !ok || base != "a" {
			t.Fatalf("%s: %q %v", name, base, ok)
		}
	}
	for _, name := range []string{"a.mp4", "a.docx", "a.exe", "a.tar.gz.txt", "a.zip.002"} {
		if _, ok := extractArchiveName(name); ok {
			t.Fatal("unexpected batch archive", name)
		}
	}
}

func expandedTAR(t *testing.T, path, name, link string) {
	t.Helper()
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	h := &tar.Header{Name: name, Mode: 0600, Size: 7}
	if link != "" {
		h.Typeflag, h.Linkname, h.Size = tar.TypeSymlink, link, 0
	}
	if e := w.WriteHeader(h); e != nil {
		t.Fatal(e)
	}
	if link == "" {
		if _, e := w.Write([]byte("example")); e != nil {
			t.Fatal(e)
		}
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, b.Bytes(), 0600); e != nil {
		t.Fatal(e)
	}
}

func TestExpandedExtractIntegration(t *testing.T) {
	for _, tc := range []struct{ suffix, format string }{{".tar", ""}, {".tar.gz", "gzip"}, {".tgz", "gzip"}, {".tar.bz2", "bzip2"}, {".txz", "xz"}, {".tar.zst", "zstd"}} {
		t.Run(tc.suffix, func(t *testing.T) {
			c := extractFixture(t)
			tarPath := filepath.Join(t.TempDir(), "sample.tar")
			expandedTAR(t, tarPath, "资料/说明.txt", "")
			source := filepath.Join(c.Root, "资源"+tc.suffix)
			if tc.format == "" {
				data, _ := os.ReadFile(tarPath)
				writeArchiveFixture(t, source, data)
			} else if tc.format == "zstd" {
				data, err := os.ReadFile(tarPath)
				if err != nil {
					t.Fatal(err)
				}
				n := len(data)
				size := n - 256
				block := (n << 3) | 1
				frame := []byte{0x28, 0xb5, 0x2f, 0xfd, 0x60, byte(size), byte(size >> 8), byte(block), byte(block >> 8), byte(block >> 16)}
				writeArchiveFixture(t, source, append(frame, data...))
			} else {
				cmd := exec.Command(c.SevenZip, "a", "-t"+tc.format, source, tarPath)
				hideWindow(cmd)
				if out, e := cmd.CombinedOutput(); e != nil {
					t.Fatalf("create: %v %s", e, out)
				}
			}
			p, e := BuildPlan(context.Background(), c, quiet)
			if e != nil || len(p.Items) != 1 || p.Items[0].Conflict != "" {
				t.Fatalf("plan: %+v %v", p, e)
			}
			r := Execute(context.Background(), p, func(s string) { t.Log(s) }, progress)
			if r.Success != 1 || r.Failed != 0 {
				t.Fatal(r)
			}
			data, e := os.ReadFile(filepath.Join(p.Items[0].Target, "资料", "说明.txt"))
			if e != nil || string(data) != "example" {
				t.Fatal("inner TAR not extracted", string(data), e)
			}
			if !regularFile(source) {
				t.Fatal("source removed by default")
			}
		})
	}
}

func TestExpandedExtractRejectsUnsafeTARIntegration(t *testing.T) {
	for _, tc := range []struct{ name, link string }{{"../outside.txt", ""}, {"linked.txt", "../../outside"}} {
		c := extractFixture(t)
		source := filepath.Join(c.Root, "unsafe.tar")
		expandedTAR(t, source, tc.name, tc.link)
		p, e := BuildPlan(context.Background(), c, quiet)
		if e != nil || len(p.Items) != 1 || p.Items[0].Conflict == "" {
			t.Fatalf("unsafe TAR accepted: %+v %v", p, e)
		}
	}
}

func TestExpandedExtractSingleStreamsAndWIMIntegration(t *testing.T) {
	for _, format := range []string{"gzip", "bzip2", "xz", "lzma", "cab", "wim", "zip", "7z"} {
		t.Run(format, func(t *testing.T) {
			c := extractFixture(t)
			input := filepath.Join(t.TempDir(), "sample.txt")
			writeArchiveFixture(t, input, []byte("archive-format-check"))
			ext := map[string]string{"gzip": ".gz", "bzip2": ".bz2", "xz": ".xz", "lzma": ".lzma", "cab": ".cab", "wim": ".wim", "zip": ".zipx", "7z": ".7z删除"}[format]
			source := filepath.Join(c.Root, "sample"+ext)
			fixtures := map[string]string{"lzma": "5d00008000ffffffffffffffff00309c888a1c4180a9a7762fae6305195b354b7de71d1e5932ffff77540000", "cab": "4d5343460000000063000000000000002c0000000000000003010100010000000000000047000000010000001400000000000000000021000000200073616d706c652e747874000000000014001400617263686976652d666f726d61742d636865636b"}
			if value, ok := fixtures[format]; ok {
				data, e := hex.DecodeString(value)
				if e != nil {
					t.Fatal(e)
				}
				writeArchiveFixture(t, source, data)
			} else {
				cmd := exec.Command(c.SevenZip, "a", "-t"+format, source, input)
				hideWindow(cmd)
				if out, e := cmd.CombinedOutput(); e != nil {
					t.Fatalf("create %s: %v %s", format, e, out)
				}
			}
			p, e := BuildPlan(context.Background(), c, quiet)
			if e != nil || len(p.Items) != 1 || p.Items[0].Conflict != "" {
				t.Fatalf("plan: %+v %v", p, e)
			}
			if r := Execute(context.Background(), p, quiet, progress); r.Success != 1 || r.Failed != 0 {
				t.Fatal(r)
			}
			found := false
			filepath.WalkDir(p.Config.Output, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					t.Fatal(err)
				}
				if !d.IsDir() {
					data, e := os.ReadFile(path)
					if e != nil {
						t.Fatal(e)
					}
					if string(data) == "archive-format-check" {
						found = true
					}
				}
				return nil
			})
			if !found {
				t.Fatal("extracted content missing")
			}
		})
	}
}

func TestCompressedTARCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := runExtractCommand(ctx, Config{}, "sample.tgz", "l", "--", "sample.tgz")
	if err != context.Canceled {
		t.Fatal(err)
	}
}

func TestExpandedExtractWrappedSafetyIntegration(t *testing.T) {
	for _, tc := range []struct {
		name, link string
		corrupt    bool
	}{
		{"../outside.txt", "", false}, {"link", "../../outside", false}, {"sample.txt", "", true},
	} {
		c := extractFixture(t)
		inner := filepath.Join(t.TempDir(), "inner.tar")
		expandedTAR(t, inner, tc.name, tc.link)
		data, e := os.ReadFile(inner)
		if e != nil {
			t.Fatal(e)
		}
		var packed bytes.Buffer
		w := gzip.NewWriter(&packed)
		if _, e = w.Write(data); e != nil {
			t.Fatal(e)
		}
		if e = w.Close(); e != nil {
			t.Fatal(e)
		}
		compressed := packed.Bytes()
		if tc.corrupt {
			compressed = compressed[:len(compressed)-8]
		}
		writeArchiveFixture(t, filepath.Join(c.Root, "unsafe.tgz"), compressed)
		p, e := BuildPlan(context.Background(), c, quiet)
		if e != nil || len(p.Items) != 1 || p.Items[0].Conflict == "" {
			t.Fatalf("unsafe/corrupt wrapped TAR accepted: %+v %v", p, e)
		}
	}
}
