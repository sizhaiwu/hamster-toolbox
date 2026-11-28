package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestScheduleCron(t *testing.T) {
	base := time.Date(2026, 11, 27, 9, 0, 0, 0, time.Local)
	cases := []struct{ cron, want string }{{"*/15 * * * *", "2026-11-27 09:15"}, {"0 9 * * 1-5", "2026-11-30 09:00"}, {"0 9 1 * *", "2026-12-01 09:00"}, {"0 9 * * 7", "2026-11-29 09:00"}, {"0 9 1 * 1", "2026-11-30 09:00"}, {"0 0 29 2 *", "2028-02-29 00:00"}}
	for _, c := range cases {
		n, e := nextScheduledTime(scheduledJob{Cron: c.cron}, base)
		if e != nil || n.Format("2006-01-02 15:04") != c.want {
			t.Fatalf("%s: %v %v", c.cron, n, e)
		}
	}
	for _, c := range []string{"* * *", "60 * * * *", "*/0 * * * *", "0 9 * 13 *", "0 9 * * 9", "0 9 * * 5-1"} {
		if _, e := parseScheduleCron(c); e == nil {
			t.Fatalf("accepted %s", c)
		}
	}
}
func scheduleRequest(t *testing.T, s *server, q any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(q)
	r := httptest.NewRequest("POST", "/api/schedules", strings.NewReader(string(b)))
	w := httptest.NewRecorder()
	s.scheduleAPI(w, r)
	return w
}
func TestSchedulePersistenceAndExecution(t *testing.T) {
	requests := make(chan string, 8)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Method + ":" + r.Header.Get("X-Test")
		w.Write([]byte("okay"))
	}))
	defer remote.Close()
	m := newScheduleManager(t.TempDir())
	defer m.stop()
	s := &server{scheduler: m}
	j := scheduledJob{Name: "请求测试", Kind: "http", Method: "POST", URL: remote.URL, Headers: map[string]string{"X-Test": "yes"}, Body: "{}", Interval: 60, Timeout: 3, Enabled: true}
	w := scheduleRequest(t, s, map[string]any{"Action": "save", "Job": j})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	id := m.store.Jobs[0].ID
	m2 := newScheduleManager(m.dir)
	defer m2.stop()
	if len(m2.store.Jobs) != 1 || m2.store.Jobs[0].URL != remote.URL {
		t.Fatal("not persisted")
	}
	if st, e := os.Stat(filepath.Join(m.dir, "schedules.json")); e != nil || st.Size() == 0 {
		t.Fatal("missing store")
	}
	w = scheduleRequest(t, s, map[string]string{"Action": "run", "ID": id})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	select {
	case v := <-requests:
		if v != "POST:yes" {
			t.Fatal(v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no execution")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		m.mu.Lock()
		done := len(m.active) == 0
		m.mu.Unlock()
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("still active")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if m.store.Runs[0].Status != "success" || !strings.Contains(m.readRunLog(m.store.Runs[0]), "okay") {
		t.Fatalf("%+v", m.store.Runs[0])
	}
	w = scheduleRequest(t, s, map[string]string{"Action": "list"})
	if strings.Contains(w.Body.String(), remote.URL) || strings.Contains(w.Body.String(), "okay") {
		t.Fatal("list leaked details")
	}
	scheduleRequest(t, s, map[string]string{"Action": "toggle", "ID": id})
	if m.store.Jobs[0].Enabled {
		t.Fatal("toggle failed")
	}
	scheduleRequest(t, s, map[string]string{"Action": "delete", "ID": id})
	if len(m.store.Jobs) != 0 || len(m.store.Runs) != 1 {
		t.Fatal("delete failed")
	}
}
func TestScheduleNoOverlapTimeoutAndRecovery(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer remote.Close()
	m := newScheduleManager(t.TempDir())
	defer m.stop()
	j := scheduledJob{ID: "job", Name: "timeout", Kind: "http", URL: remote.URL, Method: "GET", Interval: 60, Timeout: 1, Enabled: true, Next: time.Now().Add(-time.Minute)}
	m.store.Jobs = []scheduledJob{j}
	m.tick(time.Now())
	m.tick(time.Now())
	if len(m.store.Runs) != 1 {
		t.Fatal("overlap")
	}
	time.Sleep(1300 * time.Millisecond)
	m.mu.Lock()
	if len(m.active) != 0 || m.store.Runs[0].Status != "timeout" {
		t.Fatalf("%+v", m.store.Runs)
	}
	m.store.Runs[0].Status = "running"
	if e := m.save(); e != nil {
		t.Fatal(e)
	}
	m.mu.Unlock()
	recovered := newScheduleManager(m.dir)
	defer recovered.stop()
	if recovered.store.Runs[0].Status != "interrupted" {
		t.Fatal("recovery")
	}
}
func TestScheduleScriptsAndHTTPFailure(t *testing.T) {
	shell, script := "sh", "printf hello"
	if runtime.GOOS == "windows" {
		shell = "powershell"
		script = "Write-Output 'hello'"
	}
	j := scheduledJob{Kind: "script", Shell: shell, Script: script, Timeout: 5}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, e := executeScheduledJob(ctx, j, t.TempDir())
	if e != nil || !strings.Contains(out, "hello") {
		t.Fatalf("%s %v", out, e)
	}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "bad", 500) }))
	defer remote.Close()
	out, e = executeScheduledJob(ctx, scheduledJob{Kind: "http", URL: remote.URL, Method: "GET", Timeout: 3}, t.TempDir())
	if e == nil || !strings.Contains(out, "500") {
		t.Fatal("HTTP error not detected")
	}
}
func TestScheduleCorruptionAndWriteFailure(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "schedules.json")
	os.WriteFile(file, []byte("broken"), 0600)
	m := newScheduleManager(dir)
	defer m.stop()
	s := &server{scheduler: m}
	w := scheduleRequest(t, s, map[string]string{"Action": "delete", "ID": "x"})
	b, _ := os.ReadFile(file)
	if w.Code == 200 || string(b) != "broken" {
		t.Fatal("corrupt store overwritten")
	}
	root := t.TempDir()
	blocked := filepath.Join(root, "file")
	os.WriteFile(blocked, []byte("x"), 0600)
	n := newScheduleManager(filepath.Join(blocked, "child"))
	defer n.stop()
	n.err = ""
	n.store.Jobs = []scheduledJob{{ID: "x", Name: "x", Kind: "http", Method: "GET", URL: "http://127.0.0.1", Interval: 60, Timeout: 1}}
	if e := n.launch("x", time.Now()); e == nil || len(n.active) != 0 || len(n.store.Runs) != 0 {
		t.Fatal("executed without durable claim")
	}
}
func TestScheduleRemoteDenied(t *testing.T) {
	b := &fileBrowser{}
	for _, path := range []string{"/schedules.html", "/api/schedules", "/schedules.js"} {
		w := httptest.NewRecorder()
		if !b.remoteWorkspace(w, httptest.NewRequest("GET", path, nil), sharingConfig{}) || w.Code != 403 {
			t.Fatal(path)
		}
	}
}
func TestScheduleSingleOwner(t *testing.T) {
	dir := t.TempDir()
	a := newScheduleManager(dir)
	a.start()
	defer a.stop()
	b := newScheduleManager(dir)
	b.start()
	defer b.stop()
	if a.err != "" || b.err == "" {
		t.Fatalf("owner lock: %s / %s", a.err, b.err)
	}
	a.stop()
	c := newScheduleManager(dir)
	c.start()
	defer c.stop()
	if c.err != "" {
		t.Fatal(c.err)
	}
}
func TestScheduleScriptCancellation(t *testing.T) {
	shell, script := "sh", "sleep 30"
	if runtime.GOOS == "windows" {
		shell = "powershell"
		script = "Start-Sleep -Seconds 30"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, e := executeScheduledJob(ctx, scheduledJob{Kind: "script", Shell: shell, Script: script, Timeout: 1}, t.TempDir())
	if e == nil || time.Since(start) > 5*time.Second {
		t.Fatal("script was not cancelled promptly", e)
	}
}
func TestScheduleRetentionAndFairness(t *testing.T) {
	m := newScheduleManager(t.TempDir())
	defer m.stop()
	m.cancel()
	old := time.Now().Add(-time.Hour)
	newer := time.Now().Add(-time.Minute)
	job := scheduledJob{ID: "new", Name: "new", Kind: "http", URL: "http://127.0.0.1", Method: "GET", Interval: 60, Timeout: 1, Enabled: true, Next: newer}
	older := job
	older.ID = "old"
	older.Next = old
	m.store.Jobs = []scheduledJob{job, older}
	for i := 0; i < 3; i++ {
		m.active[string(rune('a'+i))] = func() {}
	}
	m.store.Runs = []scheduleRun{{ID: "keep-running", Status: "running"}}
	for i := 0; i < 99; i++ {
		m.store.Runs = append(m.store.Runs, scheduleRun{ID: "finished", Status: "success"})
	}
	m.tick(time.Now())
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.store.Runs) > 10200 || m.store.Runs[0].JobID != "old" {
		t.Fatal("did not prioritize older pending job")
	}
	found := false
	for _, r := range m.store.Runs {
		if r.ID == "keep-running" {
			found = true
		}
	}
	if !found {
		t.Fatal("evicted active run")
	}
}
func TestSchedulePanelPeriods(t *testing.T) {
	base := time.Date(2026, 11, 27, 9, 17, 0, 0, time.Local)
	cases := []struct {
		period                            string
		every, hour, minute, day, weekday int
		cron, next                        string
	}{
		{"day", 0, 10, 30, 0, 0, "30 10 * * *", "2026-11-27 10:30:00"},
		{"day-n", 2, 9, 0, 0, 0, "0 9 */2 * *", "2026-11-29 09:00:00"},
		{"hour", 0, 0, 20, 0, 0, "20 * * * *", "2026-11-27 09:20:00"},
		{"hour-n", 2, 0, 5, 0, 0, "5 */2 * * *", "2026-11-27 10:05:00"},
		{"minute-n", 30, 0, 0, 0, 0, "*/30 * * * *", "2026-11-27 09:30:00"},
		{"week", 0, 9, 0, 0, 1, "0 9 * * 1", "2026-11-30 09:00:00"},
		{"month", 0, 9, 0, 1, 0, "0 9 1 * *", "2026-12-01 09:00:00"},
		{"second-n", 5, 0, 0, 0, 0, "", "2026-11-27 09:17:05"},
	}
	for _, c := range cases {
		j := scheduledJob{Name: "test", Kind: "http", URL: "https://example.com", Method: "GET", Period: c.period, Every: c.every, AtHour: c.hour, AtMinute: c.minute, Day: c.day, Weekday: c.weekday}
		if e := validateScheduledJob(&j); e != nil {
			t.Fatal(c.period, e)
		}
		next, e := nextScheduledTime(j, base)
		if e != nil || j.Cron != c.cron || next.Format("2006-01-02 15:04:05") != c.next {
			t.Fatal(c.period, j.Cron, next, e)
		}
	}
	for _, j := range []scheduledJob{{Period: "second-n", Every: 0}, {Period: "minute-n", Every: 61}, {Period: "day-n", Every: 32}, {Period: "hour-n", Every: 25}, {Period: "month", Day: 0}, {Period: "day", AtHour: 24}, {Period: "week", Weekday: 7}} {
		if e := normalizeScheduledPeriod(&j); e == nil {
			t.Fatal("invalid period accepted", j)
		}
	}
}
func TestScheduleInitialExamplesOnlyOnce(t *testing.T) {
	dir := t.TempDir()
	m := newScheduleManager(dir)
	m.start()
	if m.err != "" || len(m.store.Jobs) != 4 || !m.store.ExamplesInitialized {
		t.Fatal("examples not initialized", m.err)
	}
	for _, j := range m.store.Jobs {
		if j.Enabled {
			t.Fatal("example enabled")
		}
		if e := validateScheduledJob(&j); e != nil {
			t.Fatal(e)
		}
	}
	m.mu.Lock()
	m.store.Jobs = nil
	if e := m.save(); e != nil {
		t.Fatal(e)
	}
	m.mu.Unlock()
	m.stop()
	next := newScheduleManager(dir)
	next.start()
	defer next.stop()
	if len(next.store.Jobs) != 0 {
		t.Fatal("deleted examples reappeared")
	}
}
func TestScheduleExamplesPreserveExisting(t *testing.T) {
	dir := t.TempDir()
	m := newScheduleManager(dir)
	j := initialScheduledJobs()[0]
	j.ID = "user-job"
	j.Name = "My existing job"
	m.store.Jobs = []scheduledJob{j}
	if e := m.save(); e != nil {
		t.Fatal(e)
	}
	m.stop()
	n := newScheduleManager(dir)
	n.start()
	defer n.stop()
	if len(n.store.Jobs) != 5 || n.store.Jobs[0].Name != j.Name {
		t.Fatal("existing job modified")
	}
}
func TestSchedulePreviewUsesRealTimingWithoutSaving(t *testing.T) {
	zone := time.FixedZone("TEST", 8*3600)
	base := time.Date(2026, 4, 30, 10, 0, 0, 0, zone)
	j := scheduledJob{Period: "month", Day: 31, AtHour: 9, AtMinute: 30}
	result, e := previewScheduleTimes(j, base)
	if e != nil {
		t.Fatal(e)
	}
	times := result["Times"].([]string)
	if len(times) != 5 || times[0] != "2026-05-31（周日）09:30:00" || times[1] != "2026-07-31（周五）09:30:00" {
		t.Fatal(times)
	}
	if !strings.Contains(result["Timezone"].(string), "+08:00") {
		t.Fatal(result)
	}
	j = scheduledJob{Period: "second-n", Every: 5}
	result, e = previewScheduleTimes(j, base)
	if e != nil || result["Times"].([]string)[4] != "2026-04-30（周四）10:00:25" {
		t.Fatal(result, e)
	}
	for _, job := range []scheduledJob{{Period: "day", AtHour: 24}, {Period: "cron", Cron: "invalid"}, {Period: "cron", Cron: "0 9 31 2 *"}} {
		if _, e := previewScheduleTimes(job, base); e == nil {
			t.Fatal("invalid preview accepted", job)
		}
	}
	m := newScheduleManager(t.TempDir())
	defer m.stop()
	s := &server{scheduler: m}
	w := scheduleRequest(t, s, map[string]any{"Action": "preview", "Job": scheduledJob{Period: "day", AtHour: 9}})
	if w.Code != 200 || len(m.store.Jobs) != 0 || len(m.store.Runs) != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, e := os.Stat(filepath.Join(m.dir, "schedules.json")); !os.IsNotExist(e) {
		t.Fatal("preview wrote data", e)
	}
}
