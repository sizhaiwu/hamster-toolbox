package toolbox

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRawExtractParts(t *testing.T, base string, data []byte) {
	t.Helper()
	mid := len(data) / 2
	writeArchiveFixture(t, base+".001", data[:mid])
	writeArchiveFixture(t, base+".002", data[mid:])
}

func TestExtractNumericSplitRecognition(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		name, format string
		header       []byte
	}{
		{"无类型.001", "7z", []byte{'7', 'z', 0xbc, 0xaf, 0x27, 0x1c, 0, 4}},
		{"zip split.001", "zip", []byte{'P', 'K', 3, 4, 0, 0, 0, 0}},
		{"empty.001", "zip", []byte{'P', 'K', 5, 6, 0, 0, 0, 0}},
		{"rar4.001", "rar", []byte{'R', 'a', 'r', '!', 0x1a, 7, 0, 0}},
		{"rar5.001", "rar", []byte{'R', 'a', 'r', '!', 0x1a, 7, 1, 0}},
		{"ordinary.001", "", []byte("ordinary file")},
		{"empty-file.001", "", nil},
	} {
		path := filepath.Join(root, tc.name)
		writeArchiveFixture(t, path, tc.header)
		format, err := extractSplitType(path)
		if err != nil || format != tc.format {
			t.Fatal(tc.name, format, err)
		}
		args, err := extractTypeOptions(path)
		if tc.format == "" {
			if err == nil {
				t.Fatal("ordinary file treated as archive")
			}
			continue
		}
		if err != nil || len(args) != 1 || args[0] != "-t"+tc.format+".split" {
			t.Fatal(args, err)
		}
	}
	for _, name := range []string{"sample.001", "sample.0001", "sample.7z.001"} {
		if _, ok := extractArchiveName(name); !ok {
			t.Fatal(name)
		}
	}
	for _, name := range []string{"sample.002", "sample.000", "sample.001.uploading.cfg", "sample.001.pdf", "sample.01"} {
		if _, ok := extractArchiveName(name); ok {
			t.Fatal("non-entry accepted", name)
		}
	}
}

func TestExtractNumericSplitIntegration(t *testing.T) {
	for _, format := range []string{"7z", "zip", "rar"} {
		t.Run(format, func(t *testing.T) {
			c := extractFixture(t)
			c.ExtractDeleteSource = true
			c.ExtractDeleteMode = "permanent"
			archive := filepath.Join(t.TempDir(), "source."+format)
			body := "RAR resource"
			if format == "rar" {
				writeExtractRAR(t, archive)
			} else {
				body = strings.Repeat("synthetic extraction content\n", 3000)
				if format == "zip" {
					writeExtractZIP(t, archive, map[string]string{"hello.txt": body})
				} else {
					input := filepath.Join(t.TempDir(), "hello.txt")
					writeArchiveFixture(t, input, []byte(body))
					if _, err := run(context.Background(), c.SevenZip, "a", "-t7z", "-mx=0", archive, input); err != nil {
						t.Fatal(err)
					}
				}
			}
			data, err := os.ReadFile(archive)
			if err != nil {
				t.Fatal(err)
			}
			base := filepath.Join(c.Root, "资源 -删除后缀.pdf后解压")
			writeRawExtractParts(t, base, data)
			writeArchiveFixture(t, base+".002.uploading.cfg", []byte("keep sidecar"))
			writeArchiveFixture(t, filepath.Join(c.Root, "ordinary.001"), []byte("not an archive"))
			p, err := BuildPlan(context.Background(), c, quiet)
			if err != nil || len(p.Items) != 1 || p.Items[0].Conflict != "" {
				t.Fatal(p, err)
			}
			job := p.extracts[0]
			if len(job.Volumes) != 2 || job.VolumeCount != 2 || len(job.Members) != 1 || job.Members[0].Name != "hello.txt" {
				t.Fatal("did not enter inner archive", job)
			}
			commands, err := NativeCommands(context.Background(), p.Config, p.Items[0])
			if err != nil || !strings.Contains(commands[0].PowerShell, "-t"+format+".split") {
				t.Fatal(commands, err)
			}
			posix, err := nativePOSIXCommands(p.Config, p.Items[0])
			if err != nil || !strings.Contains(posix[0].PowerShell, "-t"+format+".split") {
				t.Fatal(posix, err)
			}
			var logs []string
			r := Execute(context.Background(), p, func(s string) { logs = append(logs, s) }, progress)
			if r.Success != 1 || r.Failed != 0 {
				t.Fatal(r, logs)
			}
			actual, err := os.ReadFile(filepath.Join(p.Items[0].Target, "hello.txt"))
			if err != nil || string(actual) != body {
				t.Fatal("extracted content differs", err)
			}
			if regularFile(base+".001") || regularFile(base+".002") || !regularFile(base+".002.uploading.cfg") || !regularFile(filepath.Join(c.Root, "ordinary.001")) {
				t.Fatal("incorrect source cleanup")
			}
		})
	}
}

func TestExtractNumericEncryptedAndMissingVolumesIntegration(t *testing.T) {
	c := extractFixture(t)
	input := filepath.Join(t.TempDir(), "hello.txt")
	body := bytes.Repeat([]byte("encrypted split content"), 10000)
	writeArchiveFixture(t, input, body)
	archive := filepath.Join(t.TempDir(), "encrypted.7z")
	if _, err := run(context.Background(), c.SevenZip, "a", "-t7z", "-mx=0", "-pTestOnly!", "-mhe=on", archive, input); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(c.Root, "encrypted")
	writeRawExtractParts(t, base, data)
	for _, password := range []string{"", "wrong", "TestOnly!"} {
		c.ExtractPassword = password
		p, err := BuildPlan(context.Background(), c, quiet)
		if err != nil || len(p.Items) != 1 {
			t.Fatal(p, err)
		}
		if password == "" {
			if !strings.Contains(p.Items[0].Conflict, "填写解压密码") {
				t.Fatal(p.Items)
			}
			continue
		}
		if password == "wrong" {
			if !strings.Contains(p.Items[0].Conflict, "密码校验失败") {
				t.Fatal(p.Items)
			}
			continue
		}
		if p.Items[0].Conflict != "" {
			t.Fatal(p.Items)
		}
		r := Execute(context.Background(), p, quiet, progress)
		if r.Success != 1 {
			t.Fatal(r)
		}
		actual, err := os.ReadFile(filepath.Join(p.Items[0].Target, "hello.txt"))
		if err != nil || !bytes.Equal(actual, body) {
			t.Fatal("encrypted output differs", err)
		}
		if !regularFile(base+".001") || !regularFile(base+".002") {
			t.Fatal("default extraction deleted input")
		}
		p, err = BuildPlan(context.Background(), c, quiet)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.Remove(base + ".002"); err != nil {
			t.Fatal(err)
		}
		if r = Execute(context.Background(), p, quiet, progress); r.Failed != 1 {
			t.Fatal("changed volume accepted", r)
		}
		p, err = BuildPlan(context.Background(), c, quiet)
		if err != nil || len(p.Items) != 1 || p.Items[0].Conflict == "" {
			t.Fatal("missing volume not reported", p, err)
		}
	}
}

func TestExtractNumericSplitRejectsUnsafeNamesIntegration(t *testing.T) {
	c := extractFixture(t)
	archive := filepath.Join(t.TempDir(), "unsafe.zip")
	writeExtractZIP(t, archive, map[string]string{"../outside.txt": "bad"})
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(c.Root, "unsafe")
	writeRawExtractParts(t, base, data)
	p, err := BuildPlan(context.Background(), c, quiet)
	if err != nil || len(p.Items) != 1 || !strings.Contains(p.Items[0].Conflict, "不安全") {
		t.Fatal("unsafe inner archive accepted", p, err)
	}
}
