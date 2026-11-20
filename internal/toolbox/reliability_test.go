package toolbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

type cancelAfterRead struct {
	cancel context.CancelFunc
	done   bool
}

func (r *cancelAfterRead) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	n := copy(p, bytes.Repeat([]byte("x"), 128))
	r.cancel()
	return n, nil
}

func TestCancelledPublishCleansPartialOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	target := filepath.Join(t.TempDir(), "output.bin")
	e := publishReader(ctx, &cancelAfterRead{cancel: cancel}, target)
	if !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e = os.Stat(target); !os.IsNotExist(e) {
		t.Fatal("partial output remains", e)
	}
}

func TestChineseNamesUseWindowsCharacterLimit(t *testing.T) {
	count := 100
	if runtime.GOOS != "windows" {
		count = 80
	}
	if !validName(strings.Repeat("册", count)) {
		t.Fatal("valid long Chinese name rejected")
	}
	if validName(strings.Repeat("册", 256)) || validName(strings.Repeat("😀", 128)) {
		t.Fatal("overlong Windows name accepted")
	}
	root := t.TempDir()
	source := strings.Repeat("册", 90)
	if runtime.GOOS != "windows" {
		source = strings.Repeat("册", 60)
		if validName(strings.Repeat("册", 86)) {
			t.Fatal("overlong UTF-8 byte name accepted")
		}
	}
	file(t, filepath.Join(root, source, "image.jpg"), []byte("x"))
	c := DefaultConfig()
	c.Root = root
	c.Tool = "rename"
	c.Files = false
	c.Prefix = "更新-"
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil || len(p.Items) != 1 || p.Items[0].Conflict != "" {
		t.Fatal(p, e)
	}
	r := Execute(context.Background(), p, quiet, progress)
	if r.Success != 1 || r.Failed != 0 {
		t.Fatal(r)
	}
}

func TestStreamingPDFCrossReferences(t *testing.T) {
	root := t.TempDir()
	imagePath := filepath.Join(root, "cover.jpg")
	f, e := os.Create(imagePath)
	if e != nil {
		t.Fatal(e)
	}
	if e = jpeg.Encode(f, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil); e != nil {
		t.Fatal(e)
	}
	f.Close()
	out := filepath.Join(root, "index.pdf")
	if e = writePDF(out, []string{imagePath, imagePath}); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(out)
	if e != nil {
		t.Fatal(e)
	}
	s := string(b)
	if !strings.Contains(s, "/Count 2") {
		t.Fatal("page count incorrect")
	}
	marker := strings.LastIndex(s, "startxref\n")
	if marker < 0 {
		t.Fatal("missing startxref")
	}
	startLine := strings.Split(s[marker+10:], "\n")[0]
	offset, e := strconv.Atoi(startLine)
	if e != nil || offset < 0 || offset >= len(b) || !strings.HasPrefix(s[offset:], "xref\n") {
		t.Fatal("xref offset incorrect", offset, e)
	}
	lines := strings.Split(s[offset:], "\n")
	for id := 1; id <= 8; id++ {
		n, e := strconv.Atoi(strings.Fields(lines[id+2])[0])
		if e != nil || n < 0 || n >= len(b) || !strings.HasPrefix(s[n:], fmt.Sprintf("%d 0 obj\n", id)) {
			t.Fatal("object offset incorrect", id, n, e)
		}
	}
}
