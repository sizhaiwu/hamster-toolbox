package main

import (
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Optional real-browser fixture. Every file and setting is synthetic; raw PNG
// responses are delayed so first-frame layout can be checked without a cache.
func TestImagePreviewBrowserAcceptance(t *testing.T) {
	output := os.Getenv("TOOLBOX_IMAGE_QA")
	if output == "" {
		t.Skip("optional synthetic image preview browser fixture")
	}
	s := &server{dataDir: t.TempDir(), token: "synthetic-image-owner"}
	b, err := newFileBrowser(s.dataDir, s.token)
	if err != nil {
		t.Fatal(err)
	}
	s.browser = b
	root := t.TempDir()
	for _, fixture := range []struct {
		name          string
		width, height int
		fill          color.RGBA
	}{
		{"landscape.png", 2400, 1400, color.RGBA{45, 133, 156, 255}},
		{"portrait.png", 1800, 3000, color.RGBA{160, 94, 128, 255}},
	} {
		img := image.NewRGBA(image.Rect(0, 0, fixture.width, fixture.height))
		draw.Draw(img, img.Bounds(), image.NewUniform(fixture.fill), image.Point{}, draw.Src)
		f, err := os.Create(filepath.Join(root, fixture.name))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, img)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	handler := s.httpHandler()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/files/raw" && strings.HasSuffix(r.URL.Query().Get("path"), ".png") {
			timer := time.NewTimer(250 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-r.Context().Done():
				return
			}
		}
		handler.ServeHTTP(w, r)
	}))
	defer srv.Close()
	data, _ := json.Marshal(map[string]string{"URL": srv.URL + "/files/?session=" + s.token, "Root": root, "StopFile": output + ".stop"})
	if err = os.WriteFile(output, data, 0600); err != nil {
		t.Fatal(err)
	}
	for end := time.Now().Add(8 * time.Minute); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
		if _, err = os.Stat(output + ".stop"); err == nil {
			return
		}
	}
}
