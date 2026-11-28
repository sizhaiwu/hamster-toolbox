package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRemoteWorkspaceSchedules(t *testing.T) {
	owner, b, _ := workspaceFixture(t)
	m := newScheduleManager(owner.dataDir)
	owner.scheduler = m
	m.start()
	t.Cleanup(m.stop)
	c := workspaceSignIn(t, b)
	request := func(body any) *httptest.ResponseRecorder {
		t.Helper()
		w := c.post("/api/schedules", body)
		if w.Code != http.StatusOK || strings.Contains(w.Body.String(), owner.token) {
			t.Fatalf("remote schedule API: %d %s", w.Code, w.Body.String())
		}
		return w
	}
	// The page uses the remote capability and gets the same session-expiry UI.
	w := workspaceCall(b, c, "GET", "/schedules.html", nil, nil)
	if w.Code != http.StatusSeeOther || !strings.Contains(w.Header().Get("Location"), c.csrf) || strings.Contains(w.Header().Get("Location"), owner.token) {
		t.Fatal("schedule navigation capability", w.Code, w.Header())
	}
	w = workspaceCall(b, c, "GET", "/schedules.html?session="+c.csrf, nil, nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "remote-workspace.js") || !strings.Contains(w.Body.String(), "head-actions") || strings.Contains(w.Body.String(), owner.token) {
		t.Fatal("schedule page missing administrator session UI", w.Code)
	}
	for _, path := range []string{"/schedules.js", "/schedules.css", "/schedule-controls.js", "/schedule-library.js", "/schedule-status.js", "/schedule-status.css"} {
		if w := workspaceCall(b, c, "GET", path, nil, nil); w.Code != http.StatusOK {
			t.Fatal("missing schedule asset", path, w.Code)
		}
	}
	guestID := "synthetic-schedule-guest"
	b.sessions[guestID] = guestSession{Expires: time.Now().Add(time.Hour)}
	guest := &workspaceClient{b: b, cookie: &http.Cookie{Name: "hamster_guest_" + b.cookieTag, Value: guestID}}
	for _, check := range []struct {
		name, method, path string
		client             *workspaceClient
		headers            map[string]string
		code               int
	}{
		{"login required", "GET", "/schedules.html", nil, nil, 303},
		{"guest denied", "POST", "/api/schedules", guest, map[string]string{"X-Toolbox-Token": c.csrf}, 401},
		{"cookie required", "POST", "/api/schedules", nil, map[string]string{"X-Toolbox-Token": c.csrf}, 401},
		{"CSRF required", "POST", "/api/schedules", c, nil, 403},
		{"local token denied", "POST", "/api/schedules", c, map[string]string{"X-Toolbox-Token": owner.token}, 403},
		{"cross origin denied", "POST", "/api/schedules", c, map[string]string{"X-Toolbox-Token": c.csrf, "Origin": "https://evil.test"}, 403},
		{"GET API denied", "GET", "/api/schedules", c, map[string]string{"X-Toolbox-Token": c.csrf}, 403},
	} {
		w := workspaceCall(b, check.client, check.method, check.path, map[string]string{"Action": "list"}, check.headers)
		if w.Code != check.code {
			t.Fatal(check.name, w.Code, w.Body.String())
		}
	}
	// Use a harmless local script; a phone and the local page share the owner's
	// scheduler instead of creating a new scheduler or running on the phone.
	job := scheduledJob{Name: "remote schedule fixture", Kind: "script", Shell: "sh", Script: "printf 'remote-schedule-executed\\n'", Interval: 60, Timeout: 5}
	if runtime.GOOS == "windows" {
		job.Shell, job.Script = "cmd", "echo remote-schedule-executed"
	}
	request(map[string]any{"Action": "save", "Job": job})
	var listing struct{ Jobs []scheduledJob }
	json.Unmarshal(request(map[string]string{"Action": "list"}).Body.Bytes(), &listing)
	for _, j := range listing.Jobs {
		if j.Name == job.Name {
			job.ID = j.ID
		}
	}
	if job.ID == "" {
		t.Fatal("remote-created job missing from owner's scheduler")
	}
	job.Name = "remote edited fixture"
	request(map[string]any{"Action": "save", "ID": job.ID, "Job": job})
	request(map[string]string{"Action": "run", "ID": job.ID})
	deadline := time.Now().Add(8 * time.Second)
	for {
		m.mu.Lock()
		done := len(m.active) == 0
		m.mu.Unlock()
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("remote script did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	w = request(map[string]any{"Action": "logs", "ID": job.ID})
	if !strings.Contains(w.Body.String(), "remote-schedule-executed") {
		t.Fatal("remote logs missing real script output", w.Body.String())
	}
	request(map[string]any{"Action": "export", "IDs": []string{job.ID}, "Format": "toolbox"})
	request(map[string]any{"Action": "library"})
	request(map[string]any{"Action": "toggle", "ID": job.ID})
	request(map[string]any{"Action": "toggle", "ID": job.ID})
	request(map[string]any{"Action": "delete", "ID": job.ID})

	// Background status/log polling must not renew an abandoned admin session.
	b.workspace.mu.Lock()
	session := b.workspace.sessions[c.cookie.Value]
	before := time.Now().Add(-time.Minute)
	session.last = before
	b.workspace.mu.Unlock()
	w = workspaceCall(b, c, "POST", "/api/schedules", map[string]string{"Action": "list"}, map[string]string{"X-Toolbox-Token": c.csrf, "X-Hamster-Poll": "1"})
	if w.Code != 200 {
		t.Fatal("schedule polling", w.Code)
	}
	b.workspace.mu.Lock()
	renewed := !session.last.Equal(before)
	b.workspace.mu.Unlock()
	if renewed {
		t.Fatal("schedule polling kept an idle session alive")
	}
	b.config.WorkspaceHTTP = false
	if w := c.post("/api/schedules", map[string]string{"Action": "list"}); w.Code != 403 {
		t.Fatal("HTTP administrator restriction bypassed", w.Code)
	}
	b.config.WorkspaceHTTP = true
	request(map[string]string{"Action": "list"})
	b.workspace.mu.Lock()
	if !session.last.After(before) {
		t.Error("interactive access did not renew administrator session")
	}
	session.last = time.Now().Add(-31 * time.Minute)
	b.workspace.mu.Unlock()
	w = c.post("/api/schedules", map[string]string{"Action": "list"})
	if w.Code != 401 || w.Header().Get("X-Hamster-Session") != "expired-admin" {
		t.Fatal("expired schedule session", w.Code)
	}
	if w := workspaceCall(b, c, "GET", "/schedules.html?session="+c.csrf, nil, nil); w.Code != 303 || !strings.Contains(w.Header().Get("Location"), "/admin/") {
		t.Fatal("expired schedule page did not return to administrator login", w.Code)
	}
}
