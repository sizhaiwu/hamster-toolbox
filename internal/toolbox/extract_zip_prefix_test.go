package toolbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func writePrefixedZIPFixture(t *testing.T, path string, data []byte, prefix int64, trailer bool) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err = f.Write([]byte("\x00\x00\x00\x20ftypisom synthetic cover")); err != nil {
		t.Fatal(err)
	}
	if _, err = f.Seek(prefix, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write(data); err != nil {
		t.Fatal(err)
	}
	if trailer {
		if _, err = f.Write(append([]byte("Rar!\x1a\x07"), make([]byte, 12<<10)...)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestExtractPrefixedZIPRecognition(t *testing.T) {
	root := t.TempDir()
	plain := filepath.Join(root, "plain.zip")
	writeExtractZIP(t, plain, map[string]string{"hello.txt": "synthetic resource"})
	data, err := os.ReadFile(plain)
	if err != nil {
		t.Fatal(err)
	}
	const prefix = int64(9<<20 + 123)
	for _, trailer := range []bool{false, true} {
		path := filepath.Join(root, "covered-"+strconv.FormatBool(trailer)+".ZIP")
		writePrefixedZIPFixture(t, path, data, prefix, trailer)
		args, err := extractTypeOptions(path)
		if err != nil || len(args) != 1 || args[0] != "-tzip:s"+strconv.FormatInt(prefix+1, 10)+"b" {
			t.Fatal(args, err)
		}
	}
	if args, err := extractTypeOptions(plain); err != nil || len(args) != 0 {
		t.Fatal("ordinary ZIP changed", args, err)
	}
	for _, kind := range []string{"bad-central", "bad-local", "split", "zip64", "fake-footer"} {
		t.Run(kind, func(t *testing.T) {
			broken := append([]byte(nil), data...)
			at := bytes.LastIndex(broken, []byte{'P', 'K', 5, 6})
			switch kind {
			case "bad-central":
				central := bytes.Index(broken, []byte{'P', 'K', 1, 2})
				broken[central] = 0
			case "bad-local":
				broken[0] = 0
			case "split":
				binary.LittleEndian.PutUint16(broken[at+4:], 1)
			case "zip64":
				binary.LittleEndian.PutUint32(broken[at+16:], 0xffffffff)
			case "fake-footer":
				broken = append([]byte("fake header data"), broken[at:]...)
			}
			path := filepath.Join(root, kind+".zip")
			writePrefixedZIPFixture(t, path, broken, prefix, false)
			if args, err := extractTypeOptions(path); err != nil || len(args) != 0 {
				t.Fatal("unverified footer expanded scan", args, err)
			}
		})
	}
}

func TestExtractPrefixedZIPIntegration(t *testing.T) {
	for _, encrypted := range []bool{false, true} {
		t.Run(strconv.FormatBool(encrypted), func(t *testing.T) {
			c := extractFixture(t)
			archive := filepath.Join(t.TempDir(), "original.zip")
			body := "synthetic prefixed ZIP resource"
			if encrypted {
				input := filepath.Join(t.TempDir(), "hello.txt")
				writeArchiveFixture(t, input, []byte(body))
				c.ExtractPassword = "SyntheticOnly!"
				if _, err := run(context.Background(), c.SevenZip, "a", "-tzip", "-mem=AES256", "-p"+c.ExtractPassword, archive, input); err != nil {
					t.Fatal(err)
				}
			} else {
				writeExtractZIP(t, archive, map[string]string{"hello.txt": body})
			}
			data, err := os.ReadFile(archive)
			if err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(c.Root, "covered.zip")
			writePrefixedZIPFixture(t, source, data, 9<<20+123, true)
			before, _ := os.ReadFile(source)
			if encrypted {
				missing := c
				missing.ExtractPassword = ""
				plan, err := BuildPlan(context.Background(), missing, quiet)
				if err != nil || len(plan.Items) != 1 || !strings.Contains(plan.Items[0].Conflict, "已加密") {
					t.Fatal("missing password not identified", plan, err)
				}
			}
			plan, err := BuildPlan(context.Background(), c, quiet)
			if err != nil || len(plan.Items) != 1 || plan.Items[0].Conflict != "" {
				t.Fatal(plan, err)
			}
			commands, err := NativeCommands(context.Background(), plan.Config, plan.Items[0])
			if err != nil || len(commands) == 0 || !strings.Contains(commands[0].PowerShell, "-tzip:s") {
				t.Fatal("native command missing prefix compatibility", commands, err)
			}
			posix, err := nativePOSIXCommands(plan.Config, plan.Items[0])
			if err != nil || !strings.Contains(posix[0].PowerShell, "-tzip:s") {
				t.Fatal("POSIX command missing prefix compatibility", posix, err)
			}
			result := Execute(context.Background(), plan, quiet, progress)
			actual, err := os.ReadFile(filepath.Join(plan.Items[0].Target, "hello.txt"))
			if result.Success != 1 || result.Failed != 0 || err != nil || string(actual) != body {
				t.Fatal("prefixed ZIP extraction failed", result, err)
			}
			after, err := os.ReadFile(source)
			if err != nil || sha256.Sum256(before) != sha256.Sum256(after) {
				t.Fatal("original archive changed", err)
			}
		})
	}
}

func TestExtractPrefixedZIPTraversalIntegration(t *testing.T) {
	c := extractFixture(t)
	plain := filepath.Join(t.TempDir(), "unsafe.zip")
	writeExtractZIP(t, plain, map[string]string{"../outside.txt": "must never escape"})
	data, err := os.ReadFile(plain)
	if err != nil {
		t.Fatal(err)
	}
	writePrefixedZIPFixture(t, filepath.Join(c.Root, "covered.zip"), data, 9<<20+123, true)
	plan, err := BuildPlan(context.Background(), c, quiet)
	if err != nil || len(plan.Items) != 1 || !strings.Contains(plan.Items[0].Conflict, "不安全") {
		t.Fatal("prefix compatibility bypassed member path validation", plan, err)
	}
}
