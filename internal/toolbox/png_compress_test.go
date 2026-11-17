package toolbox

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPNGModeValidationAndContentGuards(t *testing.T) {
	root := t.TempDir()
	exe, _ := os.Executable()
	c := DefaultConfig()
	c.Tool = "image"
	c.Root = root
	c.Magick = exe
	c.ImageFormats = []string{".png"}
	c.PngMode = "webp-png"
	c.Quality = 0
	normalized, e := Normalize(c)
	if e != nil || normalized.PngQuality != 90 {
		t.Fatal(normalized, e)
	}
	c.PngQuality = -1
	if _, e := Normalize(c); e == nil {
		t.Fatal("invalid PNG quality accepted")
	}
	c.PngQuality = 90
	c.PngMode = "unknown"
	if _, e := Normalize(c); e == nil {
		t.Fatal("invalid mode accepted")
	}
	c.PngMode = "webp-png"
	file(t, filepath.Join(root, "already.png"), []byte("RIFF0000WEBPencoded-data"))
	var apng bytes.Buffer
	apng.Write([]byte{137, 80, 78, 71, 13, 10, 26, 10})
	binary.Write(&apng, binary.BigEndian, uint32(13))
	apng.WriteString("IHDR")
	apng.Write(make([]byte, 17))
	binary.Write(&apng, binary.BigEndian, uint32(8))
	apng.WriteString("acTL")
	apng.Write(make([]byte, 12))
	file(t, filepath.Join(root, "animation.png"), apng.Bytes())
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil || len(p.Items) != 2 {
		t.Fatal(p, e)
	}
	for _, it := range p.Items {
		if filepath.Base(it.Source) == "animation.png" && it.Conflict == "" {
			t.Fatal("animation conversion allowed", it)
		}
		if filepath.Base(it.Source) == "already.png" && it.Conflict != "" {
			t.Fatal("WebP recompression blocked", it)
		}
	}
	c.ImageFormats = []string{".jpg"}
	c.Quality = 60
	c.PngMode = "invalid-unused"
	if _, e := Normalize(c); e != nil {
		t.Fatal("unused PNG option blocked JPEG", e)
	}
}

func TestPNGWebPWithOriginalFilename(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") != "1" {
		t.Skip("requires ImageMagick WebP support")
	}
	root := t.TempDir()
	source := filepath.Join(root, "透明原图.PNG")
	img := image.NewNRGBA(image.Rect(0, 0, 320, 240))
	for y := 0; y < 240; y++ {
		for x := 0; x < 320; x++ {
			alpha := uint8(128)
			if x > 160 {
				alpha = 255
			}
			img.SetNRGBA(x, y, color.NRGBA{uint8(x % 256), uint8(y % 256), 80, alpha})
		}
	}
	f, e := os.Create(source)
	if e != nil {
		t.Fatal(e)
	}
	enc := png.Encoder{CompressionLevel: png.NoCompression}
	if e = enc.Encode(f, img); e != nil {
		t.Fatal(e)
	}
	f.Close()
	original, _ := os.ReadFile(source)
	file(t, filepath.Join(root, "keep.jpg"), []byte("jpeg must remain untouched"))
	c := integrationConfig(t)
	c.Root = root
	c.Tool = "image"
	c.ImageFormats = []string{".png"}
	c.PngMode = "webp-png"
	c.PngQuality = 90
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil || len(p.Items) != 1 || filepath.Base(p.Items[0].Target) != "透明原图.PNG" || !strings.Contains(p.Items[0].Note, "WebP") {
		t.Fatal(p, e)
	}
	logs := []string{}
	r := Execute(context.Background(), p, func(s string) { logs = append(logs, s) }, progress)
	if r.Success != 1 || r.Failed != 0 {
		t.Fatal(r, logs)
	}
	result, e := os.ReadFile(p.Items[0].Target)
	if e != nil {
		t.Fatal(e)
	}
	if len(result) >= len(original) || string(result[:4]) != "RIFF" || string(result[8:12]) != "WEBP" {
		t.Fatal("wrong format or ineffective compression")
	}
	info, e := inspectImage(context.Background(), c.Magick, p.Items[0].Target)
	if e != nil || info.format != "WEBP" || info.width != 320 || info.height != 240 {
		t.Fatal(info, e)
	}
	raw, e := run(context.Background(), c.Magick, p.Items[0].Target, "-depth", "8", "RGBA:-")
	if e != nil || len(raw) != 320*240*4 {
		t.Fatal(len(raw), e)
	}
	if raw[(60*320+80)*4+3] != 128 || raw[(60*320+240)*4+3] != 255 {
		t.Fatal("alpha values changed")
	}
	unchanged, _ := os.ReadFile(source)
	if !bytes.Equal(unchanged, original) {
		t.Fatal("source overwritten")
	}
	jpeg, _ := os.ReadFile(filepath.Join(root, "keep.jpg"))
	if string(jpeg) != "jpeg must remain untouched" {
		t.Fatal("unselected JPEG changed")
	}
	t.Logf("PNG %d bytes -> WebP %d bytes, same filename and 320x240 dimensions", len(original), len(result))

	// Normal .webp output remains an alternative with the same content checks.
	c.PngMode = "webp"
	native, e := BuildPlan(context.Background(), c, quiet)
	if e != nil || len(native.Items) != 1 || filepath.Ext(native.Items[0].Target) != ".webp" {
		t.Fatal(native, e)
	}
	r = Execute(context.Background(), native, func(s string) { logs = append(logs, s) }, progress)
	if r.Success != 1 {
		t.Fatal(r, logs)
	}
	// Simulate the user's overwrite workflow in the temporary test directory.
	if e = os.WriteFile(source, result, 0600); e != nil {
		t.Fatal(e)
	}
	c.PngMode = "webp-png"
	c.Output = filepath.Join(root, "second-pass")
	c.PngQuality = 50
	again, e := BuildPlan(context.Background(), c, quiet)
	if e != nil || len(again.Items) != 1 || again.Items[0].Conflict != "" {
		t.Fatal(again, e)
	}
	r = Execute(context.Background(), again, func(s string) { logs = append(logs, s) }, progress)
	if r.Success != 1 || r.Failed != 0 {
		t.Fatal("deliberate WebP recompression failed", r, logs)
	}
	second, e := os.ReadFile(again.Items[0].Target)
	if e != nil || string(second[:4]) != "RIFF" || len(second) >= len(result) {
		t.Fatal("second-pass output invalid", len(second), len(result), e)
	}
}
