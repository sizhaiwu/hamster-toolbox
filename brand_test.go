package main

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBrandAssetsAvailableBeforeSharedLogin(t *testing.T) {
	b, _ := newFileBrowser(t.TempDir(), "owner")
	for _, name := range []string{"hamster-logo.png", "favicon-32.png", "apple-touch-icon.png", "favicon.ico"} {
		r := httptest.NewRequest("GET", "/"+name, nil)
		r.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		b.remote(w, r)
		if w.Code != 200 || w.Header().Get("Content-Type") != brandContentType(name) || w.Body.Len() == 0 {
			t.Fatal("shared login branding unavailable", name, w.Code, w.Header())
		}
		if name == "favicon.ico" {
			data := w.Body.Bytes()
			if len(data) < 6 || binary.LittleEndian.Uint16(data[2:4]) != 1 || binary.LittleEndian.Uint16(data[4:6]) != 6 {
				t.Fatal("ICO must contain all six app/icon sizes")
			}
		} else {
			img, err := png.Decode(bytes.NewReader(w.Body.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			_, _, _, alpha := img.At(0, 0).RGBA()
			if alpha != 0 {
				t.Fatal("brand icon lost transparent background", name)
			}
		}
		w = httptest.NewRecorder()
		b.dispatch(w, httptest.NewRequest("HEAD", "/files/"+name, nil), true, name)
		if w.Code != http.StatusOK || w.Body.Len() != 0 {
			t.Fatal("local brand HEAD response invalid", name)
		}
	}
	for _, name := range []string{"settings.json", "../hamster-logo.png", "brand_windows_amd64.syso"} {
		if brandContentType(name) != "" {
			t.Fatal("brand whitelist exposes unrelated files", name)
		}
	}
}
