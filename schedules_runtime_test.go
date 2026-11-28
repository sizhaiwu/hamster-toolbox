package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func runtimeTestJob(id string) scheduledJob {
	return scheduledJob{ID: id, Name: id, Kind: "http", Method: "GET", URL: "https://example.invalid", Interval: 3600, Timeout: 5}
}
func saveScheduleFixture(t *testing.T, dir string, s scheduleStore) {
	t.Helper()
	b, e := json.Marshal(s)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dir, "schedules.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
}
func waitScheduleIdle(t *testing.T, m *scheduleManager) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		m.mu.Lock()
		idle := len(m.active) == 0
		m.mu.Unlock()
		if idle {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("still running")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func TestScheduleRuntimeIsolationRecoveryAndReload(t *testing.T) {
	dir := t.TempDir()
	bad := runtimeTestJob("missing-dir")
	bad.Kind = "script"
	bad.Directory = filepath.Join(dir, "missing")
	bad.Shell = "sh"
	bad.Script = "echo example"
	if runtime.GOOS == "windows" {
		bad.Shell = "powershell"
	}
	bad.Enabled = true
	good := runtimeTestJob("healthy")
	saveScheduleFixture(t, dir, scheduleStore{Version: 1, ExamplesInitialized: true, Jobs: []scheduledJob{bad, good}, Runs: []scheduleRun{{ID: "interrupted-run", JobID: good.ID, Status: "running", Output: "before restart"}}})
	owner := newScheduleManager(dir)
	owner.start()
	defer owner.stop()
	if owner.err != "" || owner.store.Jobs[0].Enabled || owner.store.Jobs[0].Error == "" {
		t.Fatalf("isolation: %s %+v", owner.err, owner.store.Jobs)
	}
	s := &server{scheduler: owner}
	if r := scheduleRequest(t, s, map[string]string{"Action": "delete", "ID": bad.ID}); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	b, _ := os.ReadFile(filepath.Join(dir, "schedules-history.json"))
	if !strings.Contains(string(b), "interrupted") {
		t.Fatal("recovery not durable")
	}
	if out := owner.readRunLog(owner.store.Runs[0]); !strings.Contains(out, "before restart") {
		t.Fatal("lost recovery output", out)
	}
	follower := newScheduleManager(dir)
	defer follower.stop()
	owner.mu.Lock()
	owner.store.Jobs[0].Name = "latest-owner-edit"
	if e := owner.save(); e != nil {
		t.Fatal(e)
	}
	owner.mu.Unlock()
	owner.stop()
	follower.start()
	if follower.err != "" || follower.store.Jobs[0].Name != "latest-owner-edit" {
		t.Fatal("stale takeover", follower.err, follower.store.Jobs)
	}
}
func TestScheduleRuntimeQueueAndSettings(t *testing.T) {
	requests := make(chan string, 10)
	allow := make(chan struct{}, 10)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.URL.Path
		select {
		case <-allow:
			fmt.Fprint(w, "done")
		case <-r.Context().Done():
		}
	}))
	defer remote.Close()
	m := newScheduleManager(t.TempDir())
	defer m.stop()
	m.store.Concurrency = 1
	for i := 0; i < 3; i++ {
		j := runtimeTestJob(fmt.Sprint(i))
		j.URL = remote.URL + "/" + j.ID
		m.store.Jobs = append(m.store.Jobs, j)
	}
	s := &server{scheduler: m}
	for i := 0; i < 3; i++ {
		if r := scheduleRequest(t, s, map[string]string{"Action": "run", "ID": fmt.Sprint(i)}); r.Code != 200 {
			t.Fatal(r.Body.String())
		}
	}
	if n := <-requests; n != "/0" {
		t.Fatal(n)
	}
	m.mu.Lock()
	if len(m.active) != 1 || len(m.queue) != 2 {
		t.Fatal("not queued")
	}
	if e := m.submit("1", time.Now()); e == nil {
		t.Fatal("duplicate queued")
	}
	m.store.Jobs[1].Enabled = true
	m.store.Jobs[1].Next = time.Now().Add(-time.Minute)
	m.mu.Unlock()
	if r := scheduleRequest(t, s, map[string]string{"Action": "cancel", "ID": "1"}); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	m.tick(time.Now())
	m.mu.Lock()
	if m.isQueued("1") || !m.store.Jobs[1].Next.After(time.Now()) {
		t.Fatal("cancel immediately requeued")
	}
	m.mu.Unlock()
	allow <- struct{}{}
	waitScheduleIdle(t, m)
	m.tick(time.Now())
	if n := <-requests; n != "/2" {
		t.Fatal("FIFO or cancelled executed", n)
	}
	allow <- struct{}{}
	waitScheduleIdle(t, m)
	r := scheduleRequest(t, s, map[string]any{"Action": "options", "Options": map[string]int{"Concurrency": 2, "Retention": 5}})
	if r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	n := newScheduleManager(m.dir)
	defer n.stop()
	if n.parallelism() != 2 || n.retention() != 5 {
		t.Fatal("options not persisted")
	}
	if r := scheduleRequest(t, s, map[string]any{"Action": "options", "Options": map[string]int{"Concurrency": 9, "Retention": 5}}); r.Code == 200 {
		t.Fatal("invalid concurrency")
	}
}
func TestScheduleRuntimeLogsTailRetentionAndMigration(t *testing.T) {
	dir := t.TempDir()
	m := newScheduleManager(dir)
	defer m.stop()
	m.store.Retention = 5
	m.store.Jobs = []scheduledJob{runtimeTestJob("chatty"), runtimeTestJob("quiet")}
	m.store.Runs = []scheduleRun{{ID: "quiet-old", JobID: "quiet", Status: "success", Output: "retained"}}
	for i := 0; i < 12; i++ {
		m.store.Runs = append([]scheduleRun{{ID: fmt.Sprint("chatty-", i), JobID: "chatty", Status: "success", Output: "log"}}, m.store.Runs...)
	}
	m.trimRuns()
	if len(m.store.Runs) != 6 {
		t.Fatal("not per-job retention", len(m.store.Runs))
	}
	if e := m.save(); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "schedules.json"))
	if strings.Contains(string(b), "quiet-old") || strings.Contains(string(b), "retained") {
		t.Fatal("logs inside config")
	}
	n := newScheduleManager(dir)
	defer n.stop()
	if n.readRunLog(n.store.Runs[5]) != "retained" {
		t.Fatal("log migration failed")
	}
	output := &scheduleOutput{}
	output.Write([]byte(strings.Repeat("a", scheduleLogLimit+10)))
	output.Write([]byte("FINAL ERROR"))
	if out := output.snapshot(); !strings.HasSuffix(out, "FINAL ERROR") || !strings.Contains(out, "64 KB") || len(out) > scheduleLogLimit+128 {
		t.Fatal("tail not preserved")
	}
	m.live["live-run"] = output
	m.store.Runs = append([]scheduleRun{{ID: "live-run", JobID: "quiet", Status: "running"}}, m.store.Runs...)
	m.flushLiveLogs()
	saved, _ := os.ReadFile(m.logPath("live-run"))
	if !strings.Contains(string(saved), "FINAL ERROR") {
		t.Fatal("live output not flushed")
	}
}
func TestScheduleRuntimeLiveScriptOutput(t *testing.T) {
	b := &scheduleOutput{}
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), scheduleLogKey{}, b), 5*time.Second)
	defer cancel()
	j := scheduledJob{Kind: "script", Shell: "sh", Script: "printf 'VISIBLE\\n'; sleep 1; printf 'FINISHED\\n'"}
	if runtime.GOOS == "windows" {
		j.Shell = "powershell"
		j.Script = "Write-Output 'VISIBLE'; Start-Sleep -Seconds 1; Write-Output 'FINISHED'"
	}
	done := make(chan error, 1)
	dir := t.TempDir()
	go func() { _, e := executeScheduledJob(ctx, j, dir); done <- e }()
	deadline := time.Now().Add(3 * time.Second)
	for !strings.Contains(b.snapshot(), "VISIBLE") {
		if time.Now().After(deadline) {
			t.Fatal("no live output")
		}
		select {
		case e := <-done:
			t.Fatal("finished before observable output", e)
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	if strings.Contains(b.snapshot(), "FINISHED") {
		t.Fatal("output only returned on completion")
	}
	if e := <-done; e != nil {
		t.Fatal(e)
	}
}
func TestScheduleRuntimeHTTPChecksRetryAndCancel(t *testing.T) {
	var count atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if count.Add(1) == 1 {
			w.WriteHeader(503)
		} else {
			w.WriteHeader(201)
		}
		fmt.Fprint(w, "expected payload")
	}))
	defer remote.Close()
	j := runtimeTestJob("http")
	j.URL = remote.URL
	j.ExpectedStatus = 201
	j.ResponseContains = "expected"
	j.RetryCount = 1
	j.RetryDelay = 1
	if e := validateScheduledJob(&j); e != nil {
		t.Fatal(e)
	}
	out, e := executeScheduledJob(context.Background(), j, t.TempDir())
	if e != nil || count.Load() != 2 || !strings.Contains(out, "503") {
		t.Fatal(out, e, count.Load())
	}
	j.RetryCount = 0
	j.ResponseContains = "absent"
	if _, e = executeScheduledJob(context.Background(), j, t.TempDir()); e == nil {
		t.Fatal("missing keyword accepted")
	}
	j.Method = "POST"
	j.RetryCount = 1
	if e = validateScheduledJob(&j); e == nil {
		t.Fatal("POST retry without opt in")
	}
	j.RetryPOST = true
	if e = validateScheduledJob(&j); e != nil {
		t.Fatal(e)
	}
	j.Method = "GET"
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	before := count.Load()
	_, e = executeScheduledJob(ctx, j, t.TempDir())
	if e == nil || count.Load() != before+1 {
		t.Fatal("retry cancelled incorrectly", e, count.Load())
	}
}
func TestScheduleRuntimePicker(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "子目录"), 0700)
	os.WriteFile(filepath.Join(dir, "sample.txt"), []byte("sample"), 0600)
	d, e := schedulePathList(dir, "dir", "")
	if e != nil || len(d["Entries"].([]schedulePathEntry)) != 1 {
		t.Fatal(d, e)
	}
	d, e = schedulePathList(dir, "file", "sample")
	if e != nil || len(d["Entries"].([]schedulePathEntry)) != 1 || d["Entries"].([]schedulePathEntry)[0].Directory {
		t.Fatal(d, e)
	}
	if _, e = schedulePathList(filepath.Join(dir, "missing"), "dir", ""); e == nil {
		t.Fatal("missing directory accepted")
	}
}
