package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestVideoRangesYieldAndPreserveSeekingAndDownloads(t *testing.T) {
	b, _ := newFileBrowser(t.TempDir(), "owner")
	data := make([]byte, 3<<20)
	for i := range data {
		data[i] = byte(i % 251)
	}
	path := filepath.Join(t.TempDir(), "long.mp4")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	get := func(method, requested, extra string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/files/raw?path="+url.QueryEscape(path)+extra, nil)
		r.AddCookie(&http.Cookie{Name: "hamster_files_" + b.cookieTag, Value: b.localCookie})
		if requested != "" {
			r.Header.Set("Range", requested)
		}
		w := httptest.NewRecorder()
		b.local(w, r)
		return w
	}
	for _, start := range []int{0, 1 << 20, 2 << 20} {
		w := get("GET", "bytes="+strconv.Itoa(start)+"-", "")
		if w.Code != 206 || w.Body.Len() != 1<<20 || !bytes.Equal(w.Body.Bytes(), data[start:start+(1<<20)]) {
			t.Fatal("bounded continuation corrupted", start, w.Code, w.Body.Len())
		}
		if w.Header().Get("ETag") == "" {
			t.Fatal("range validator missing")
		}
	}
	w := get("GET", "bytes=123-456", "")
	if w.Code != 206 || !bytes.Equal(w.Body.Bytes(), data[123:457]) {
		t.Fatal("small seek changed")
	}
	w = get("GET", "bytes=-100", "")
	if w.Code != 206 || !bytes.Equal(w.Body.Bytes(), data[len(data)-100:]) {
		t.Fatal("suffix range changed")
	}
	w = get("GET", "bytes=999999999-", "")
	if w.Code != 416 {
		t.Fatal("invalid seek accepted", w.Code)
	}
	w = get("GET", "bytes=0-", "&download=1")
	if w.Body.Len() != len(data) {
		t.Fatal("download was truncated")
	}
	w = get("GET", "", "")
	if w.Code != 200 || w.Body.Len() != len(data) {
		t.Fatal("non-range response changed")
	}
	w = get("HEAD", "bytes=0-", "")
	if w.Body.Len() != 0 {
		t.Fatal("HEAD had a body")
	}
	r := httptest.NewRequest("GET", "/raw", nil)
	r.Header.Set("Range", "bytes=0-")
	shared := fairBrowserVideoRange(r, int64(len(data)), false)
	if shared.Header.Get("Range") != "bytes=0-262143" || r.Header.Get("Range") != "bytes=0-" {
		t.Fatal("shared chunks too large or original request mutated")
	}
}

func TestLightweightMediaCheckDoesNotNeedFFprobe(t *testing.T) {
	b, _ := newFileBrowser(t.TempDir(), "owner")
	path := filepath.Join(t.TempDir(), "sample.mp4")
	if err := os.WriteFile(path, []byte{0, 0, 0, 8, 'm', 'o', 'o', 'f'}, 0600); err != nil {
		t.Fatal(err)
	}
	w := explorerSettingsRequest(b, "media-info", map[string]any{"Path": path, "Lightweight": true}, true)
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"Fragmented":true`)) {
		t.Fatal("lightweight fragment detection failed", w.Code, w.Body.String())
	}
}
