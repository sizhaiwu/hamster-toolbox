package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

func TestBrowserHoverCacheRangeAndBudget(t *testing.T) {
	b, err := newFileBrowser(t.TempDir(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "movie.mp4")
	if err = os.WriteFile(path, []byte("synthetic source"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := toolbox.BrowseOpen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, _ := f.Stat()
	state := b.mediaState()
	request := func(method, byteRange string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/files/video-preview?path="+url.QueryEscape(path), nil)
		r.AddCookie(&http.Cookie{Name: "hamster_files_" + b.cookieTag, Value: b.localCookie})
		if byteRange != "" {
			r.Header.Set("Range", byteRange)
		}
		w := httptest.NewRecorder()
		b.local(w, r)
		return w
	}
	// Header probes do not start a decoder, even if every generation slot is busy.
	state.hoverSlots <- struct{}{}
	w := request("HEAD", "")
	if w.Code != 200 || w.Body.Len() != 0 {
		t.Fatal("HEAD launched work or returned content", w.Code)
	}
	w = request("GET", "")
	if w.Code != http.StatusTooManyRequests {
		t.Fatal("generation budget bypass", w.Code)
	}
	data := []byte("short cached preview bytes")
	state.hover.put(browserMediaKey(f, st), data, len(data))
	w = request("GET", "bytes=6-11")
	if w.Code != 206 || !bytes.Equal(w.Body.Bytes(), data[6:12]) || w.Header().Get("Content-Type") != "video/mp4" {
		t.Fatal("cached range playback failed", w.Code, w.Body.String())
	}
	// Source changes must not return stale cached media. A busy slot proves a miss.
	f.Close()
	if err = os.WriteFile(path, []byte("changed source with a different size"), 0600); err != nil {
		t.Fatal(err)
	}
	w = request("GET", "")
	if w.Code != http.StatusTooManyRequests {
		t.Fatal("stale clip reused after source change", w.Code)
	}
	<-state.hoverSlots
}
