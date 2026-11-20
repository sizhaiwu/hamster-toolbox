package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
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
	args := nativePlayerArgs(file, "private-pipe", 0)
	if args[len(args)-1] != file || args[len(args)-2] != "--" {
		t.Fatal("filename was interpreted as options")
	}
	joined := strings.Join(args, "\n")
	if !strings.Contains(joined, "--hwdec=auto") || !strings.Contains(joined, "--input-ipc-server=private-pipe") || !strings.Contains(joined, "--no-config") {
		t.Fatal("safe native defaults absent")
	}
	if !strings.Contains(joined, "osc-vidscale=no") || !strings.Contains(joined, "osc-scalewindowed=1.4") || !strings.Contains(joined, "--osd-scale-by-window=no") {
		t.Fatal("readable native controls absent")
	}
	if !strings.Contains(joined, "--mute=no") || strings.Contains(joined, "--mute=yes") || !strings.Contains(joined, "--autofit=75%x80%") {
		t.Fatal("native playback must open large with sound enabled")
	}
	imageArgs := strings.Join(nativePlayerArgs(`E:\media\photo.PNG`, "private-pipe", 0), "\n")
	if !strings.Contains(imageArgs, "--image-display-duration=inf") || strings.Contains(joined, "--image-display-duration=inf") {
		t.Fatal("native images must stay open without changing video playback")
	}
}
