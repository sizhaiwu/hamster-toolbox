package toolbox

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestBrowserTextEncodingBackupAndConflict(t *testing.T) {
	for _, format := range []BrowserText{{Encoding: "UTF-8", Newline: "LF"}, {Encoding: "UTF-8", BOM: true, Newline: "CRLF"}, {Encoding: "UTF-16 LE", BOM: true, Newline: "CRLF"}, {Encoding: "UTF-16 BE", BOM: true, Newline: "CR"}} {
		t.Run(format.Encoding+format.Newline, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.txt")
			original, err := encodeBrowserText("旧内容\nhello 😀", format)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			read, err := BrowserReadText(path)
			if err != nil || read.Text != "旧内容\nhello 😀" {
				t.Fatal(read, err)
			}
			saved, backup, err := BrowserSaveText(context.Background(), path, read.Revision, "新内容\n<script>text</script>", t.TempDir())
			if err != nil || backup == "" || saved.Encoding != format.Encoding || saved.Newline != format.Newline {
				t.Fatal(saved, backup, err)
			}
			before, err := os.ReadFile(backup)
			if err != nil || !bytes.Equal(before, original) {
				t.Fatal("original backup not preserved", err)
			}
			after, err := os.ReadFile(path)
			want, _ := encodeBrowserText(saved.Text, format)
			if err != nil || !bytes.Equal(after, want) {
				t.Fatal("encoding or newline changed", err)
			}
			if _, _, err = BrowserSaveText(context.Background(), path, read.Revision, "stale", t.TempDir()); !errors.Is(err, ErrTextConflict) {
				t.Fatal(err)
			}
			if _, _, err = BrowserSaveText(context.Background(), path, saved.Revision, "再更新\n末行", t.TempDir()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBrowserTextRejectsUnsafeSaves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.txt")
	os.WriteFile(path, []byte("original"), 0600)
	original, _ := BrowserReadText(path)
	st, _ := os.Stat(path)
	os.WriteFile(path, []byte("modified"), 0600)
	os.Chtimes(path, st.ModTime(), st.ModTime())
	if _, _, err := BrowserSaveText(context.Background(), path, original.Revision, "lost update", t.TempDir()); !errors.Is(err, ErrTextConflict) {
		t.Fatal("same-size, same-time change not detected", err)
	}
	current, _ := BrowserReadText(path)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := BrowserSaveText(ctx, path, current.Revision, "cancelled", t.TempDir()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "modified" {
		t.Fatal("cancel changed source")
	}
	if _, _, err := BrowserSaveText(context.Background(), path, current.Revision, string(make([]byte, BrowserTextLimit+1)), t.TempDir()); err == nil {
		t.Fatal("oversize accepted")
	}
	os.Chmod(path, 0400)
	defer os.Chmod(path, 0600)
	if _, _, err := BrowserSaveText(context.Background(), path, current.Revision, "readonly", t.TempDir()); err == nil {
		t.Fatal("readonly file saved")
	}
}

func TestBrowserTextBinaryAndLinks(t *testing.T) {
	protected := filepath.Join(t.TempDir(), "private-data", "settings.json")
	os.MkdirAll(filepath.Dir(protected), 0700)
	os.WriteFile(protected, []byte("protected"), 0600)
	ProtectDataDirectory(filepath.Dir(protected))
	if _, _, err := BrowserSaveText(context.Background(), protected, textDigest([]byte("protected")), "bad", t.TempDir()); err == nil {
		t.Fatal("protected configuration saved")
	}
	for _, data := range [][]byte{{0, 1, 2}, {0xff, 0, 0xff}, {0xff, 0xfe, 0x00, 0xd8}, {0xfe, 0xff, 0xd8, 0x00}, bytes.Repeat([]byte("x"), BrowserTextLimit+1)} {
		path := filepath.Join(t.TempDir(), "payload.txt")
		os.WriteFile(path, data, 0600)
		if _, err := BrowserReadText(path); err == nil {
			t.Fatal("unsupported data accepted")
		}
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	os.WriteFile(target, []byte("keep"), 0600)
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err == nil {
		if _, _, err := BrowserSaveText(context.Background(), link, textDigest([]byte("keep")), "bad", t.TempDir()); err == nil {
			t.Fatal("symlink saved")
		}
		if data, _ := os.ReadFile(target); string(data) != "keep" {
			t.Fatal("link target changed")
		}
	}
}
