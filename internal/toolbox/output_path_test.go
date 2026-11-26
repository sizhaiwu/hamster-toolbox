package toolbox

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRelativeOutputUsesInputDirectory(t *testing.T) {
	root := t.TempDir()
	program, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	font := filepath.Join(t.TempDir(), "synthetic.ttf")
	if err := os.WriteFile(font, []byte("normalization-only font fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"image", "video", "exif", "pdf", "thumbnail", "archive", "extract"} {
		t.Run(tool, func(t *testing.T) {
			c := DefaultConfig()
			c.Tool, c.Root, c.Output = tool, root, filepath.Join("south", "nested")
			if tool == "pdf" {
				c.Output = filepath.Join("south", "index.pdf")
			}
			c.Magick, c.FFmpeg, c.FFprobe, c.Exiftool, c.SevenZip, c.Font = program, program, program, program, program, font
			want := filepath.Join(root, c.Output)
			normalized, err := Normalize(c)
			if err != nil || !sameExtractPath(normalized.Output, want) {
				t.Fatalf("relative output: got %q, want %q: %v", normalized.Output, want, err)
			}
			if _, err := os.Stat(want); !os.IsNotExist(err) {
				t.Fatalf("normalization must not create output: %v", err)
			}
			// Exported previews and retry configurations contain an absolute
			// output, so normalizing again must not append the input twice.
			again, err := Normalize(normalized)
			if err != nil || again.Output != normalized.Output {
				t.Fatalf("renormalized output changed: %q %v", again.Output, err)
			}
			c.Output = filepath.Join(t.TempDir(), "elsewhere")
			if tool == "pdf" {
				c.Output += ".pdf"
			}
			normalized, err = Normalize(c)
			if err != nil || normalized.Output != c.Output {
				t.Fatalf("absolute output changed: %q %v", normalized.Output, err)
			}
		})
	}
}

func TestOutputResolutionRetainsSafetyAndLabels(t *testing.T) {
	program, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c := DefaultConfig()
	c.Tool, c.Root, c.SevenZip = "extract", t.TempDir(), program
	for _, output := range []string{".", ".."} {
		c.Output = output
		if _, err := Normalize(c); err == nil || !strings.Contains(err.Error(), "输出") {
			t.Fatalf("accepted input/ancestor as relative output %q: %v", output, err)
		}
	}
	for _, output := range []string{filepath.Dir(program), systemDirectory()} {
		c.Output = output
		if _, err := Normalize(c); err == nil || !strings.Contains(err.Error(), "输出路径") || !strings.Contains(err.Error(), "安全拦截") {
			t.Fatalf("protected output not identified %q: %v", output, err)
		}
	}
	if runtime.GOOS == "windows" {
		for _, output := range []string{`E:south`, `\south`} {
			c.Output = output
			if _, err := Normalize(c); err == nil || !strings.Contains(err.Error(), "输出路径") {
				t.Fatalf("ambiguous Windows output accepted %q: %v", output, err)
			}
		}
	}
	c.Root, c.Output = filepath.Dir(program), ""
	if _, err := Normalize(c); err == nil || !strings.Contains(err.Error(), "输入路径") || !strings.Contains(err.Error(), "安全拦截") {
		t.Fatalf("protected input not identified: %v", err)
	}
}
