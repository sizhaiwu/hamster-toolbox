package toolbox

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractErrorMessages(t *testing.T) {
	for _, tc := range []struct{ diagnostic, password, source, want string }{
		{"Cannot open encrypted archive. Wrong password?", "", "a.7z", "填写解压密码"},
		{"Cannot open encrypted archive. Wrong password?", "incorrect", "a.7z", "密码校验失败"},
		{"ERROR: Wrong password : file.txt", "incorrect", "a.zip", "密码校验失败"},
		{"Data Error in encrypted file. Wrong password?", "incorrect", "a.7z", "密码校验失败或加密数据损坏"},
		{"Missing volume : a.7z.002", "", "a.7z.001", "缺少压缩分卷"},
		{"Unexpected end of archive\nHeaders Error", "", "a.7z.001", "分卷不完整"},
		{"Unexpected end of archive", "", "a.zip", "压缩包不完整"},
		{"CRC Failed : file.txt", "", "a.zip", "压缩包校验失败"},
		{"Unsupported Method", "", "a.7z", "不支持此压缩方式"},
		{"ERROR: wrong password.zip : Is not archive", "", "wrong password.zip", "不是可识别的压缩包"},
	} {
		e := classifyExtractError(Config{ExtractPassword: tc.password}, tc.source, tc.diagnostic)
		if e == nil || !strings.Contains(e.Error(), tc.want) {
			t.Fatalf("%q: %v", tc.diagnostic, e)
		}
	}
}

func TestExtractPasswordDiagnosticsIntegration(t *testing.T) {
	for _, kind := range []string{"7z-headers", "7z-content", "zip"} {
		t.Run(kind, func(t *testing.T) {
			c := extractFixture(t)
			source := filepath.Join(t.TempDir(), "file.txt")
			writeArchiveFixture(t, source, []byte("password test contents"))
			ext := "7z"
			if kind == "zip" {
				ext = "zip"
			}
			archive := filepath.Join(c.Root, "encrypted."+ext)
			args := []string{"a", "-t" + ext, "-pTestOnly!", archive, source}
			if kind == "7z-headers" {
				args = append(args, "-mhe=on")
			}
			if _, e := run(context.Background(), c.SevenZip, args...); e != nil {
				t.Fatal(e)
			}
			c.ExtractPassword = ""
			p, e := BuildPlan(context.Background(), c, quiet)
			if e != nil {
				t.Fatal(e)
			}
			if len(p.Items) != 1 || !strings.Contains(p.Items[0].Conflict, "填写解压密码") {
				t.Fatal("missing password not identified", p.Items)
			}
			c.ExtractPassword = "Wrong-Test-Only!"
			p, e = BuildPlan(context.Background(), c, quiet)
			if e != nil {
				t.Fatal(e)
			}
			if kind == "7z-headers" {
				if !strings.Contains(p.Items[0].Conflict, "密码校验失败") {
					t.Fatal("encrypted header error not identified", p.Items)
				}
			} else {
				if p.Items[0].Conflict != "" {
					t.Fatal("readable directory should remain fast to scan", p.Items)
				}
				var logs []string
				r := Execute(context.Background(), p, func(s string) { logs = append(logs, s) }, progress)
				if r.Failed != 1 || !strings.Contains(strings.Join(logs, "\n"), "密码校验失败") {
					t.Fatal("wrong content password not identified", r, logs)
				}
				if strings.Contains(strings.Join(logs, "\n"), c.ExtractPassword) {
					t.Fatal("password leaked")
				}
				if _, e = os.Stat(filepath.Join(p.Items[0].Target, "file.txt")); !os.IsNotExist(e) {
					t.Fatal("failed extraction published a file")
				}
			}
			c.ExtractPassword = "TestOnly!"
			p, e = BuildPlan(context.Background(), c, quiet)
			if e != nil || p.Items[0].Conflict != "" {
				t.Fatal(p, e)
			}
			r := Execute(context.Background(), p, quiet, progress)
			if r.Success != 1 {
				t.Fatal("correct password failed", r)
			}
		})
	}
}

func TestExtractMissingVolumeDiagnosticIntegration(t *testing.T) {
	c := extractFixture(t)
	source := filepath.Join(t.TempDir(), "file.bin")
	writeArchiveFixture(t, source, bytes.Repeat([]byte("split test"), 20000))
	base := filepath.Join(c.Root, "split.7z")
	if _, e := run(context.Background(), c.SevenZip, "a", "-t7z", "-mx=0", "-v64k", base, source); e != nil {
		t.Fatal(e)
	}
	if e := os.Remove(base + ".002"); e != nil {
		t.Fatal(e)
	}
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Items) != 1 || !strings.Contains(p.Items[0].Conflict, "分卷") || strings.Contains(p.Items[0].Conflict, "密码") {
		t.Fatal("missing volume misclassified", p.Items)
	}
}
