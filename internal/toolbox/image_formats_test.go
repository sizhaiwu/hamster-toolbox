package toolbox

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestImageFormatSelectionInMixedDirectory(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"a.jpg", "b.JPEG", "c.png", "nested/d.PNG", "e.webp", "old.gif", "old.bmp", "old.tiff", "old.avif", "old.heic", "old.svg", "notes.txt", "toolbox-output/image/ignored.png"} {
		file(t, filepath.Join(root, n), []byte("fixture"))
	}
	executable, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		formats []string
		want    int
		allowed string
	}{
		{nil, 2, ".jpg .jpeg"}, {[]string{".png"}, 2, ".png"}, {[]string{" PNG ", "webp", ".PNG"}, 3, ".png .webp"}, {strings.Fields(ImageCompressionFormats), 5, ImageCompressionFormats},
	} {
		c := DefaultConfig()
		c.Tool = "image"
		c.Root = root
		c.Magick = executable
		c.ImageFormats = tc.formats
		p, e := BuildPlan(context.Background(), c, quiet)
		if e != nil || len(p.Items) != tc.want {
			t.Fatal(tc, p, e)
		}
		for _, it := range p.Items {
			if !has(it.Source, tc.allowed) {
				t.Fatal("unselected format entered plan", it.Source)
			}
			if strings.Contains(it.Source, "ignored.png") {
				t.Fatal("output directory included")
			}
		}
	}
	c := DefaultConfig()
	c.Tool = "image"
	c.Root = root
	c.Magick = executable
	for _, invalid := range [][]string{{}, {".exe"}, {".png", "../jpg"}, {".foo.jpg"}, {".jpg .jpeg"}} {
		c.ImageFormats = invalid
		if _, e := BuildPlan(context.Background(), c, quiet); e == nil {
			t.Fatal("invalid selection accepted", invalid)
		}
	}
	for _, ext := range strings.Fields(Images) {
		if has("image"+ext, ImageCompressionFormats) {
			continue
		}
		c.ImageFormats = []string{".png", ext}
		if _, e := BuildPlan(context.Background(), c, quiet); e == nil {
			t.Fatal("retired compression format accepted", ext)
		}
	}
}

func TestCompressionSkipsAnimatedWebP(t *testing.T) {
	root := t.TempDir()
	// RIFF/WEBP + VP8X chunk, with the animation flag set.
	header := append([]byte("RIFF\x16\x00\x00\x00WEBPVP8X\x0a\x00\x00\x00"), make([]byte, 10)...)
	header[20] = 2
	for _, name := range []string{"animation.webp", "animation.png"} {
		file(t, filepath.Join(root, name), header)
	}
	header[20] = 16 // A static transparent WebP must still be eligible.
	file(t, filepath.Join(root, "static.webp"), header)
	c := DefaultConfig()
	c.Tool, c.Root = "image", root
	c.Magick, _ = os.Executable()
	c.ImageFormats = []string{".png", ".webp"}
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil || len(p.Items) != 3 {
		t.Fatal(p, e)
	}
	for _, it := range p.Items {
		if filepath.Base(it.Source) == "static.webp" {
			if it.Conflict != "" {
				t.Fatal("static WebP blocked", it)
			}
		} else if !strings.Contains(it.Conflict, "动画 WebP") {
			t.Fatal("animated WebP not skipped", it)
		}
	}
	// Execution also rejects animation before starting the external encoder.
	if e := compressImage(context.Background(), c, filepath.Join(root, "animation.webp"), filepath.Join(root, "output.webp")); e == nil || !strings.Contains(e.Error(), "动画 WebP") {
		t.Fatal(e)
	}
}

func TestImageFormatsAreScopedAndNormalized(t *testing.T) {
	c := DefaultConfig()
	c.ImageFormats = []string{".png", ".jpg"}
	c.Tool = "image"
	scoped := ScopeConfig(c)
	if !reflect.DeepEqual(scoped.ImageFormats, c.ImageFormats) {
		t.Fatal(scoped.ImageFormats)
	}
	scoped.ImageFormats[0] = ".webp"
	if c.ImageFormats[0] != ".png" {
		t.Fatal("settings share backing slice")
	}
	c.Tool = "video"
	if ScopeConfig(c).ImageFormats != nil {
		t.Fatal("image setting leaked into video")
	}
	got, e := normalizeImageFormats([]string{"PNG", ".JPG", "png"})
	if e != nil || !reflect.DeepEqual(got, []string{".jpg", ".png"}) {
		t.Fatal(got, e)
	}
}

func TestPNGOnlyCompressionLeavesJPEGUnchanged(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") != "1" {
		t.Skip("requires local ImageMagick")
	}
	root := t.TempDir()
	c := integrationConfig(t)
	c.Tool = "image"
	c.Root = root
	c.ImageFormats = []string{".png"}
	img := image.NewNRGBA(image.Rect(0, 0, 160, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 160; x++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(x), uint8(y), 30, 128})
		}
	}
	source := filepath.Join(root, "transparent.png")
	f, e := os.Create(source)
	if e != nil {
		t.Fatal(e)
	}
	encoder := png.Encoder{CompressionLevel: png.NoCompression}
	if e = encoder.Encode(f, img); e != nil {
		t.Fatal(e)
	}
	f.Close()
	jpegPath := filepath.Join(root, "already-compressed.jpg")
	original := []byte("do not process this jpeg")
	file(t, jpegPath, original)
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil || len(p.Items) != 1 || p.Items[0].Source != source {
		t.Fatal(p, e)
	}
	logs := []string{}
	r := Execute(context.Background(), p, func(s string) { logs = append(logs, s) }, progress)
	if r.Success != 1 || r.Failed != 0 {
		t.Fatal(r, logs)
	}
	b, e := os.ReadFile(jpegPath)
	if e != nil || !reflect.DeepEqual(b, original) {
		t.Fatal("jpeg changed", e)
	}
	if _, e = os.Stat(filepath.Join(p.Config.Output, "already-compressed.jpg")); !os.IsNotExist(e) {
		t.Fatal("jpeg output unexpectedly created")
	}
	output, e := os.Open(p.Items[0].Target)
	if e != nil {
		t.Fatal(e)
	}
	defer output.Close()
	decoded, e := png.Decode(output)
	if e != nil {
		t.Fatal(e)
	}
	_, _, _, alpha := decoded.At(80, 60).RGBA()
	if alpha != 128*257 {
		t.Fatal("PNG transparency not preserved", alpha)
	}
}
