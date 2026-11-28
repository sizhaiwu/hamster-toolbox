package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func nativeFeatureSession(t *testing.T, m *nativePlayerManager, id, kind string) (*nativePlayback, <-chan []any) {
	t.Helper()
	a, b := net.Pipe()
	commands := make(chan []any, 32)
	t.Cleanup(func() { a.Close(); b.Close() })
	s := &nativePlayback{view: nativePlaybackView{ID: id, Name: id, Kind: kind, Connected: true, Speed: 1, Volume: 100}, conn: a}
	m.sessions[id] = s
	go func() {
		d := json.NewDecoder(b)
		for {
			var message struct {
				Command []any `json:"command"`
			}
			if d.Decode(&message) != nil {
				return
			}
			commands <- message.Command
		}
	}()
	return s, commands
}
func nextNativeCommand(t *testing.T, c <-chan []any) string {
	t.Helper()
	select {
	case command := <-c:
		return fmt.Sprint(command)
	case <-time.After(time.Second):
		t.Fatal("command timeout")
		return ""
	}
}
func TestNativeMediaCommandsAndProperties(t *testing.T) {
	m := &nativePlayerManager{dataDir: t.TempDir(), sessions: map[string]*nativePlayback{}}
	video, commands := nativeFeatureSession(t, m, "video", "video")
	image, imageCommands := nativeFeatureSession(t, m, "image", "image")
	video.updateMediaProperty("track-list", json.RawMessage(`[{"id":1,"type":"audio","title":"English","selected":true},{"id":2,"type":"sub","lang":"zh"}]`))
	video.updateMediaProperty("video-params/w", json.RawMessage(`3840`))
	video.updateMediaProperty("speed", json.RawMessage(`1.5`))
	video.updateMediaProperty("frame-drop-count", json.RawMessage(`3`))
	video.updateMediaProperty("decoder-frame-drop-count", json.RawMessage(`null`))
	if video.view.VideoWidth != 3840 || video.view.Speed != 1.5 || !video.view.RenderMetricsKnown || video.view.DropMetricsKnown {
		t.Fatal(video.view)
	}
	for _, tc := range []struct {
		action  string
		value   float64
		track   int
		command string
	}{{"speed", 2, 0, "[set_property speed 2]"}, {"audio-track", 0, 1, "[set_property aid 1]"}, {"subtitle-track", 0, 2, "[set_property sid 2]"}, {"subtitle-track", 0, 0, "[set_property sid no]"}} {
		if e := m.mediaAction(tc.action, "video", tc.value, tc.track, false); e != nil {
			t.Fatal(e)
		}
		if command := nextNativeCommand(t, commands); command != tc.command {
			t.Fatal(command, tc.command)
		}
	}
	if m.mediaAction("speed", "video", 5, 0, false) == nil || m.mediaAction("audio-track", "video", 0, 2, false) == nil || m.mediaAction("subtitle-track", "video", 0, 999, false) == nil || m.mediaAction("speed", "image", 2, 0, false) == nil {
		t.Fatal("invalid media command accepted")
	}
	image.view.Rotation = 90
	if e := m.mediaAction("rotate-left", "image", 0, 0, false); e != nil {
		t.Fatal(e)
	}
	if command := nextNativeCommand(t, imageCommands); command != "[set_property video-rotate 0]" {
		t.Fatal(command)
	}
	if e := m.mediaAction("resume-all", "", 0, 0, false); e != nil {
		t.Fatal(e)
	}
	if command := nextNativeCommand(t, commands); command != "[set_property pause false]" {
		t.Fatal(command)
	}
}
func TestNativeMediaPreferencesHistoryAndFocus(t *testing.T) {
	dir := t.TempDir()
	m := &nativePlayerManager{dataDir: dir, sessions: map[string]*nativePlayback{}}
	base := nativePlayerArgs("test.mp4", "pipe", 0, "compact")
	if args := strings.Join(m.mediaPlaybackArgs(base, "video"), " "); !strings.Contains(args, "--resume-playback=no") {
		t.Fatal(args)
	}
	if e := m.setMediaPreference("remember-media", true); e != nil {
		t.Fatal(e)
	}
	args := m.mediaPlaybackArgs(base, "video")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--save-position-on-quit=yes") || !strings.Contains(joined, "--write-filename-in-watch-later-config=no") || !strings.Contains(joined, "--resume-playback-check-mtime=yes") || args[len(args)-2] != "--" || args[len(args)-1] != "test.mp4" {
		t.Fatal(joined)
	}
	reloaded := &nativePlayerManager{dataDir: dir}
	reloaded.loadMediaPreferences()
	if !reloaded.mediaPreferences.Remember {
		t.Fatal("remember preference lost")
	}
	file := filepath.Join(dir, "native-watch-later", strings.Repeat("A", 32))
	os.WriteFile(file, []byte("start=12\nvolume=70"), 0600)
	other := filepath.Join(dir, "native-watch-later", "keep.txt")
	os.WriteFile(other, []byte("unrelated"), 0600)
	if e := m.cleanMediaHistory(true); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(file); !os.IsNotExist(e) {
		t.Fatal("record not cleared", e)
	}
	if _, e := os.Stat(other); e != nil {
		t.Fatal("unrelated file touched", e)
	}
	a, ca := nativeFeatureSession(t, m, "a", "video")
	b, cb := nativeFeatureSession(t, m, "b", "video")
	if e := m.setMediaPreference("focus-audio", true); e != nil {
		t.Fatal(e)
	}
	m.focusMedia("a")
	if command := nextNativeCommand(t, cb); command != "[set_property mute true]" {
		t.Fatal(command)
	}
	b.view.Muted = true
	m.focusMedia("b")
	if command := nextNativeCommand(t, ca); command != "[set_property mute true]" {
		t.Fatal(command)
	}
	if command := nextNativeCommand(t, cb); command != "[set_property mute false]" {
		t.Fatal(command)
	}
	a.view.Muted = true
	if e := m.setMediaPreference("focus-audio", false); e != nil {
		t.Fatal(e)
	}
	if command := nextNativeCommand(t, ca); command != "[set_property mute false]" {
		t.Fatal(command)
	}
	before := m.mediaPreferences
	m.dataDir = filepath.Join(dir, "native-media.json")
	if m.setMediaPreference("remember-media", false) == nil || m.mediaPreferences != before {
		t.Fatal("failed write changed preference")
	}
}

func TestNativeEndActions(t *testing.T) {
	m := &nativePlayerManager{dataDir: t.TempDir()}
	if m.status()["EndAction"] != "keep" {
		t.Fatal("default ending action changed")
	}
	for _, tc := range []struct{ action, keepOpen, loop string }{{"keep", "yes", "no"}, {"loop", "yes", "inf"}, {"close", "no", "no"}} {
		if err := m.setEndAction(tc.action); err != nil {
			t.Fatal(err)
		}
		loaded := &nativePlayerManager{dataDir: m.dataDir}
		loaded.loadMediaPreferences()
		if loaded.mediaPreferences.EndAction != tc.action || loaded.status()["EndAction"] != tc.action {
			t.Fatal("end action was not remembered", tc.action)
		}
		args := strings.Join(loaded.mediaPlaybackArgs(nativePlayerArgs("video.mp4", "pipe", 0, "compact"), "video"), " ")
		if !strings.Contains(args, "--keep-open="+tc.keepOpen) || !strings.Contains(args, "--loop-file="+tc.loop) || !strings.HasSuffix(args, "-- video.mp4") {
			t.Fatal("wrong video ending action", tc.action, args)
		}
		args = strings.Join(loaded.mediaPlaybackArgs(nativePlayerArgs("image.png", "pipe", 0, "compact"), "image"), " ")
		if !strings.Contains(args, "--keep-open=yes") || !strings.Contains(args, "--loop-file=no") || !strings.Contains(args, "--image-display-duration=inf") {
			t.Fatal("video ending action affected images", args)
		}
	}
	if m.setEndAction("--bad") == nil || m.mediaPreferences.EndAction != "close" {
		t.Fatal("invalid end action accepted")
	}
	m.dataDir = filepath.Join(m.dataDir, "native-media.json")
	if m.setEndAction("loop") == nil || m.mediaPreferences.EndAction != "close" {
		t.Fatal("failed save changed the end action")
	}
	if nativeEndAction("invalid") != "keep" || nativeEndAction("") != "keep" {
		t.Fatal("invalid or missing setting must keep the last frame")
	}
}
