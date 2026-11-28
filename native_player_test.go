package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func nativeRequest(b *fileBrowser, body string, owner bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/files/api/native-player", strings.NewReader(body))
	r.Header.Set("X-Hamster-Browser", "1")
	if owner {
		r.AddCookie(&http.Cookie{Name: "hamster_files_" + b.cookieTag, Value: b.localCookie})
	}
	w := httptest.NewRecorder()
	b.local(w, r)
	return w
}

func TestNativePlayerWindowSizePersistence(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "native-player.json")
	if err := os.WriteFile(config, []byte(`{"Program":"tools/mpv/mpv.exe"}`), 0600); err != nil {
		t.Fatal(err)
	}
	b, _ := newFileBrowser(dir, "owner")
	if b.nativePlayer().windowSize != "compact" {
		t.Fatal("old configuration must use compact default")
	}
	for _, size := range []string{"medium", "large", "compact"} {
		w := nativeRequest(b, `{"Action":"window-size","WindowSize":"`+size+`"}`, true)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		restarted, _ := newFileBrowser(dir, "owner")
		m := restarted.nativePlayer()
		if m.windowSize != size || m.program != "tools/mpv/mpv.exe" {
			t.Fatal("size did not survive restart or changed configured program", m.windowSize, m.program)
		}
	}
	before, _ := os.ReadFile(config)
	if w := nativeRequest(b, `{"Action":"window-size","WindowSize":"--run=bad"}`, true); w.Code != 400 {
		t.Fatal("unexpected size accepted", w.Code)
	}
	after, _ := os.ReadFile(config)
	if string(after) != string(before) || b.nativePlayer().windowSize != "compact" {
		t.Fatal("invalid size changed saved settings")
	}
	if err := b.nativePlayer().setWindowSize("medium"); err != nil {
		t.Fatal(err)
	}
	if err := b.nativePlayer().configure(""); err != nil {
		t.Fatal(err)
	}
	if b.nativePlayer().windowSize != "medium" || b.nativePlayer().program != "" {
		t.Fatal("program detection reset window size")
	}
	m := b.nativePlayer()
	m.dataDir = config // A file cannot be used as a settings directory.
	if m.setWindowSize("large") == nil || m.windowSize != "medium" {
		t.Fatal("failed persistence changed active size")
	}
}

func TestNativePlayerWindowSizesAndPosition(t *testing.T) {
	for _, tc := range []struct{ size, fit string }{{"compact", "800x600"}, {"medium", "1100x825"}, {"large", "75%x80%"}} {
		for _, file := range []string{"sample.mp4", "portrait.png"} {
			args := strings.Join(nativePlayerArgs(file, "test-pipe", 2, tc.size), "\n")
			if !strings.Contains(args, "--autofit="+tc.fit+"\n") || !strings.Contains(args, "--autofit-larger=90%x90%") || !strings.Contains(args, "--geometry=+88+88") {
				t.Fatal(tc.size, args)
			}
		}
	}
}
func TestNativePlayerOwnerBoundaryAndValidation(t *testing.T) {
	b, _ := newFileBrowser(t.TempDir(), "owner")
	if w := nativeRequest(b, `{"Action":"status"}`, false); w.Code != 403 {
		t.Fatal("owner capability missing", w.Code)
	}
	r := httptest.NewRequest("POST", "/api/native-player", strings.NewReader(`{"Action":"status"}`))
	r.Header.Set("X-Hamster-Browser", "1")
	w := httptest.NewRecorder()
	b.dispatch(w, r, false, "api/native-player")
	if w.Code != 404 {
		t.Fatal("shared guest can control host player", w.Code)
	}
	r = httptest.NewRequest("POST", "/files/api/native-player", strings.NewReader(`{"Action":"status"}`))
	r.AddCookie(&http.Cookie{Name: "hamster_files_" + b.cookieTag, Value: b.localCookie})
	r.Header.Set("X-Hamster-Browser", "1")
	r.Header.Set("Origin", "https://outside.example")
	w = httptest.NewRecorder()
	b.local(w, r)
	if w.Code != 403 {
		t.Fatal("cross-origin native control allowed")
	}
	for _, body := range []string{`{"Action":"run"}`, `{"Action":"status","Command":["run","bad"]}`, `{"Action":"open","Path":"https://example.com/movie.mp4"}`, `{"Action":"open","Path":"C:\\demo\\movie.mp4","Version":"expired"}`, `{"Action":"pause","ID":"unknown"}`} {
		if w = nativeRequest(b, body, true); w.Code != 400 {
			t.Fatal("invalid native request accepted", body, w.Code)
		}
	}
}
func TestNativePlayerIPCWhitelistAndArgumentIsolation(t *testing.T) {
	b, _ := newFileBrowser(t.TempDir(), "owner")
	m := b.nativePlayer()
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	m.sessions["fixture"] = &nativePlayback{conn: client, view: nativePlaybackView{ID: "fixture", Name: "sample.mp4", Connected: true, Decoder: "d3d11va"}}
	commands := make(chan []any, 8)
	go func() {
		decoder := json.NewDecoder(server)
		for {
			var request struct{ Command []any }
			if decoder.Decode(&request) != nil {
				return
			}
			commands <- request.Command
		}
	}()
	for _, tc := range []struct {
		action, property string
		value            any
	}{{"pause", "pause", true}, {"resume", "pause", false}, {"mute", "mute", true}, {"unmute", "mute", false}, {"pin", "ontop", true}, {"unpin", "ontop", false}} {
		body, _ := json.Marshal(map[string]any{"Action": tc.action, "ID": "fixture"})
		w := nativeRequest(b, string(body), true)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		got := <-commands
		if len(got) != 3 || got[0] != "set_property" || got[1] != tc.property || got[2] != tc.value {
			t.Fatal("unexpected IPC", got)
		}
	}
	if w := nativeRequest(b, `{"Action":"seek","ID":"fixture","Value":-1}`, true); w.Code != 400 {
		t.Fatal("negative seek accepted")
	}
	if w := nativeRequest(b, `{"Action":"seek","ID":"fixture","Value":35}`, true); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	got := <-commands
	if got[0] != "seek" || got[1] != float64(35) || got[2] != "absolute" {
		t.Fatal(got)
	}
	session := m.sessions["fixture"]
	session.mu.Lock()
	session.view.Time, session.view.Duration = 120, 120
	session.mu.Unlock()
	if w := nativeRequest(b, `{"Action":"resume","ID":"fixture"}`, true); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if got = <-commands; got[0] != "seek" || got[1] != float64(0) {
		t.Fatal("ended playback must restart", got)
	}
	if got = <-commands; got[0] != "set_property" || got[1] != "pause" || got[2] != false {
		t.Fatal("restart must resume", got)
	}
	file := `E:\media\中文 & $(sample) [01].mp4`
	args := nativePlayerArgs(file, "private-pipe", 0, "compact")
	if args[len(args)-1] != file || args[len(args)-2] != "--" {
		t.Fatal("filename was interpreted as options")
	}
	joined := strings.Join(args, "\n")
	if !strings.Contains(joined, "--hwdec=auto") || !strings.Contains(joined, "--input-ipc-server=private-pipe") || !strings.Contains(joined, "--no-config") {
		t.Fatal("safe native defaults absent")
	}
	if !strings.Contains(joined, "--osc=no") || !strings.Contains(joined, "hamster-mode=full") || !strings.Contains(joined, "--osd-scale-by-window=no") {
		t.Fatal("readable native controls absent")
	}
	if !strings.Contains(joined, "--mute=no") || strings.Contains(joined, "--mute=yes") || !strings.Contains(joined, "--autofit=800x600") {
		t.Fatal("native playback must open compact with sound enabled")
	}
	imageArgs := strings.Join(nativePlayerArgs(`E:\media\photo.PNG`, "private-pipe", 0, "compact"), "\n")
	if !strings.Contains(imageArgs, "--image-display-duration=inf") || strings.Contains(joined, "--image-display-duration=inf") {
		t.Fatal("native images must stay open without changing video playback")
	}
}
