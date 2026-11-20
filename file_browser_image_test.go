package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

func TestBrowserLargeJPEGThumbnailIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") != "1" {
		t.Skip("requires ImageMagick")
	}
	program, _ := filepath.Abs(filepath.Join("tools", "imagemagick", "magick.exe"))
	if _, err := os.Stat(program); err != nil {
		program = toolbox.DefaultConfig().Magick
	}
	if program == "" {
		t.Fatal("ImageMagick unavailable")
	}
	dataDir, dir := t.TempDir(), t.TempDir()
	settings, _ := json.Marshal(preferences{Version: 1, Workspaces: map[string]toolbox.Config{"image": {Magick: program}}})
	if err := os.WriteFile(filepath.Join(dataDir, "settings.json"), settings, 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "large-photo.jpg")
	var original bytes.Buffer
	// Above the old 24 MP limit; synthetic content, no private media.
	if err := jpeg.Encode(&original, image.NewGray(image.Rect(0, 0, 4800, 6000)), &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, original.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	b, _ := newFileBrowser(dataDir, "owner")
	request := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/files/thumb?path="+url.QueryEscape(path), nil)
		r.AddCookie(&http.Cookie{Name: "hamster_files_" + b.cookieTag, Value: b.localCookie})
		w := httptest.NewRecorder()
		b.local(w, r)
		return w
	}
	w := request()
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatal("large image still has no thumbnail", w.Code, w.Body.String())
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(w.Body.Bytes()))
	if err != nil || cfg.Width != 384 || cfg.Height != 480 {
		t.Fatal("wrong thumbnail dimensions", cfg, err)
	}
	cached := request()
	if cached.Code != 200 || !bytes.Equal(cached.Body.Bytes(), w.Body.Bytes()) {
		t.Fatal("cached thumbnail unavailable")
	}
	after, err := os.ReadFile(path)
	if err != nil || sha256.Sum256(after) != sha256.Sum256(original.Bytes()) {
		t.Fatal("thumbnail generation changed source", err)
	}
}
