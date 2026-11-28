package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Uses generated color bars, tones and subtitles; never opens user media.
func TestNativeMediaRealPlayer(t *testing.T) {
	if os.Getenv("TOOLBOX_NATIVE_MEDIA_INTEGRATION") != "1" {
		t.Skip("set TOOLBOX_NATIVE_MEDIA_INTEGRATION=1 for real mpv acceptance")
	}
	if runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Skip("native display required")
	}
	name := "mpv"
	ffname := "ffmpeg"
	if runtime.GOOS == "windows" {
		name += ".exe"
		ffname += ".exe"
	}
	program, e := filepath.Abs(filepath.Join("tools", "mpv", name))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(program); e != nil {
		program, e = exec.LookPath(name)
		if e != nil {
			t.Skip("mpv not installed")
		}
	}
	ffmpeg, _ := filepath.Abs(filepath.Join("tools", "ffmpeg", "bin", ffname))
	if _, e = os.Stat(ffmpeg); e != nil {
		ffmpeg, e = exec.LookPath(ffname)
		if e != nil {
			t.Skip("ffmpeg not installed")
		}
	}
	data := t.TempDir()
	sub := filepath.Join(data, "demo.srt")
	os.WriteFile(sub, []byte("1\n00:00:00,000 --> 00:00:11,000\nSynthetic subtitle\n"), 0600)
	file := filepath.Join(data, "synthetic.mkv")
	t.Cleanup(func() {
		for i := 0; i < 30; i++ {
			e := os.Remove(file)
			if e == nil || os.IsNotExist(e) {
				return
			}
			time.Sleep(80 * time.Millisecond)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=12", "-f", "lavfi", "-i", "sine=frequency=440", "-f", "lavfi", "-i", "sine=frequency=880", "-i", sub, "-map", "0:v", "-map", "1:a", "-map", "2:a", "-map", "3:s", "-t", "12", "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", "-c:s", "srt", "-y", file)
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatal(e, string(out))
	}
	m := &nativePlayerManager{program: program, dataDir: filepath.Join(data, "settings"), windowSize: "compact", sessions: map[string]*nativePlayback{}}
	t.Cleanup(func() {
		for _, s := range m.mediaSessions() {
			_ = s.send([]any{"quit"})
		}
		deadline := time.Now().Add(3 * time.Second)
		for len(m.mediaSessions()) > 0 && time.Now().Before(deadline) {
			time.Sleep(40 * time.Millisecond)
		}
	})
	if e = m.setMediaPreference("remember-media", true); e != nil {
		t.Fatal(e)
	}
	wait := func(label string, predicate func() bool) {
		t.Helper()
		until := time.Now().Add(12 * time.Second)
		for time.Now().Before(until) {
			if predicate() {
				return
			}
			time.Sleep(80 * time.Millisecond)
		}
		t.Fatal(label, m.status())
	}
	if e = m.open(file, "video"); e != nil {
		t.Fatal(e)
	}
	var first *nativePlayback
	wait("initial playback", func() bool {
		sessions := m.mediaSessions()
		if len(sessions) != 1 {
			return false
		}
		first = sessions[0]
		first.mu.Lock()
		defer first.mu.Unlock()
		return first.view.Connected && first.view.Duration > 10 && len(first.view.Tracks) >= 4
	})

	var layout nativeControlLayout
	wait("native controller loaded", func() bool {
		result, err := nativeTestProperty(first, "user-data/hamster-controls")
		if err != nil {
			return false
		}
		return json.Unmarshal(result, &layout) == nil && layout.Mode != ""
	})
	for _, mode := range []string{"full"} {
		if e = m.setControlsMode(mode); e != nil {
			t.Fatal(e)
		}
		if e = first.request([]any{"script-message", "hamster-controls-show", "yes"}); e != nil {
			t.Fatal(e)
		}
		for _, size := range []struct{ w, h int }{{240, 400}, {320, 400}, {800, 600}, {1280, 700}} {
			if e = first.request([]any{"set_property", "geometry", fmt.Sprintf("%dx%d", size.w, size.h)}); e != nil {
				t.Fatal(e)
			}
			wait("responsive bottom controls "+mode, func() bool {
				result, err := nativeTestProperty(first, "user-data/hamster-controls")
				if err != nil || json.Unmarshal(result, &layout) != nil {
					return false
				}
				return layout.Mode == mode && layout.Bottom && mathAbs(layout.Width*layout.Scale-float64(size.w)) < 10
			})
			var volumeControl bool
			for _, control := range layout.Controls {
				if control.ID == "mute" {
					volumeControl = true
				}
				if control.X < -1 || control.X+control.W > layout.Width*layout.Scale+1 || control.Y < 0 || control.Y+control.H > layout.Height*layout.Scale+1 {
					t.Fatal("controls overflow", mode, size, control, layout)
				}
				if control.ID != "minimize" && control.ID != "maximize" && control.ID != "close" && control.ID != "volume" && control.Y < (layout.Height-layout.Bar)*layout.Scale-1 {
					t.Fatal("controller escaped its bottom strip", control)
				}
			}
			if !volumeControl || layout.Font < 14 {
				t.Fatal("volume or readable fonts missing", layout)
			}

			if layout.Bar != 48 {
				t.Fatal("video controls must use one compact row", layout)
			}
			var muteRow nativeControlRect
			for _, control := range layout.Controls {
				if control.ID == "mute" {
					muteRow = control
				}
				if control.ID == "back" || control.ID == "forward" || control.ID == "audio" || control.ID == "subtitle" {
					t.Fatal("removed control still displayed", control)
				}
			}
			var extensions int
			for _, control := range layout.Controls {
				if control.ID == "speed" || control.ID == "fullscreen" {
					extensions++
					if control.Y != muteRow.Y || control.X < muteRow.X+muteRow.W {
						t.Fatal("extra controls must sit beside volume", layout)
					}
				}
			}
			if mode == "full" && layout.Width >= 220 && extensions != 2 {
				t.Fatal("compact full controls missing", layout)
			}
			if mode == "full" && size.w == 320 {
				vb := nativeTestVolumeSlider(t, first)
				nativeTestSetVolume(t, first, vb, 45)
				wait("compact volume slider", func() bool {
					r, e := nativeTestProperty(first, "volume")
					var v float64
					return e == nil && json.Unmarshal(r, &v) == nil && mathAbs(v-45) < 1
				})
				nativeTestClick(t, first, muteRow)
				wait("compact mute", func() bool {
					r, e := nativeTestProperty(first, "mute")
					var v bool
					return e == nil && json.Unmarshal(r, &v) == nil && v
				})
				_ = first.request([]any{"set_property", "mute", false})
			}
			for _, property := range []string{"video-margin-ratio-top", "video-margin-ratio-bottom"} {
				result, err := nativeTestProperty(first, property)
				var margin float64
				if err != nil || json.Unmarshal(result, &margin) != nil || margin != 0 {
					t.Fatal("controller changed the video viewport", property, string(result), err)
				}
			}
		}

		shown := nativeTestVideoBounds(t, first)
		_ = first.request([]any{"script-message", "hamster-controls-show", "no"})
		_ = first.request([]any{"mouse", 600, 300})
		wait("controller hides without resizing video", func() bool {
			r, e := nativeTestProperty(first, "user-data/hamster-controls")
			return e == nil && json.Unmarshal(r, &layout) == nil && !layout.Bottom && !layout.Top
		})
		hidden := nativeTestVideoBounds(t, first)
		if shown != hidden {
			t.Fatal("control visibility changed displayed video bounds", mode, shown, hidden)
		}
		_ = first.request([]any{"script-message", "hamster-controls-show", "yes"})
		wait("controller restored", func() bool {
			r, e := nativeTestProperty(first, "user-data/hamster-controls")
			return e == nil && json.Unmarshal(r, &layout) == nil && layout.Bottom && layout.Top
		})
	}
	// Test the real controls through mpv's internal mouse input, never the OS cursor.
	if e = first.request([]any{"set_property", "volume", 40}); e != nil {
		t.Fatal(e)
	}
	volumeButton := nativeTestVolumeSlider(t, first)
	nativeTestSetVolume(t, first, volumeButton, 70)
	wait("volume slider changes real volume", func() bool {
		r, e := nativeTestProperty(first, "volume")
		var v float64
		return e == nil && json.Unmarshal(r, &v) == nil && v > 65 && v < 75
	})

	_ = first.request([]any{"set_property", "pause", true})
	for _, control := range layout.Controls {
		if control.ID == "seek" {
			_ = first.request([]any{"mouse", int(control.X + control.W*0.2), int(control.Y + control.H/2)})
			_ = first.request([]any{"keydown", "MBTN_LEFT"})
			_ = first.request([]any{"mouse", int(control.X + control.W*0.4), int(control.Y + control.H/2)})
			_ = first.request([]any{"keyup", "MBTN_LEFT"})
		}
	}
	wait("seek strip still changes playback position", func() bool {
		r, e := nativeTestProperty(first, "time-pos")
		var v float64
		return e == nil && json.Unmarshal(r, &v) == nil && v > 4.4 && v < 5.3
	})
	var playControl nativeControlRect
	for _, control := range layout.Controls {
		if control.ID == "play" {
			playControl = control
		}
	}
	if playControl.W == 0 {
		t.Fatal("play button missing")
	}
	nativeTestClick(t, first, playControl)
	wait("play button resumes without restarting paused video", func() bool {
		r, e := nativeTestProperty(first, "pause")
		var paused bool
		position, pe := nativeTestProperty(first, "time-pos")
		var pos float64
		return e == nil && json.Unmarshal(r, &paused) == nil && !paused && pe == nil && json.Unmarshal(position, &pos) == nil && pos > 4.4 && pos < 7
	})
	nativeTestClick(t, first, playControl)
	wait("play button pauses normally", func() bool {
		r, e := nativeTestProperty(first, "pause")
		var paused bool
		return e == nil && json.Unmarshal(r, &paused) == nil && paused
	})
	// Let the synthetic video reach EOF with keep-open, then use the actual button.
	if e = first.request([]any{"seek", 11.7, "absolute+exact"}); e != nil {
		t.Fatal(e)
	}
	_ = first.request([]any{"set_property", "pause", false})
	wait("video reaches end and remains open", func() bool {
		r, e := nativeTestProperty(first, "eof-reached")
		var ended bool
		return e == nil && json.Unmarshal(r, &ended) == nil && ended
	})
	nativeTestClick(t, first, playControl)
	wait("play button restarts ended video", func() bool {
		r, e := nativeTestProperty(first, "eof-reached")
		var ended bool
		pausedData, pauseErr := nativeTestProperty(first, "pause")
		var paused bool
		position, positionErr := nativeTestProperty(first, "time-pos")
		var pos float64
		return e == nil && json.Unmarshal(r, &ended) == nil && !ended && pauseErr == nil && json.Unmarshal(pausedData, &paused) == nil && !paused && positionErr == nil && json.Unmarshal(position, &pos) == nil && pos > 0 && pos < 3
	})
	nativeTestABRepeat(t, m, first, file, wait)
	if os.Getenv("TOOLBOX_NATIVE_MEDIA_SNAPSHOT") == "1" {
		for _, mode := range []string{"full"} {
			_ = m.setControlsMode(mode)
			_ = first.request([]any{"set_property", "geometry", "800x600"})
			_ = first.request([]any{"set_property", "pause", true})
			wait("snapshot controls", func() bool {
				r, e := nativeTestProperty(first, "user-data/hamster-controls")
				return e == nil && json.Unmarshal(r, &layout) == nil && layout.Mode == mode && mathAbs(layout.Width*layout.Scale-800) < 10
			})
			nativeTestVolumeSlider(t, first)
			output, _ := filepath.Abs(filepath.Join("work", "native-controls-"+mode+".png"))
			if e = first.request([]any{"screenshot-to-file", output, "window"}); e != nil {
				t.Fatal(e)
			}
		}
	}
	if e = first.request([]any{"script-message", "hamster-controls-show", "no"}); e != nil {
		t.Fatal(e)
	}
	// Move away from both hot areas; hiding must leave video geometry untouched.
	_ = first.request([]any{"mouse", 600, 300})
	wait("hidden controller preserves video viewport", func() bool {
		r, e := nativeTestProperty(first, "video-margin-ratio-bottom")
		var v float64
		return e == nil && json.Unmarshal(r, &v) == nil && v == 0
	})
	first.mu.Lock()
	var audio, subtitle int
	for _, track := range first.view.Tracks {
		if track.Type == "audio" {
			audio = track.ID
		}
		if track.Type == "sub" {
			subtitle = track.ID
		}
	}
	id := first.view.ID
	first.mu.Unlock()
	for _, c := range []struct {
		action string
		value  float64
		track  int
	}{{"speed", 1.5, 0}, {"audio-track", 0, audio}, {"subtitle-track", 0, subtitle}} {
		if e = m.mediaAction(c.action, id, c.value, c.track, false); e != nil {
			t.Fatal(e)
		}
	}
	if e = first.send([]any{"seek", 5, "absolute"}); e != nil {
		t.Fatal(e)
	}
	_ = first.send([]any{"set_property", "volume", 35})
	wait("updated speed and seek", func() bool {
		first.mu.Lock()
		defer first.mu.Unlock()
		return first.view.Speed == 1.5 && first.view.Time >= 4 && first.view.Time < 10 && first.view.Volume == 35
	})
	if e = first.send([]any{"quit"}); e != nil {
		t.Fatal(e)
	}
	wait("player exit", func() bool { return len(m.mediaSessions()) == 0 })
	entries, e := os.ReadDir(filepath.Join(m.dataDir, "native-watch-later"))
	if e != nil || len(entries) == 0 {
		t.Fatal("resume record missing", e)
	}
	for _, entry := range entries {
		if watchLaterFile(entry.Name()) {
			b, e := os.ReadFile(filepath.Join(m.dataDir, "native-watch-later", entry.Name()))
			if e != nil || strings.Contains(string(b), file) {
				t.Fatal("resume record exposes path", e)
			}
		}
	}
	if e = m.open(file, "video"); e != nil {
		t.Fatal(e)
	}
	var second *nativePlayback
	wait("restored position and preferences", func() bool {
		sessions := m.mediaSessions()
		if len(sessions) != 1 {
			return false
		}
		second = sessions[0]
		second.mu.Lock()
		defer second.mu.Unlock()
		return second.view.Connected && second.view.Time >= 4 && second.view.Speed == 1.5 && second.view.Volume == 35
	})
	_ = second.send([]any{"quit"})
	wait("second player exit", func() bool { return len(m.mediaSessions()) == 0 })
	if e = m.cleanMediaHistory(true); e != nil {
		t.Fatal(e)
	}
	// Still images keep native zoom/rotation controls, without video-only widgets.
	still := filepath.Join(data, "synthetic.png")
	out, e := os.Create(still)
	if e != nil {
		t.Fatal(e)
	}
	if e = png.Encode(out, image.NewRGBA(image.Rect(0, 0, 80, 60))); e != nil {
		out.Close()
		t.Fatal(e)
	}
	out.Close()
	if e = m.open(still, "image"); e != nil {
		t.Fatal(e)
	}
	var imageSession *nativePlayback
	wait("native image controller", func() bool {
		sessions := m.mediaSessions()
		if len(sessions) != 1 {
			return false
		}
		imageSession = sessions[0]
		r, e := nativeTestProperty(imageSession, "user-data/hamster-controls")
		return e == nil && json.Unmarshal(r, &layout) == nil && layout.Kind == "image"
	})
	_ = imageSession.request([]any{"set_property", "geometry", "640x480"})
	_ = imageSession.request([]any{"script-message", "hamster-controls-show", "yes"})
	wait("image controls ready", func() bool {
		r, e := nativeTestProperty(imageSession, "user-data/hamster-controls")
		if e != nil || json.Unmarshal(r, &layout) != nil || mathAbs(layout.Width*layout.Scale-640) > 10 || mathAbs(layout.Height*layout.Scale-480) > 10 {
			return false
		}
		for _, c := range layout.Controls {
			if c.ID == "image-in" {
				return true
			}
		}
		return false
	})
	for _, control := range layout.Controls {
		if control.ID == "volume" || control.ID == "seek" {
			t.Fatal("video controls leaked into image view", layout)
		}
		if control.ID == "image-in" {
			nativeTestClick(t, imageSession, control)
		}
	}
	wait("image zoom still works", func() bool {
		r, e := nativeTestProperty(imageSession, "video-zoom")
		var v float64
		return e == nil && json.Unmarshal(r, &v) == nil && v > 0
	})
	for _, control := range layout.Controls {
		if control.ID == "image-rotate" {
			nativeTestClick(t, imageSession, control)
		}
	}
	wait("image rotate still works", func() bool {
		r, e := nativeTestProperty(imageSession, "video-rotate")
		var v float64
		return e == nil && json.Unmarshal(r, &v) == nil && v == 90
	})

	for _, control := range layout.Controls {
		if control.ID == "close" {
			_ = imageSession.request([]any{"mouse", int(control.X + control.W/2), int(control.Y + control.H/2)})
			_ = imageSession.request([]any{"keydown", "MBTN_LEFT"})
			_ = imageSession.send([]any{"keyup", "MBTN_LEFT"})
		}
	}

	wait("image player exit", func() bool { return len(m.mediaSessions()) == 0 })
	nativeTestEndActions(t, m, file, still, wait)
}

func nativeTestEndActions(t *testing.T, m *nativePlayerManager, file, still string, wait func(string, func() bool)) {
	t.Helper()
	if err := m.setMediaPreference("remember-media", false); err != nil {
		t.Fatal(err)
	}
	open := func(action, kind, path string) *nativePlayback {
		t.Helper()
		if err := m.setEndAction(action); err != nil {
			t.Fatal(err)
		}
		if err := m.open(path, kind); err != nil {
			t.Fatal(err)
		}
		var opened *nativePlayback
		wait("end action window "+action+" "+kind, func() bool {
			for _, candidate := range m.mediaSessions() {
				candidate.mu.Lock()
				ready := candidate.view.Kind == kind && candidate.view.Connected && (kind == "image" && candidate.view.VideoWidth > 0 || kind == "video" && candidate.view.Duration > 10)
				candidate.mu.Unlock()
				if ready {
					opened = candidate
					return true
				}
			}
			return false
		})
		return opened
	}
	ending := func(s *nativePlayback) {
		t.Helper()
		if err := s.request([]any{"seek", 11.7, "absolute+exact"}); err != nil {
			t.Fatal(err)
		}
		if err := s.request([]any{"set_property", "pause", false}); err != nil {
			t.Fatal(err)
		}
	}
	keep := open("keep", "video", file)
	ending(keep)
	wait("keep action holds last frame", func() bool {
		data, err := nativeTestProperty(keep, "eof-reached")
		return err == nil && string(data) == "true" && len(m.mediaSessions()) == 1
	})
	if err := keep.request([]any{"quit"}); err != nil {
		t.Fatal(err)
	}
	wait("keep fixture closed", func() bool { return len(m.mediaSessions()) == 0 })
	loop := open("loop", "video", file)
	nativeTestABRepeat(t, m, loop, file, wait)
	ending(loop)
	wait("loop action restarts the complete video", func() bool {
		data, err := nativeTestProperty(loop, "time-pos")
		var value float64
		return err == nil && json.Unmarshal(data, &value) == nil && value > 0 && value < 2
	})
	if err := loop.request([]any{"quit"}); err != nil {
		t.Fatal(err)
	}
	wait("loop fixture closed", func() bool { return len(m.mediaSessions()) == 0 })
	image := open("close", "image", still)
	if data, err := nativeTestProperty(image, "keep-open"); err != nil || string(data) != "true" {
		t.Fatal("close action changed image behavior", string(data), err)
	}
	closeVideo := open("close", "video", file)
	ending(closeVideo)
	wait("close action closes only the finished video", func() bool {
		sessions := m.mediaSessions()
		return len(sessions) == 1 && sessions[0] == image
	})
	if err := image.request([]any{"quit"}); err != nil {
		t.Fatal(err)
	}
	wait("end action fixtures closed", func() bool { return len(m.mediaSessions()) == 0 })
}

func nativeTestABRepeat(t *testing.T, m *nativePlayerManager, s *nativePlayback, file string, wait func(string, func() bool)) {
	t.Helper()
	command := func(args ...any) {
		t.Helper()
		if err := s.request(args); err != nil {
			t.Fatal(err)
		}
	}
	point := func(name string, expected float64) bool {
		data, err := nativeTestProperty(s, "ab-loop-"+name)
		var value float64
		return err == nil && json.Unmarshal(data, &value) == nil && mathAbs(value-expected) < 0.02
	}
	unset := func(session *nativePlayback, name string) bool {
		data, err := nativeTestProperty(session, "ab-loop-"+name)
		return err == nil && string(data) == `"no"`
	}
	seek := func(pos float64) {
		t.Helper()
		command("seek", pos, "absolute+exact")
		wait("AB fixture seek", func() bool {
			data, err := nativeTestProperty(s, "time-pos")
			var value float64
			return err == nil && json.Unmarshal(data, &value) == nil && mathAbs(value-pos) < 0.02
		})
	}
	command("set_property", "pause", true)
	seek(2)
	command("keypress", "Ctrl+2")
	wait("B without A stays disabled", func() bool { return unset(s, "a") && unset(s, "b") })
	command("keypress", "Ctrl+1")
	wait("Ctrl+1 sets A", func() bool { return point("a", 2) && unset(s, "b") })
	command("keypress", "Ctrl+2")
	wait("B equal to A is rejected", func() bool { return point("a", 2) && unset(s, "b") })
	seek(1)
	command("keypress", "Ctrl+2")
	wait("B before A is rejected", func() bool { return point("a", 2) && unset(s, "b") })
	seek(3)
	command("keypress", "Ctrl+2")
	wait("Ctrl+2 sets B", func() bool { return point("a", 2) && point("b", 3) })
	data, err := nativeTestProperty(s, "pause")
	if err != nil || string(data) != "true" {
		t.Fatal("setting AB while paused started playback", string(data), err)
	}
	bounds := nativeTestVideoBounds(t, s)
	command("seek", 2, "absolute+exact")
	command("set_property", "pause", false)
	previous, loops := 2.0, 0
	wait("mpv loops back from B to A repeatedly", func() bool {
		data, err := nativeTestProperty(s, "time-pos")
		var value float64
		if err != nil || json.Unmarshal(data, &value) != nil {
			return false
		}
		if value < 1.9 || value > 3.2 {
			t.Fatal("AB playback escaped its range", value)
		}
		if previous > 2.7 && value < 2.3 {
			loops++
		}
		previous = value
		return loops >= 2
	})
	command("set_property", "pause", true)
	if bounds != nativeTestVideoBounds(t, s) {
		t.Fatal("AB controls changed video geometry")
	}
	if os.Getenv("TOOLBOX_NATIVE_MEDIA_SNAPSHOT") == "1" {
		command("show-text", "")
		output, _ := filepath.Abs(filepath.Join("work", "native-controls-ab-loop.png"))
		command("screenshot-to-file", output, "window")
	}
	// Opening another native window must not inherit this window's loop points.
	if err := m.open(file, "video"); err != nil {
		t.Fatal(err)
	}
	var other *nativePlayback
	wait("second AB fixture window", func() bool {
		for _, candidate := range m.mediaSessions() {
			if candidate != s {
				candidate.mu.Lock()
				connected := candidate.view.Connected
				candidate.mu.Unlock()
				if connected {
					other = candidate
					return true
				}
			}
		}
		return false
	})
	if !unset(other, "a") || !unset(other, "b") || !point("a", 2) || !point("b", 3) {
		t.Fatal("AB state leaked between native windows")
	}
	if err := other.request([]any{"quit"}); err != nil {
		t.Fatal(err)
	}
	wait("second AB fixture closed", func() bool { return len(m.mediaSessions()) == 1 })
	seek(2.5)
	command("keypress", "Ctrl+1")
	wait("new A resets the old B", func() bool { return point("a", 2.5) && unset(s, "b") })
	command("keypress", "Ctrl+3")
	wait("Ctrl+3 clears both loop points", func() bool { return unset(s, "a") && unset(s, "b") })
	seek(0)
	command("keypress", "Ctrl+1")
	wait("A accepts the start of the file", func() bool { return point("a", 0) && unset(s, "b") })
	seek(0.5)
	command("keypress", "Ctrl+2")
	wait("sub-second B works with A at zero", func() bool { return point("a", 0) && point("b", 0.5) })
	command("keypress", "Ctrl+3")
	wait("zero-based range can be cleared", func() bool { return unset(s, "a") && unset(s, "b") })
	seek(3.5)
	command("set_property", "pause", false)
	wait("normal playback continues past old B", func() bool {
		data, err := nativeTestProperty(s, "time-pos")
		var pos float64
		return err == nil && json.Unmarshal(data, &pos) == nil && pos > 3.6 && pos < 5
	})
}

type nativeControlRect struct {
	ID         string
	X, Y, W, H float64
}
type nativeControlLayout struct {
	Kind                            string
	Mode                            string
	Width, Height, Font, Scale, Bar float64
	Bottom, Top                     bool
	Controls                        []nativeControlRect
}

func mathAbs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
func nativeTestProperty(s *nativePlayback, name string) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := dialNativePlayer(ctx, s.endpoint)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	if err = json.NewEncoder(conn).Encode(map[string]any{"command": []any{"get_property", name}, "request_id": 9001}); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(conn)
	for {
		var result struct {
			RequestID int             `json:"request_id"`
			Error     string          `json:"error"`
			Data      json.RawMessage `json:"data"`
		}
		if err = decoder.Decode(&result); err != nil {
			return nil, err
		}
		if result.RequestID == 9001 {
			if result.Error != "success" {
				return nil, fmt.Errorf("%s: %s", name, result.Error)
			}
			return result.Data, nil
		}
	}
}

func nativeTestClick(t *testing.T, s *nativePlayback, b nativeControlRect) {
	t.Helper()
	for _, cmd := range [][]any{{"mouse", int(b.X + b.W/2), int(b.Y + b.H/2)}, {"keydown", "MBTN_LEFT"}, {"keyup", "MBTN_LEFT"}} {
		if err := s.request(cmd); err != nil {
			t.Fatal(err)
		}
	}
}

type nativeVideoBounds struct{ W, H, ML, MR, MT, MB float64 }

func nativeTestVideoBounds(t *testing.T, s *nativePlayback) nativeVideoBounds {
	t.Helper()
	r, e := nativeTestProperty(s, "osd-dimensions")
	var bounds nativeVideoBounds
	if e != nil || json.Unmarshal(r, &bounds) != nil {
		t.Fatal("cannot read displayed video bounds", string(r), e)
	}
	return bounds
}

func nativeTestVolumeSlider(t *testing.T, s *nativePlayback) nativeControlRect {
	t.Helper()
	r, e := nativeTestProperty(s, "user-data/hamster-controls")
	var layout nativeControlLayout
	if e != nil || json.Unmarshal(r, &layout) != nil {
		t.Fatal("controller unavailable", e)
	}
	for _, c := range layout.Controls {
		if c.ID == "mute" {
			if e = s.request([]any{"mouse", int(c.X + c.W/2), int(c.Y + c.H/2)}); e != nil {
				t.Fatal(e)
			}
		}
	}
	until := time.Now().Add(2 * time.Second)
	for time.Now().Before(until) {
		r, e = nativeTestProperty(s, "user-data/hamster-controls")
		if e == nil && json.Unmarshal(r, &layout) == nil {
			for _, c := range layout.Controls {
				if c.ID == "volume" {
					return c
				}
			}
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("hover volume popover missing", layout)
	return nativeControlRect{}
}
func nativeTestSetVolume(t *testing.T, s *nativePlayback, b nativeControlRect, value float64) {
	t.Helper()
	x := int(b.X + b.W/2)
	for _, cmd := range [][]any{{"mouse", x, int(b.Y + b.H*0.9)}, {"keydown", "MBTN_LEFT"}, {"mouse", x, int(b.Y + b.H*(1-value/100))}, {"keyup", "MBTN_LEFT"}} {
		if e := s.request(cmd); e != nil {
			t.Fatal(e)
		}
	}
}
