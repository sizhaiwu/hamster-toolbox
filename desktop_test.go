package main

import (
	"testing"
)

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
