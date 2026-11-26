package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

func syntheticTransportStream(size, prefix int) []byte {
	data := bytes.Repeat([]byte{0xff}, prefix+size*5)
	for i := 0; i < 5; i++ {
		copy(data[prefix+i*size:], []byte{0x47, 0x1f, 0xff, 0x10})
	}
	return data
}

func TestBrowserTypeScriptAndTransportStreamKinds(t *testing.T) {
	root := t.TempDir()
	// Printable G's at packet intervals must never turn source into video.
	lookalike := bytes.Repeat([]byte(" "), 188*5)
	for i := 0; i < 5; i++ {
		copy(lookalike[i*188:], []byte("Gabc"))
	}
	invalid := syntheticTransportStream(188, 0)
	for i := 0; i < 5; i++ {
		invalid[i*188+3] = 0 // Reserved adaptation-field-control is invalid.
	}
	for _, tc := range []struct {
		name string
		data []byte
		kind string
	}{
		{"main.ts", []byte("export const message: string = '代码';\n"), "text"},
		{"types.d.ts", []byte("declare interface Demo { id: number }\n"), "text"},
		{"CAPS.TS", []byte("// comment\nexport {};"), "text"},
		{"empty.ts", nil, "text"},
		{"bom.ts", append([]byte{0xef, 0xbb, 0xbf}, []byte("export {};")...), "text"},
		{"utf16.ts", []byte{0xff, 0xfe, 'a', 0, '=', 0, '1', 0}, "text"},
		{"lookalike.ts", lookalike, "text"},
		{"module.mts", []byte("export const a = 1;"), "text"},
		{"component.tsx", []byte("const View = () => <div/>;"), "text"},
		{"common.cts", []byte("export = {};"), "text"},
		{"stream.ts", syntheticTransportStream(188, 0), "video"},
		{"timestamped.TS", syntheticTransportStream(192, 4), "video"},
		{"protected.ts", syntheticTransportStream(204, 0), "video"},
		{"leading.ts", syntheticTransportStream(188, 1024), "video"},
		{"recording.mts", syntheticTransportStream(192, 4), "video"},
		{"broken.ts", invalid, "file"},
		{"short.ts", []byte{0x47, 0x00, 0x00, 0x10}, "file"},
		{"regular.mp4", []byte("extension-only for unambiguous types"), "video"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, tc.name)
			if err := os.WriteFile(path, tc.data, 0600); err != nil {
				t.Fatal(err)
			}
			f, err := toolbox.BrowseOpen(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			st, _ := f.Stat()
			f.Seek(2, io.SeekStart)
			if kind := browserFileKind(f, st); kind != tc.kind {
				t.Fatalf("kind = %q, want %q", kind, tc.kind)
			}
			if at, _ := f.Seek(0, io.SeekCurrent); at != 2 {
				t.Fatal("classification moved the reader offset", at)
			}
			if kind := browserPathKind(path, st); kind != tc.kind {
				t.Fatal("listing kind differs", kind)
			}
		})
	}
	if browserKind("new.ts") != "text" || browserKind("new.mts") != "text" {
		t.Fatal("new source files must open in the editor")
	}
}

func TestBrowserTypeScriptRoutesDoNotStartMedia(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.ts")
	video := filepath.Join(root, "recording.ts")
	source := []byte("export const message: string = 'hello';\n")
	os.WriteFile(path, source, 0600)
	os.WriteFile(video, syntheticTransportStream(188, 0), 0600)
	b, err := newFileBrowser(t.TempDir(), "synthetic-owner")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := toolbox.NewBrowserScope(root)
	if err != nil {
		t.Fatal(err)
	}
	b.scopes["demo"] = scope
	b.config.Roots = []sharedFolder{{ID: "demo", Path: root, Name: "Demo", Writable: true}}
	for _, local := range []bool{true, false} {
		display, folder := path, root
		if !local {
			display, folder = "main.ts", ""
		}
		post := func(route string, value any) *httptest.ResponseRecorder {
			data, _ := json.Marshal(value)
			r := httptest.NewRequest("POST", "/api/"+route+"?root=demo", bytes.NewReader(data))
			r.Header.Set("X-Hamster-Browser", "1")
			w := httptest.NewRecorder()
			b.dispatch(w, r, local, "api/"+route)
			return w
		}
		w := post("list", map[string]any{"Path": folder, "PageSize": 200})
		var listing struct{ Entries []browserEntry }
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &listing) != nil || len(listing.Entries) != 2 {
			t.Fatal("listing", local, w.Code, w.Body.String())
		}
		for _, item := range listing.Entries {
			want := "text"
			if item.Name == "recording.ts" {
				want = "video"
			}
			if item.Kind != want {
				t.Fatal("wrong listing kind", local, item.Name, item.Kind)
			}
		}
		w = post("properties", map[string]string{"Path": display})
		var properties map[string]any
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &properties) != nil || properties["Kind"] != "text" || properties["MediaError"] != nil {
			t.Fatal("source properties probed as video", local, w.Code, w.Body.String())
		}
		w = post("text-open", map[string]string{"Path": display})
		if w.Code != 200 || !strings.Contains(w.Body.String(), "export const") {
			t.Fatal("source editor", local, w.Code, w.Body.String())
		}
		w = post("media-info", map[string]any{"Path": display, "Lightweight": true})
		if w.Code == 200 {
			t.Fatal("source accepted as media", local)
		}
		for _, route := range []string{"thumb", "video-preview", "media-play"} {
			r := httptest.NewRequest("HEAD", "/"+route+"?root=demo&path="+url.QueryEscape(display)+"&mode=auto", nil)
			w := httptest.NewRecorder()
			b.content(w, r, local, route)
			if w.Code == 200 || strings.HasPrefix(w.Header().Get("Content-Type"), "video/") {
				t.Fatal("source entered media route", local, route, w.Code)
			}
		}
		for _, route := range []string{"video-preview", "media-play"} {
			videoDisplay := video
			if !local {
				videoDisplay = "recording.ts"
			}
			r := httptest.NewRequest("HEAD", "/"+route+"?root=demo&path="+url.QueryEscape(videoDisplay)+"&mode=auto", nil)
			w := httptest.NewRecorder()
			b.content(w, r, local, route)
			if w.Code != 200 {
				t.Fatal("actual transport video rejected", local, route, w.Code, w.Body.String())
			}
		}
	}
	if len(b.mediaState().hoverSlots) != 0 || len(b.mediaState().playSlots) != 0 {
		t.Fatal("source inspection consumed video slots")
	}
	// A hand-crafted request must not launch a local player for source code.
	ref := shortcutTestReference(t, b, path)
	data, _ := json.Marshal(map[string]string{"Action": "open", "Path": path, "Version": ref})
	w := httptest.NewRecorder()
	b.nativePlayerAPI(w, httptest.NewRequest("POST", "/api/native-player", bytes.NewReader(data)))
	if w.Code == 200 || !strings.Contains(w.Body.String(), "图片或视频") {
		t.Fatal("source accepted by native player", w.Code, w.Body.String())
	}
	if st, err := os.Stat(path); err != nil || st.Size() != int64(len(source)) {
		t.Fatal("source changed during inspection")
	}
}
