package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func nativeMediaWait(t *testing.T, label string, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		time.Sleep(80 * time.Millisecond)
	}
	t.Fatal("native integration timeout: " + label)
}
func nativeMediaProperty(t *testing.T, s *nativePlayback, name string) json.RawMessage {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := dialNativePlayer(ctx, s.endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	if err = json.NewEncoder(conn).Encode(map[string]any{"command": []any{"get_property", name}}); err != nil {
		t.Fatal(err)
	}
	var reply struct {
		Data  json.RawMessage
		Error string
	}
	if err = json.NewDecoder(conn).Decode(&reply); err != nil || reply.Error != "success" {
		t.Fatal(name, reply.Error, err)
	}
	return reply.Data
}
func TestNativeMediaWindowIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_NATIVE_MEDIA_QA") != "1" {
		t.Skip("optional synthetic-media mpv window test")
	}
	program, _ := filepath.Abs("tools/mpv/mpv.exe")
	if _, err := os.Stat(program); err != nil {
		t.Fatal(err)
	}
	b, _ := newFileBrowser(t.TempDir(), "native-synthetic-owner")
	m := b.nativePlayer()
	m.program = program
	// Images exercise the same native window placement without generating sound.
	image, _ := filepath.Abs("samples/assets/color-card.jpg")
	for i := 0; i < 3; i++ {
		if err := m.open(image, "image"); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		m.mu.Lock()
		sessions := []*nativePlayback{}
		for _, s := range m.sessions {
			sessions = append(sessions, s)
		}
		m.mu.Unlock()
		for _, s := range sessions {
			_ = s.send([]any{"quit"})
		}
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			m.mu.Lock()
			n := len(m.sessions)
			m.mu.Unlock()
			if n == 0 {
				return
			}
			time.Sleep(40 * time.Millisecond)
		}
		for _, s := range sessions {
			if s.cmd != nil && s.cmd.Process != nil {
				_ = s.cmd.Process.Kill()
			}
		}
	})
	var sessions []*nativePlayback
	nativeMediaWait(t, "three ready windows", func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		sessions = nil
		for _, s := range m.sessions {
			s.mu.Lock()
			ready := s.view.Connected && s.view.Width > 0 && s.view.Height > 0
			s.mu.Unlock()
			if !ready {
				return false
			}
			sessions = append(sessions, s)
		}
		return len(sessions) == 3
	})
	s := sessions[0]
	if err := s.request([]any{"set_property", "geometry", "720x500+40+40"}); err != nil {
		t.Fatal("geometry resize", err)
	}
	nativeMediaWait(t, "manual resize", func() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.view.Width == 720 && s.view.Height == 500 })
	nativeMediaWait(t, "remember resize", func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		return m.windowMemory["image"] == (nativeWindowMemory{720, 500})
	})
	if err := s.request([]any{"keypress", "WHEEL_UP"}); err != nil {
		t.Fatal(err)
	}
	var zoom float64
	json.Unmarshal(nativeMediaProperty(t, s, "video-zoom"), &zoom)
	if zoom <= 0 {
		t.Fatal("image wheel did not zoom", zoom)
	}
	if err := m.controlWindows("cascade"); err != nil {
		t.Fatal("cascade", err)
	}
	s.mu.Lock()
	w, h := s.view.Width, s.view.Height
	s.mu.Unlock()
	if w != 720 || h != 500 {
		t.Fatalf("cascade resized window: %dx%d", w, h)
	}
	if err := m.controlWindows("tile"); err != nil {
		t.Fatal("tile", err)
	}
	nativeMediaWait(t, "tile changed dimensions", func() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.view.Width != 720 || s.view.Height != 500 })
	if err := m.controlWindows("pause-all"); err != nil {
		t.Fatal(err)
	}
	if err := m.controlWindows("mute-all"); err != nil {
		t.Fatal(err)
	}
	restarted, _ := newFileBrowser(m.dataDir, "owner")
	if got := restarted.nativePlayer().windowMemory["image"]; got != (nativeWindowMemory{720, 500}) {
		t.Fatal("tiling overwrote manual size", got)
	}
	ffmpeg, _ := filepath.Abs("tools/ffmpeg/bin/ffmpeg.exe")
	clip := filepath.Join(t.TempDir(), "silent-demo.mp4")
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=c=blue:s=640x360:r=25:d=10", "-an", "-c:v", "libx264", "-y", clip).CombinedOutput(); err != nil {
		t.Fatal("synthetic silent video", err, string(out))
	}
	if err := m.open(clip, "video"); err != nil {
		t.Fatal(err)
	}
	var video *nativePlayback
	nativeMediaWait(t, "video metadata", func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		for _, s := range m.sessions {
			s.mu.Lock()
			ready := s.view.Kind == "video" && s.view.Connected && s.view.Duration > 0
			s.mu.Unlock()
			if ready {
				video = s
				return true
			}
		}
		return false
	})
	for _, command := range [][]any{{"set_property", "pause", true}, {"set_property", "volume", 32}, {"seek", 3, "absolute"}} {
		if err := video.request(command); err != nil {
			t.Fatal(err)
		}
	}
	nativeMediaWait(t, "individual video controls", func() bool {
		video.mu.Lock()
		defer video.mu.Unlock()
		return video.view.Paused && video.view.Volume == 32 && video.view.Time >= 2.9
	})
	video.mu.Lock()
	t.Logf("native video: decoder=%s, duration=%.1f, paused=%v, volume=%.0f, position=%.1f", video.view.Decoder, video.view.Duration, video.view.Paused, video.view.Volume, video.view.Time)
	video.mu.Unlock()
	t.Log("three native windows: image wheel zoom, remembered 720x500, cascade preserved size, tile and group controls acknowledged")
}
