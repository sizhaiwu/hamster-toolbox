package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

func historyFixture(t *testing.T) (*server, *toolbox.Plan) {
	t.Helper()
	s := &server{token: "test-session", dataDir: t.TempDir(), state: state{PlanID: "test-plan", Status: "running"}}
	c := toolbox.DefaultConfig()
	c.Tool = "exif"
	c.Root = t.TempDir()
	c.Output = t.TempDir()
	c.MetadataTypes = "images"
	c.Exiftool, _ = os.Executable()
	p := &toolbox.Plan{Config: c}
	for _, name := range []string{"completed.jpg", "failed.jpg", "unfinished.jpg", "excluded.jpg"} {
		source := filepath.Join(c.Root, name)
		os.WriteFile(source, []byte("synthetic media"), 0600)
		p.Items = append(p.Items, toolbox.Item{Source: source, Target: filepath.Join(c.Output, name)})
	}
	p.Items[3].Conflict = "预览冲突"
	return s, p
}
func TestTaskHistoryRestartAndSafeRetry(t *testing.T) {
	s, p := historyFixture(t)
	j, err := s.newTaskJournal(p, "test-plan", "")
	if err != nil {
		t.Fatal(err)
	}
	j.observe(toolbox.FileOutcome{Source: p.Items[0].Source, Status: "success"})
	j.observe(toolbox.FileOutcome{Source: p.Items[1].Source, Status: "failed", Message: "test failure"})
	if !taskRecordActive(j.root, j.meta.ID) {
		t.Fatal("running task lease was not held")
	}
	w := requestAPI(t, s, "/api/task-history-detail", map[string]any{"ID": j.meta.ID})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var detail struct{ CanRetry bool }
	json.Unmarshal(w.Body.Bytes(), &detail)
	if detail.CanRetry {
		t.Fatal("active task can be retried")
	}
	if err = j.finish(state{Status: "cancelled", Done: 2, Result: toolbox.Result{Success: 1, Failed: 1}, Logs: []string{"cancelled"}}); err != nil {
		t.Fatal(err)
	}
	restarted := &server{token: s.token, dataDir: s.dataDir}
	sources, err := restarted.remainingHistorySources(context.Background(), j.meta.ID, p.Config)
	if err != nil || len(sources) != 2 || sources[0] != p.Items[1].Source || sources[1] != p.Items[2].Source {
		t.Fatal(sources, err)
	}
	// An already-published result must remain a conflict even when the old
	// record says unfinished (e.g. the process died just after publication).
	os.WriteFile(p.Items[1].Target, []byte("existing output"), 0600)
	retry, err := toolbox.BuildRemainingPlan(context.Background(), p.Config, sources, func(string) {})
	if err != nil || len(retry.Items) != 2 {
		t.Fatal(retry, err)
	}
	for _, it := range retry.Items {
		if it.Source == p.Items[0].Source || it.Source == p.Items[3].Source {
			t.Fatal("successful or excluded source included")
		}
		if it.Source == p.Items[1].Source && it.Conflict == "" {
			t.Fatal("existing output would be overwritten")
		}
	}
	wrong := p.Config
	wrong.Root = t.TempDir()
	if _, err = restarted.remainingHistorySources(context.Background(), j.meta.ID, wrong); err == nil {
		t.Fatal("changed root accepted")
	}
	wrong = p.Config
	wrong.Tool = "rename"
	if _, err = restarted.remainingHistorySources(context.Background(), j.meta.ID, wrong); err == nil {
		t.Fatal("changed tool accepted")
	}
}
func TestTaskHistoryInterruptedAndPrivacy(t *testing.T) {
	s, p := historyFixture(t)
	p.Config.ArchivePassword = "Private-Record-Secret!"
	j, err := s.newTaskJournal(p, "test-plan", "")
	if err != nil {
		t.Fatal(err)
	}
	j.observe(toolbox.FileOutcome{Source: p.Items[0].Source, Status: "success"})
	// Release the OS lease without writing a finish record, as process exit does.
	close(j.stop)
	<-j.stopped
	j.events.Close()
	unlockTaskRecord(j.lease)
	j.lease.Close()
	w := requestAPI(t, s, "/api/task-history-detail", map[string]any{"ID": j.meta.ID})
	var detail struct {
		Record    taskRecord
		Remaining int
		CanRetry  bool
	}
	if err = json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatal(err, w.Body.String())
	}
	if detail.Record.Status != "interrupted" || detail.Record.Result.Success != 1 || detail.Remaining != 2 || !detail.CanRetry {
		t.Fatal(detail)
	}
	for _, suffix := range []string{".json", ".events"} {
		b, _ := os.ReadFile(historyFile(j.root, j.meta.ID, suffix))
		if strings.Contains(string(b), p.Config.ArchivePassword) {
			t.Fatal("history contains a password")
		}
	}
	for _, id := range []string{"../settings", strings.Repeat("a", 47), strings.Repeat("a", 48) + "/x"} {
		if response := requestAPI(t, s, "/api/task-history-detail", map[string]any{"ID": id}); response.Code == 200 {
			t.Fatal("history path traversal accepted")
		}
	}
	active, err := s.newTaskJournal(p, "test-plan", "")
	if err != nil {
		t.Fatal(err)
	}
	defer active.finish(state{Status: "finished"})
	if response := requestAPI(t, s, "/api/task-history-clear", map[string]any{}); response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	if _, err = os.Stat(historyFile(j.root, j.meta.ID, ".json")); !os.IsNotExist(err) {
		t.Fatal("inactive record not cleared")
	}
	if _, err = os.Stat(historyFile(active.root, active.meta.ID, ".json")); err != nil {
		t.Fatal("active record deleted")
	}
}
func TestTaskHistoryLeaseAcrossProcesses(t *testing.T) {
	const key = "HAMSTER_TEST_HISTORY_LOCK_PATH"
	if path := os.Getenv(key); path != "" {
		f, err := os.OpenFile(path, os.O_RDWR, 0600)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if lockTaskRecord(f) == nil {
			unlockTaskRecord(f)
			t.Fatal("another process acquired an active task lease")
		}
		return
	}
	s, p := historyFixture(t)
	j, err := s.newTaskJournal(p, "test-plan", "")
	if err != nil {
		t.Fatal(err)
	}
	defer j.finish(state{Status: "finished"})
	exe, _ := os.Executable()
	cmd := exec.Command(exe, "-test.run=^TestTaskHistoryLeaseAcrossProcesses$", "-test.count=1")
	cmd.Env = append(os.Environ(), key+"="+historyFile(j.root, j.meta.ID, ".lock"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
}
func TestTaskHistoryRetention(t *testing.T) {
	s, _ := historyFixture(t)
	root, err := s.historyDirectory(true)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 55; i++ {
		r := taskRecord{Version: 1, ID: randomID(), Tool: "exif", Config: toolbox.Config{Tool: "exif", Root: s.dataDir}, Status: "finished", Started: time.Now().Add(-time.Duration(i) * time.Minute)}
		if err = writeTaskRecord(root, r); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.pruneTaskHistory(false); err != nil {
		t.Fatal(err)
	}
	records, err := s.listTaskHistory()
	if err != nil || len(records) != historyKeep {
		t.Fatal(len(records), err)
	}
}

func TestTaskHistoryUnreadableItemsAllowSettingsOnly(t *testing.T) {
	s, p := historyFixture(t)
	j, err := s.newTaskJournal(p, "test-plan", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = j.finish(state{Status: "cancelled", Done: 1, Result: toolbox.Result{Success: 1}}); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(historyFile(j.root, j.meta.ID, ".events"), []byte("broken record"), 0600); err != nil {
		t.Fatal(err)
	}
	w := requestAPI(t, s, "/api/task-history-detail", map[string]any{"ID": j.meta.ID})
	var detail struct {
		Record   taskRecord
		CanRetry bool
		Warning  string
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &detail) != nil || detail.CanRetry || !detail.Record.Limited || detail.Warning == "" || detail.Record.Config.Root != p.Config.Root {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, err = s.remainingHistorySources(context.Background(), j.meta.ID, p.Config); err == nil {
		t.Fatal("unreadable item log allowed targeted retry")
	}
	if _, err = readTaskRecord("", j.meta.ID); err == nil {
		t.Fatal("empty data directory accepted")
	}
}
