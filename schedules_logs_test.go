package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScheduleLogsWindowAndCombined(t *testing.T) {
	zone := time.FixedZone("TEST", 8*3600)
	now := time.Date(2026, 11, 27, 15, 0, 0, 0, zone)
	start, end, e := scheduleLogWindow("7", "", "", now)
	if e != nil || !end.IsZero() || start.Format("2006-01-02 15:04") != "2026-11-21 00:00" {
		t.Fatal(start, end, e)
	}
	start, end, e = scheduleLogWindow("custom", "2026-11-01", "2026-11-02", now)
	if e != nil || end.Sub(start) != 48*time.Hour {
		t.Fatal(start, end, e)
	}
	for _, v := range [][3]string{{"unknown", "", ""}, {"custom", "", "2026-11-02"}, {"custom", "2026-11-03", "2026-11-02"}, {"custom", "invalid", "2026-11-02"}} {
		if _, _, e = scheduleLogWindow(v[0], v[1], v[2], now); e == nil {
			t.Fatal("accepted invalid range", v)
		}
	}
	m := newScheduleManager(t.TempDir())
	defer m.stop()
	m.store.Jobs = []scheduledJob{runtimeTestJob("A")}
	m.store.Runs = []scheduleRun{{ID: "new", JobID: "A", Status: "success", Started: now, Output: "latest <script>safe</script>"}, {ID: "other", JobID: "B", Status: "success", Started: now, Output: "other private"}, {ID: "old", JobID: "A", Status: "failed", Started: now.AddDate(0, 0, -10), Output: "older error"}}
	r, e := m.scheduleLogs("A", map[string]string{"range": "all"}, now)
	if e != nil {
		t.Fatal(e)
	}
	output := r["Output"].(string)
	if r["Total"] != 2 || r["Completed"] != 2 || strings.Index(output, "older error") > strings.Index(output, "latest") || strings.Contains(output, "other private") {
		t.Fatal(r)
	}
	r, e = m.scheduleLogs("A", map[string]string{"range": "7"}, now)
	if e != nil || r["Matched"] != 1 || strings.Contains(r["Output"].(string), "older error") {
		t.Fatal(r, e)
	}
	r, e = m.scheduleLogs("A", map[string]string{"range": "custom", "from": "2026-11-17", "to": "2026-11-17"}, now)
	if e != nil || r["Matched"] != 1 || !strings.Contains(r["Output"].(string), "older error") {
		t.Fatal(r, e)
	}
	if _, e = m.scheduleLogs("unknown", nil, now); e == nil {
		t.Fatal("unknown task")
	}
	m.store.Runs = nil
	for i := 0; i < 30; i++ {
		m.store.Runs = append(m.store.Runs, scheduleRun{ID: fmt.Sprint("run-", i), JobID: "A", Status: "success", Started: now.Add(-time.Duration(i) * time.Minute), Output: fmt.Sprint("entry-", i) + strings.Repeat("x", scheduleLogLimit-30)})
	}
	r, e = m.scheduleLogs("A", nil, now)
	if e != nil || r["Truncated"] != true || len(r["Output"].(string)) > scheduleCombinedLogLimit+256 || !strings.Contains(r["Output"].(string), "entry-0") {
		t.Fatal("unbounded or lost newest log", e)
	}
}
func TestScheduleLogsClearIsolationAndDurability(t *testing.T) {
	m := newScheduleManager(t.TempDir())
	defer m.stop()
	m.store.Jobs = []scheduledJob{runtimeTestJob("A"), runtimeTestJob("B")}
	m.store.Runs = []scheduleRun{{ID: "live", JobID: "A", Status: "running"}, {ID: "done-a", JobID: "A", Status: "failed", Output: "done"}, {ID: "done-b", JobID: "B", Status: "success", Output: "other"}}
	buf := &scheduleOutput{}
	buf.Write([]byte("live output"))
	m.live["live"] = buf
	m.active["A"] = func() {}
	if e := m.save(); e != nil {
		t.Fatal(e)
	}
	r, e := m.clearScheduleLogs("A")
	if e != nil || r["Count"] != 1 || r["PendingFiles"] != 0 {
		t.Fatal(r, e)
	}
	if len(m.store.Runs) != 2 || m.store.Runs[0].ID != "live" || m.readRunLog(m.store.Runs[0]) != "live output" || len(m.store.Jobs) != 2 || m.active["A"] == nil {
		t.Fatal("changed active/config/other state")
	}
	if _, e = os.Stat(m.logPath("done-a")); !os.IsNotExist(e) {
		t.Fatal("log not deleted", e)
	}
	if _, e = os.Stat(m.logPath("done-b")); e != nil {
		t.Fatal("other log removed", e)
	}
	b, _ := os.ReadFile(filepath.Join(m.dir, "schedules-history.json"))
	if strings.Contains(string(b), "done-a") || !strings.Contains(string(b), "done-b") {
		t.Fatal("clear not durable")
	}
	if r, e = m.clearScheduleLogs("A"); e != nil || r["Count"] != 0 {
		t.Fatal("idempotent", r, e)
	}
	s := &server{scheduler: m}
	if w := scheduleRequest(t, s, map[string]string{"Action": "logs", "ID": "B"}); w.Code != 200 || !strings.Contains(w.Body.String(), "other") {
		t.Fatal(w.Body.String())
	}
	if w := scheduleRequest(t, s, map[string]string{"Action": "clear-logs", "ID": "B"}); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
}
func TestScheduleLogsClearSaveFailure(t *testing.T) {
	m := newScheduleManager(t.TempDir())
	defer m.stop()
	m.store.Jobs = []scheduledJob{runtimeTestJob("A")}
	m.store.Runs = []scheduleRun{{ID: "done", JobID: "A", Status: "success", Output: "keep"}}
	if e := m.save(); e != nil {
		t.Fatal(e)
	}
	history := filepath.Join(m.dir, "schedules-history.json")
	os.Remove(history)
	os.Mkdir(history, 0700)
	if _, e := m.clearScheduleLogs("A"); e == nil {
		t.Fatal("expected durable save failure")
	}
	if len(m.store.Runs) != 1 {
		t.Fatal("memory not restored")
	}
	if _, e := os.Stat(m.logPath("done")); e != nil {
		t.Fatal("log removed before durable history", e)
	}

}
