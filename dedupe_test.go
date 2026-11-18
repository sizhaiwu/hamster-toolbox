package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

func TestDedupeAPIAndWorkspace(t *testing.T) {
	s := &server{token: "test-session", dataDir: t.TempDir()}
	path := filepath.Join(t.TempDir(), "VDF.GUI.exe")
	w := requestAPI(t, s, "/api/dedupe-info", map[string]string{"Path": path})
	var info toolbox.DedupeInfo
	if e := json.Unmarshal(w.Body.Bytes(), &info); e != nil || info.Available {
		t.Fatal(info, e)
	}
	if w = requestAPI(t, s, "/api/dedupe-open", map[string]string{"Path": path}); w.Code == 200 {
		t.Fatal("missing executable launched")
	}
	// A named file can be selected, but invalid executable data must fail safely.
	if e := os.WriteFile(path, []byte("not an executable"), 0600); e != nil {
		t.Fatal(e)
	}
	w = requestAPI(t, s, "/api/dedupe-open", map[string]string{"Path": path})
	if w.Code == 200 {
		t.Fatal("invalid executable launched")
	}
	if _, e := s.saveWorkspace(toolbox.Config{Tool: "dedupe", DedupeProgram: path, Root: "unrelated", ExtractPassword: "unrelated"}); e != nil {
		t.Fatal(e)
	}
	p, e := s.loadPreferences()
	if e != nil || p.LastTool != "dedupe" || p.Workspaces["dedupe"].DedupeProgram != path || p.Workspaces["dedupe"].Root != "" || p.Workspaces["dedupe"].ExtractPassword != "" {
		t.Fatal(p, e)
	}
}
