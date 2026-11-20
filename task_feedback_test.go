package main

import (
	"encoding/json"
	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
	"testing"
)

func TestTaskNotificationSettingsPersistence(t *testing.T) {
	s := &server{token: "test-session", dataDir: t.TempDir()}
	p, err := s.loadPreferences()
	if err != nil || p.TaskPopup != nil || p.TaskSound != nil {
		t.Fatal("legacy settings should default to enabled in the UI", p, err)
	}
	for _, change := range []map[string]any{
		{"SidebarCollapsed": true}, {"TaskSound": false}, {"TaskPopup": false},
	} {
		w := requestAPI(t, s, "/api/ui-settings", change)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	// A workspace save and later sidebar update must not reset notification options.
	c := toolbox.DefaultConfig()
	c.Tool = "format"
	if _, err = s.saveWorkspace(c); err != nil {
		t.Fatal(err)
	}
	w := requestAPI(t, s, "/api/ui-settings", map[string]any{"SidebarCollapsed": false})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	restarted := &server{dataDir: s.dataDir}
	p, err = restarted.loadPreferences()
	if err != nil || p.SidebarCollapsed || p.TaskSound == nil || *p.TaskSound || p.TaskPopup == nil || *p.TaskPopup || len(p.Workspaces) != 1 {
		t.Fatal("settings were overwritten or did not survive restart", p, err)
	}
	before, _ := json.Marshal(p)
	w = requestAPI(t, s, "/api/ui-settings", map[string]any{"TaskSound": "false"})
	if w.Code == 200 {
		t.Fatal("invalid settings accepted")
	}
	p, _ = s.loadPreferences()
	after, _ := json.Marshal(p)
	if string(before) != string(after) {
		t.Fatal("invalid input changed settings")
	}
}
