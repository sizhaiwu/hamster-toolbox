package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

func toolRequest(t *testing.T, s *server, tool, path string, data any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/api/"+path, bytes.NewReader(b))
	r.Header.Set("X-Toolbox-Tool", tool)
	w := httptest.NewRecorder()
	s.api(w, r)
	return w
}
func toolSnapshot(t *testing.T, s *server, tool string) state {
	t.Helper()
	w := toolRequest(t, s, tool, "state", map[string]bool{"Summary": true})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var st state
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	return st
}
func waitTool(t *testing.T, s *server, tool, status string) state {
	t.Helper()
	end := time.Now().Add(5 * time.Second)
	for time.Now().Before(end) {
		st := toolSnapshot(t, s, tool)
		if st.Status == status {
			return st
		}
		if st.Status == "error" {
			t.Fatal(st.Error)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s did not become %s", tool, status)
	return state{}
}
func TestIndependentToolTasks(t *testing.T) {
	s := &server{dataDir: t.TempDir()}
	started := make(chan string, 2)
	release := make(chan struct{})
	s.taskExecutor = func(ctx context.Context, p *toolbox.Plan, log func(string), progress func(int, int), observe func(toolbox.FileOutcome)) toolbox.Result {
		log("isolated " + p.Config.Tool)
		progress(0, len(p.Items))
		started <- p.Config.Tool
		select {
		case <-release:
		case <-ctx.Done():
		}
		return toolbox.ExecuteObserved(ctx, p, log, progress, observe)
	}
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		for _, slot := range s.taskSlots() {
			slot.mu.Lock()
			if slot.cancel != nil {
				slot.cancel()
			}
			slot.mu.Unlock()
		}
		s.taskWait.Wait()
	})
	configs := map[string]toolbox.Config{}
	for _, tool := range []string{"format", "rename"} {
		c := toolbox.DefaultConfig()
		c.Tool, c.Root = tool, t.TempDir()
		c.Recursive = false
		if tool == "format" {
			if err := os.Mkdir(filepath.Join(c.Root, "album"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(c.Root, "album", "a.jpg"), []byte("sample"), 0600); err != nil {
				t.Fatal(err)
			}
		} else {
			c.Find, c.Replace, c.Files, c.Dirs = "old", "new", true, false
			if err := os.WriteFile(filepath.Join(c.Root, "old.txt"), []byte("sample"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		configs[tool] = c
		w := toolRequest(t, s, tool, "plan", c)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		var accepted struct{ TaskID string }
		if err := json.Unmarshal(w.Body.Bytes(), &accepted); err != nil || accepted.TaskID == "" {
			t.Fatal("scan admission must identify the accepted task", w.Body.String(), err)
		}
		if waitTool(t, s, tool, "ready").TaskID != accepted.TaskID {
			t.Fatal("scan response and ready plan refer to different tasks")
		}
	}
	fmtState := toolSnapshot(t, s, "format")
	renameState := toolSnapshot(t, s, "rename")
	if fmtState.PlanID == renameState.PlanID || fmtState.Total != 1 || renameState.Total != 1 {
		t.Fatal(fmtState, renameState)
	}
	if w := toolRequest(t, s, "rename", "execute", map[string]string{"PlanID": fmtState.PlanID}); w.Code == 200 {
		t.Fatal("cross-tool plan accepted")
	}
	for _, tool := range []string{"format", "rename"} {
		st := toolSnapshot(t, s, tool)
		if w := toolRequest(t, s, tool, "execute", map[string]string{"PlanID": st.PlanID}); w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	for range 2 {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("second tool was blocked by first")
		}
	}
	for _, tool := range []string{"format", "rename"} {
		st := toolSnapshot(t, s, tool)
		if st.Status != "running" || !strings.Contains(strings.Join(st.Logs, "\n"), "isolated "+tool) || strings.Contains(strings.Join(st.Logs, "\n"), "isolated "+map[string]string{"format": "rename", "rename": "format"}[tool]) {
			t.Fatal(st)
		}
		if w := toolRequest(t, s, tool, "plan", configs[tool]); w.Code == 200 {
			t.Fatal("new scan replaced a running task")
		}
		if toolSnapshot(t, s, tool).PlanID != st.PlanID {
			t.Fatal("rejected request changed running plan")
		}
	}
	if s.desktopCanExit() || s.beginDesktopExit() {
		t.Fatal("tray exit ignored active child tasks")
	}
	records, err := s.listTaskHistory()
	if err != nil || len(records) != 2 {
		t.Fatal("parallel tasks did not create separate history records", records, err)
	}
	for _, record := range records {
		if record.Status != "running" || record.Config.Root != configs[record.Tool].Root {
			t.Fatal("parallel history lease or parameters were mixed", record)
		}
	}
	if err := s.pruneTaskHistory(true); err != nil {
		t.Fatal(err)
	}
	if records, err := s.listTaskHistory(); err != nil || len(records) != 2 {
		t.Fatal("clearing history removed an active task", records, err)
	}
	if w := toolRequest(t, s, "rename", "cancel", map[string]string{"TaskID": fmtState.TaskID}); w.Code == 200 {
		t.Fatal("stale/cross-tool cancel accepted")
	}
	if w := toolRequest(t, s, "rename", "cancel", map[string]string{"TaskID": renameState.TaskID}); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	waitTool(t, s, "rename", "cancelled")
	if toolSnapshot(t, s, "format").Status != "running" {
		t.Fatal("cancel leaked into another tool")
	}
	close(release)
	done := waitTool(t, s, "format", "finished")
	s.taskWait.Wait()
	if done.Result.Success != 1 {
		t.Fatal(done)
	}
	records, err = s.listTaskHistory()
	if err != nil || len(records) != 2 {
		t.Fatal(records, err)
	}
	for _, record := range records {
		if record.Status != map[string]string{"format": "finished", "rename": "cancelled"}[record.Tool] {
			t.Fatal("history saved another task's final status", record)
		}
	}
	if _, err := os.Stat(filepath.Join(configs["rename"].Root, "old.txt")); err != nil {
		t.Fatal("cancelled rename modified source", err)
	}
	snapshot := toolSnapshot(t, s, "video")
	if len(snapshot.Completed) != 2 {
		t.Fatal("completion events lost", snapshot.Completed)
	}
	if len(snapshot.Tasks) < 3 {
		t.Fatal("missing navigation summaries")
	}
	for _, event := range snapshot.Completed {
		if event.PlanID == "" {
			t.Fatal(event)
		}
	}
	if !s.desktopCanExit() {
		t.Fatal("idle tasks blocked exit")
	}
}

func TestToolTaskPathConflictsAndIsolation(t *testing.T) {
	s := &server{}
	root := t.TempDir()
	image := s.toolTask("image")
	c := toolbox.DefaultConfig()
	c.Tool, c.Root = "image", root
	image.state.Status = "running"
	image.reserved = &c
	check := func(config toolbox.Config) error {
		slot := s.toolTask(config.Tool)
		s.taskDispatchMu.Lock()
		defer s.taskDispatchMu.Unlock()
		slot.mu.Lock()
		defer slot.mu.Unlock()
		return slot.taskConflict(config)
	}
	video := toolbox.DefaultConfig()
	video.Tool, video.Root = "video", root
	if err := check(video); err != nil {
		t.Fatal("shared source readers with isolated output should run together", err)
	}
	thumb := video
	thumb.Tool = "thumbnail"
	if err := check(thumb); err == nil {
		t.Fatal("thumbnail output could be read by an active image task")
	}
	image.state.Status = "idle"
	vslot := s.toolTask("video")
	vslot.state.Status = "running"
	vslot.reserved = &video
	if err := check(thumb); err != nil {
		t.Fatal("video readers should coexist with image-only thumbnail output", err)
	}
	destructive := video
	destructive.Tool = "format"
	if check(destructive) == nil {
		t.Fatal("destructive input overlap accepted")
	}
	other := c
	other.Root = t.TempDir()
	other.Output = filepath.Join(root, "toolbox-output", "video")
	if check(other) == nil {
		t.Fatal("overlapping outputs accepted")
	}
	other.Output = ""
	if err := check(other); err != nil {
		t.Fatal("independent roots blocked", err)
	}
	secret := toolbox.Config{Tool: "archive", ArchivePassword: "private", ExtractPassword: "other"}
	visible := publicTaskConfig(secret)
	if visible.ArchivePassword != "" || visible.ExtractPassword != "" {
		t.Fatal("state exposed passwords")
	}
}

func TestToolTaskAliasesConflict(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Skip("symlink unavailable")
	}
	if !taskOverlap(taskPath(root), taskPath(filepath.Join(alias, "child"))) {
		t.Fatal("path alias escaped reservation")
	}
}

func TestToolTaskLateHistoryLogIsolation(t *testing.T) {
	s := &server{state: state{TaskID: "next-task", Logs: []string{"next task log"}}}
	s.lineForTask("previous-task", "previous history failed")
	if len(s.state.Logs) != 1 {
		t.Fatal("late history warning leaked into a newer task")
	}
	s.lineForTask("next-task", "current history warning")
	if len(s.state.Logs) != 2 || !strings.Contains(s.state.Logs[1], "current history warning") {
		t.Fatal("current task lost its history warning")
	}
}

func TestToolTaskRechecksReadyPlanConflict(t *testing.T) {
	s := &server{}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "old.txt"), []byte("sample"), 0600); err != nil {
		t.Fatal(err)
	}
	c := toolbox.DefaultConfig()
	c.Tool, c.Root, c.Find, c.Replace = "rename", root, "old", "new"
	if w := toolRequest(t, s, "rename", "plan", c); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	ready := waitTool(t, s, "rename", "ready")
	writer := s.toolTask("format")
	writer.state.Status = "running"
	other := c
	other.Tool = "format"
	writer.reserved = &other
	if w := toolRequest(t, s, "rename", "execute", map[string]string{"PlanID": ready.PlanID}); w.Code == 200 {
		t.Fatal("ready plan ignored a writer that started after its scan")
	}
	if toolSnapshot(t, s, "rename").Status != "ready" {
		t.Fatal("conflict rejection lost the ready plan")
	}
	if _, err := os.Stat(filepath.Join(root, "old.txt")); err != nil {
		t.Fatal("rejected execution modified source", err)
	}
}

func TestToolTaskPreviewCountsInBackgroundSummary(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		files                      []string
		total, actionable, skipped int
	}{
		{"empty", []string{"unmatched.txt"}, 0, 0, 0},
		{"all-skipped", []string{"old.txt", "new.txt"}, 1, 0, 1},
		{"mixed", []string{"old.txt", "new.txt", "old2.txt"}, 2, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &server{}
			root := t.TempDir()
			for _, name := range tc.files {
				if err := os.WriteFile(filepath.Join(root, name), []byte("sample"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			c := toolbox.DefaultConfig()
			c.Tool, c.Root, c.Find, c.Replace, c.Dirs = "rename", root, "old", "new", false
			if w := toolRequest(t, s, "rename", "plan", c); w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			waitTool(t, s, "rename", "ready")
			st := toolSnapshot(t, s, "video")
			for _, summary := range st.Tasks {
				if summary.Tool == "rename" {
					if summary.Status != "ready" || summary.Total != tc.total || summary.Actionable != tc.actionable || summary.Conflicts != tc.skipped {
						t.Fatalf("incorrect background preview counts: %+v", summary)
					}
					return
				}
			}
			t.Fatal("missing background rename preview")
		})
	}
}
