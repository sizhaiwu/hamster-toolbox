package main

import (
	"net/url"
	"testing"
)

func TestDesktopTrayDestinations(t *testing.T) {
	base := "http://127.0.0.1:28741/notes/?session=synthetic-token&other=ignored#previous"
	seen := map[int]bool{}
	want := map[int]string{1: "/", 2: "/", 3: "/files/", 4: "/notes/", 5: "/schedules.html", 6: "/"}
	for _, action := range desktopTrayActions {
		address, browser, err := desktopTrayDestination(base, action.ID)
		if err != nil || seen[action.ID] {
			t.Fatal(action, err)
		}
		seen[action.ID] = true
		u, err := url.Parse(address)
		if err != nil || u.Host != "127.0.0.1:28741" || u.Path != want[action.ID] || u.Query().Get("session") != "synthetic-token" || len(u.Query()) != 1 || browser != (action.ID == 2) {
			t.Fatal("tray destination changed server, session or mode", action, address, err)
		}
		if (u.Fragment == "settings") != (action.ID == 6) || action.ID != 6 && u.Fragment != "" {
			t.Fatal("wrong settings intent", address)
		}
	}
	if len(seen) != len(want) {
		t.Fatal("missing tray entry", seen)
	}
	for _, address := range []string{"http://127.0.0.1:28741/", "https://example.com/?session=x", "file:///local", "http://user@127.0.0.1:28741/?session=x"} {
		if _, _, err := desktopTrayDestination(address, 1); err == nil {
			t.Fatal("unsafe tray address accepted", address)
		}
	}
	if _, _, err := desktopTrayDestination(base, 100); err == nil {
		t.Fatal("exit command treated as navigation")
	}
}

func TestDesktopExitDoesNotInterruptTasks(t *testing.T) {
	s := &server{}
	for _, status := range []string{"running", "scanning"} {
		s.state.Status = status
		if s.desktopCanExit() || s.beginDesktopExit() || s.desktopClosing {
			t.Fatal("task allowed tray exit", status)
		}
	}
	s.state.Status = "finished"
	if !s.desktopCanExit() || !s.beginDesktopExit() || !s.desktopClosing {
		t.Fatal("idle exit failed")
	}
	w := requestAPI(t, s, "/api/plan", map[string]any{"Tool": "image", "Root": t.TempDir()})
	if w.Code == 200 {
		t.Fatal("new task started during shutdown")
	}
}

func TestDesktopInstanceUsesDataDirectory(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	if desktopInstanceName(a) != desktopInstanceName(a) || desktopInstanceName(a) == desktopInstanceName(b) {
		t.Fatal("instance identity is unstable")
	}
}
