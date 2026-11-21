package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPrivateNotesResumeAuthenticatedSession(t *testing.T) {
	s, session := notesNew(t)
	defer s.notes.lock()
	note := notesSave(t, s, session, privateNote{Title: "Synthetic resume note", Markdown: "local encrypted content"})
	for _, invalid := range []string{"", "wrong-session"} {
		w := notesCall(t, s, invalid, "resume", notesRequest{})
		if w.Code != http.StatusUnauthorized || strings.Contains(w.Body.String(), note.Markdown) {
			t.Fatal("unauthenticated resume", w.Code)
		}
	}
	data := notesJSON(t, notesCall(t, s, session, "resume", notesRequest{}))
	if notesString(t, data, "Session") != session {
		t.Fatal("resume unexpectedly rotated session")
	}
	var notes []privateNote
	json.Unmarshal(data["Notes"], &notes)
	if len(notes) != 1 || notes[0].Markdown != note.Markdown {
		t.Fatal("resume lost notes")
	}
	for _, field := range []string{"VaultID", "AutoLockMinutes", "FontSize", "EditorSettings", "BackupSettings", "TwoFA"} {
		if _, ok := data[field]; !ok {
			t.Fatal("missing restored field", field)
		}
	}
	restarted := &server{token: s.token, dataDir: s.dataDir}
	if notesCall(t, restarted, session, "resume", notesRequest{}).Code != http.StatusUnauthorized {
		t.Fatal("resume survived service restart")
	}
	notesJSON(t, notesCall(t, s, session, "lock", notesRequest{}))
	if notesCall(t, s, session, "resume", notesRequest{}).Code != http.StatusUnauthorized {
		t.Fatal("resume bypassed manual lock")
	}
	unlocked := notesJSON(t, notesCall(t, s, "", "unlock", notesRequest{Password: testNotesPassword}))
	next := notesString(t, unlocked, "Session")
	if next == session {
		t.Fatal("unlock reused old capability")
	}
	if notesCall(t, s, session, "resume", notesRequest{}).Code != http.StatusUnauthorized {
		t.Fatal("old capability still accepted")
	}
	s.notes.mu.Lock()
	s.notes.last = time.Now().Add(-11 * time.Minute)
	s.notes.mu.Unlock()
	if notesCall(t, s, next, "resume", notesRequest{}).Code != http.StatusUnauthorized {
		t.Fatal("resume bypassed idle expiry")
	}
}
func TestPrivateNotesResumeRejectsReplacedVault(t *testing.T) {
	s, session := notesNew(t)
	defer s.notes.lock()
	other := &server{token: s.token, dataDir: s.dataDir}
	data := notesJSON(t, notesCall(t, other, "", "unlock", notesRequest{Password: testNotesPassword}))
	defer other.notes.lock()
	notesJSON(t, notesCall(t, other, notesString(t, data, "Session"), "password", notesRequest{Password: testNotesPassword, NewPassword: "replacement-password"}))
	if notesCall(t, s, session, "resume", notesRequest{}).Code != http.StatusUnauthorized {
		t.Fatal("resume bypassed external password replacement")
	}
}
