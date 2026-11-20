package toolbox

import (
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestBrowserImageDimensionsPinnedHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "misnamed.jpg")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(f, image.NewRGBA(image.Rect(0, 0, 17, 9))); err != nil {
		t.Fatal(err)
	}
	f.Close()
	f, err = BrowseOpen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.Seek(12, 0)
	w, h, format, err := BrowserImageDimensions(context.Background(), "", f)
	if err != nil || w != 17 || h != 9 || format != "png" {
		t.Fatal(w, h, format, err)
	}
	bad := filepath.Join(t.TempDir(), "bad.jpg")
	os.WriteFile(bad, []byte("not an image"), 0600)
	broken, err := BrowseOpen(bad)
	if err != nil {
		t.Fatal(err)
	}
	defer broken.Close()
	if _, _, _, err = BrowserImageDimensions(context.Background(), "not-a-program", broken); err == nil {
		t.Fatal("unrecognized format accepted")
	}
}
