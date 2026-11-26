package main

import (
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
)

func TestNativeWindowMemoryPersistenceAndPresetReset(t *testing.T) {
	dir := t.TempDir()
	b, _ := newFileBrowser(dir, "owner")
	m := b.nativePlayer()
	s := &nativePlayback{view: nativePlaybackView{Kind: "image", Width: 920, Height: 680}, windowDirty: true}
	m.rememberWindow(s)
	restarted, _ := newFileBrowser(dir, "owner")
	if got := restarted.nativePlayer().windowMemory["image"]; got != (nativeWindowMemory{920, 680}) {
		t.Fatal("image window not remembered", got)
	}
	if got := restarted.nativePlayer().windowMemory["video"]; got != (nativeWindowMemory{}) {
		t.Fatal("image size leaked into video preset")
	}
	if err := m.setRememberWindows(false); err != nil {
		t.Fatal(err)
	}
	s.view.Width = 1000
	s.windowDirty = true
	m.rememberWindow(s)
	if m.windowMemory["image"].Width != 920 {
		t.Fatal("disabled memory still changed")
	}
	if err := m.setRememberWindows(true); err != nil {
		t.Fatal(err)
	}
	s.fullscreen = true
	m.rememberWindow(s)
	if m.windowMemory["image"].Width != 920 {
		t.Fatal("fullscreen overwrote normal size")
	}
	s.fullscreen = false
	s.maximized = true
	m.rememberWindow(s)
	if m.windowMemory["image"].Width != 920 {
		t.Fatal("maximized size remembered")
	}
	s.maximized = false
	if err := m.setWindowSize("medium"); err != nil {
		t.Fatal(err)
	}
	m.rememberWindow(s)
	if len(m.windowMemory) != 0 {
		t.Fatal("old session overwrote preset reset")
	}
	s.memoryEpoch = m.memoryEpoch
	m.rememberWindow(s)
	if m.windowMemory["image"].Width != 1000 {
		t.Fatal("new session cannot remember size")
	}
	if err := m.configure(""); err != nil {
		t.Fatal(err)
	}
	if m.windowMemory["image"].Width != 1000 {
		t.Fatal("program detection lost memory")
	}
	args := strings.Join(replaceNativeFit(nativePlayerArgs("photo.jpg", "ipc", 0, "compact"), nativeWindowMemory{1000, 680}), "\n")
	if !strings.Contains(args, "--autofit=1000x680\n") || !strings.Contains(args, "--autofit-larger=90%x90%") {
		t.Fatal("remembered size lost screen cap", args)
	}
}

func nativeAcknowledgingSession(t *testing.T, id string, reject bool) *nativePlayback {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() { client.Close(); server.Close() })
	s := &nativePlayback{conn: client, view: nativePlaybackView{ID: id, Connected: true, Kind: "video"}}
	go func() {
		decoder := json.NewDecoder(server)
		for {
			var command struct {
				Command   []any
				RequestID int `json:"request_id"`
			}
			if decoder.Decode(&command) != nil {
				return
			}
			s.mu.Lock()
			ch := s.pending[command.RequestID]
			s.mu.Unlock()
			if ch != nil {
				if reject {
					ch <- errors.New("test rejected")
				} else {
					ch <- nil
				}
			}
		}
	}()
	return s
}
func TestNativeWindowLayoutAcknowledgmentAndIsolation(t *testing.T) {
	b, _ := newFileBrowser(t.TempDir(), "owner")
	m := b.nativePlayer()
	for _, action := range []string{"tile", "cascade", "pause-all", "mute-all"} {
		if err := m.controlWindows(action); err == nil {
			t.Fatal("empty layout falsely succeeded")
		}
	}
	m.sessions["one"] = nativeAcknowledgingSession(t, "one", false)
	m.sessions["two"] = nativeAcknowledgingSession(t, "two", false)
	for _, action := range []string{"tile", "cascade", "pause-all", "mute-all"} {
		if err := m.controlWindows(action); err != nil {
			t.Fatal(action, err)
		}
	}
	m.sessions["two"] = nativeAcknowledgingSession(t, "two", true)
	if err := m.controlWindows("tile"); err == nil || !strings.Contains(err.Error(), "已完成 1 个窗口，1 个未完成") {
		t.Fatal("partial failure hidden", err)
	}
	for _, count := range []int{1, 2, 6, 30, 100} {
		for index := 0; index < count; index++ {
			g := nativeLayoutGeometry(count, index, true)
			if strings.Contains(g, "0%x") || strings.Contains(g, "x0%") {
				t.Fatal("zero-size tile", g)
			}
		}
	}
	if g := nativeLayoutGeometry(4, 3, false); strings.Contains(g, "x") {
		t.Fatal("cascade must not resize", g)
	}
}

func TestNativeWindowExtendedActionsRejectInvalidValues(t *testing.T) {
	b, _ := newFileBrowser(t.TempDir(), "owner")
	m := b.nativePlayer()
	m.sessions["video"] = &nativePlayback{view: nativePlaybackView{ID: "video", Kind: "video"}}
	for _, body := range []string{`{"Action":"volume","ID":"video","Value":-1}`, `{"Action":"volume","ID":"video","Value":101}`, `{"Action":"zoom-in","ID":"video"}`, `{"Action":"tile","Command":"run"}`} {
		if w := nativeRequest(b, body, true); w.Code != 400 {
			t.Fatal("unsafe action accepted", body, w.Code)
		}
	}
}
