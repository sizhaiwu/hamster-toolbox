package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestBrowserMediaCacheBoundsAndRecency(t *testing.T) {
	cache := newBrowserMediaCache[[]byte](2, 6)
	cache.put("a", []byte("aaa"), 3)
	cache.put("b", []byte("bb"), 2)
	cache.get("a")
	cache.put("c", []byte("ccc"), 3)
	if _, ok := cache.get("b"); ok {
		t.Fatal("least recently used data retained")
	}
	if _, ok := cache.get("a"); !ok {
		t.Fatal("recently used data evicted")
	}
	cache.put("huge", make([]byte, 7), 7)
	if _, ok := cache.get("huge"); ok {
		t.Fatal("cache exceeded byte limit")
	}
	cache.clear()
	if _, ok := cache.get("a"); ok {
		t.Fatal("cache not cleared")
	}
}
func TestBrowserMediaRoutesRequireOwnerAuthenticationAndValidInput(t *testing.T) {
	b, _ := newFileBrowser(t.TempDir(), "owner")
	dir := t.TempDir()
	path := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(path, []byte("invalid media"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"media-play", "image-preview", "video-preview"} {
		r := httptest.NewRequest("GET", "/files/"+action+"?path="+url.QueryEscape(path)+"&mode=auto", nil)
		w := httptest.NewRecorder()
		b.local(w, r)
		if w.Code != 403 {
			t.Fatal("owner authentication missing", action, w.Code)
		}
	}
	for _, action := range []string{"thumb", "media-play", "image-preview", "video-preview"} {
		r := httptest.NewRequest("GET", "/files/"+action+"?path="+url.QueryEscape(path)+"&mode=auto", nil)
		r.AddCookie(&http.Cookie{Name: "hamster_files_" + b.cookieTag, Value: b.localCookie})
		r.Header.Set("Sec-Fetch-Site", "same-site")
		w := httptest.NewRecorder()
		b.local(w, r)
		if w.Code != 403 {
			t.Fatal("cross-origin preview started expensive processing", action, w.Code)
		}
	}
	for _, query := range []string{"mode=unsafe", "mode=auto&start=NaN", "mode=auto&start=-1", "mode=auto&start=Inf", "mode=auto&start=999999999"} {
		r := httptest.NewRequest("GET", "/files/media-play?path="+url.QueryEscape(path)+"&"+query, nil)
		r.AddCookie(&http.Cookie{Name: "hamster_files_" + b.cookieTag, Value: b.localCookie})
		w := httptest.NewRecorder()
		b.local(w, r)
		if w.Code != 400 {
			t.Fatal("invalid streaming input accepted", query, w.Code)
		}
	}
	// Read permission is checked before media probing, even when the caller guesses an absolute path.
	for _, action := range []string{"media-play", "image-preview", "video-preview"} {
		r := httptest.NewRequest("GET", "/"+action+"?root=missing&path="+url.QueryEscape(path)+"&mode=auto", nil)
		w := httptest.NewRecorder()
		b.dispatch(w, r, false, action)
		if w.Code == 200 {
			t.Fatal("shared scope bypass", action)
		}
	}
}
