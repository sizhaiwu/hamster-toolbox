package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPrivateNotesHistoryTrashAndMetadata(t *testing.T) {
	s, session := notesNew(t)
	defer s.notes.lock()
	n := notesSave(t, s, session, privateNote{Title: "synthetic note", Markdown: "before change", Category: "examples", Tags: []string{"sample"}, Pinned: true})
	old := n
	n.Markdown = "after change"
	n = notesSave(t, s, session, n)
	var list struct{ Versions []notesVersion }
	w := notesCall(t, s, session, "history", notesRequest{Note: n})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	json.Unmarshal(w.Body.Bytes(), &list)
	if len(list.Versions) != 1 {
		t.Fatal("missing history")
	}
	id := list.Versions[0].ID
	detail := notesJSON(t, notesCall(t, s, session, "history-read", notesRequest{Note: n, VersionID: id}))
	var version notesVersion
	json.Unmarshal(detail["Version"], &version)
	if version.Note.Markdown != old.Markdown || !version.Note.Pinned {
		t.Fatal("history lost body or metadata")
	}
	locked := true
	reply := notesJSON(t, notesCall(t, s, session, "note-lock", notesRequest{Note: n, Locked: &locked}))
	json.Unmarshal(reply["Note"], &n)
	if notesCall(t, s, session, "history-restore", notesRequest{Note: n, VersionID: id, Confirm: true}).Code == 200 {
		t.Fatal("history bypassed edit lock")
	}
	locked = false
	reply = notesJSON(t, notesCall(t, s, session, "note-lock", notesRequest{Note: n, Locked: &locked}))
	json.Unmarshal(reply["Note"], &n)
	reply = notesJSON(t, notesCall(t, s, session, "history-restore", notesRequest{Note: n, VersionID: id, Confirm: true}))
	json.Unmarshal(reply["Note"], &n)
	if n.Markdown != "before change" || n.Revision <= old.Revision {
		t.Fatal("restore revision/content")
	}
	notesJSON(t, notesCall(t, s, session, "delete", notesRequest{Note: n, Confirm: true}))
	var trash struct{ Trash []struct{ ID, Title string } }
	w = notesCall(t, s, session, "trash", notesRequest{})
	json.Unmarshal(w.Body.Bytes(), &trash)
	if len(trash.Trash) != 1 || trash.Trash[0].Title != n.Title {
		t.Fatal("delete is not recoverable")
	}
	reply = notesJSON(t, notesCall(t, s, session, "trash-restore", notesRequest{Note: n, Confirm: true}))
	json.Unmarshal(reply["Note"], &n)
	if !n.Pinned || len(n.Tags) != 1 || n.Category != "examples" {
		t.Fatal("metadata lost on recovery")
	}
	notesJSON(t, notesCall(t, s, session, "delete", notesRequest{Note: n, Confirm: true}))
	if notesCall(t, s, session, "trash-delete", notesRequest{Note: n}).Code == 200 {
		t.Fatal("unconfirmed permanent deletion")
	}
	notesJSON(t, notesCall(t, s, session, "trash-delete", notesRequest{Note: n, Confirm: true}))
	reply = notesJSON(t, notesCall(t, s, session, "history", notesRequest{Note: n}))
	if string(reply["Versions"]) != "[]" {
		t.Fatal("permanent deletion retained accessible history")
	}
	for _, text := range []string{"synthetic note", "before change", "after change", "examples"} {
		if bytes.Contains(notesDisk(t, s), []byte(text)) {
			t.Fatal("plaintext archive/metadata on disk")
		}
	}
}

func TestPrivateNotesCorruptRecoveryAndLock(t *testing.T) {
	s, session := notesNew(t)
	defer s.notes.lock()
	notesSave(t, s, session, privateNote{Title: "restore example", Markdown: "kept content"})
	backup := notesDisk(t, s)
	vault := filepath.Join(s.dataDir, "private-notes", "vault.json")
	if err := os.WriteFile(vault, []byte("broken synthetic file"), 0600); err != nil {
		t.Fatal(err)
	}
	status := notesJSON(t, notesCall(t, s, "", "status", notesRequest{}))
	if string(status["Damaged"]) != "true" {
		t.Fatal("corruption not reported")
	}
	notesJSON(t, notesCall(t, s, session, "lock", notesRequest{}))
	if len(s.notes.key) != 0 || s.notes.session != "" {
		t.Fatal("broken disk prevented memory lock")
	}
	before, _ := os.ReadFile(vault)
	if notesCall(t, s, "", "restore", notesRequest{Password: "wrong", Backup: backup, Confirm: true}).Code == 200 {
		t.Fatal("bad password restored")
	}
	after, _ := os.ReadFile(vault)
	if !bytes.Equal(before, after) {
		t.Fatal("failed recovery modified broken original")
	}
	restored := notesJSON(t, notesCall(t, s, "", "restore", notesRequest{Password: testNotesPassword, Backup: backup, Confirm: true}))
	var notes []privateNote
	json.Unmarshal(restored["Notes"], &notes)
	if len(notes) != 1 || notes[0].Markdown != "kept content" {
		t.Fatal("valid backup not recoverable")
	}
	saved, _ := filepath.Glob(filepath.Join(s.dataDir, "private-notes", "damaged-*.hamster-notes"))
	if len(saved) != 1 {
		t.Fatal("broken file not preserved")
	}
	original, _ := os.ReadFile(saved[0])
	if !bytes.Equal(original, before) {
		t.Fatal("wrong quarantined file")
	}
}

func TestPrivateNotesAutomaticBackupsAndFailureIsolation(t *testing.T) {
	s, session := notesNew(t)
	defer s.notes.lock()
	n := notesSave(t, s, session, privateNote{Title: "backup example", Markdown: "first"})
	status := notesJSON(t, notesCall(t, s, session, "backup-status", notesRequest{}))
	if string(status["Count"]) != "1" {
		t.Fatal("first save did not back up")
	}
	backupDir := s.notesBackupDirectory(notesBackupSettings{})
	if err := os.WriteFile(filepath.Join(backupDir, "keep-user-file.txt"), []byte("unrelated synthetic file"), 0600); err != nil {
		t.Fatal(err)
	}
	config := &notesBackupSettings{Enabled: true, Keep: 2, IntervalMinutes: 1}
	notesJSON(t, notesCall(t, s, session, "backup-settings", notesRequest{BackupSettings: config}))
	for i := 0; i < 4; i++ {
		notesJSON(t, notesCall(t, s, session, "backup-now", notesRequest{}))
	}
	status = notesJSON(t, notesCall(t, s, session, "backup-status", notesRequest{}))
	if string(status["Count"]) != "2" {
		t.Fatal("retention ignored")
	}
	if _, err := os.Stat(filepath.Join(backupDir, "keep-user-file.txt")); err != nil {
		t.Fatal("unrelated file removed")
	}
	paths, _ := filepath.Glob(filepath.Join(backupDir, "*.hamster-notes"))
	for _, path := range paths {
		b, _ := os.ReadFile(path)
		env, err := parseNotesEnvelope(b)
		if err != nil {
			t.Fatal(err)
		}
		key := notesKey(testNotesPassword, env.notesHeader)
		p, err := openNotes(env, key)
		clear(key)
		if err != nil || len(p.Notes) != 1 {
			t.Fatal("invalid automatic backup")
		}
		if bytes.Contains(b, []byte(n.Title)) {
			t.Fatal("plaintext backup")
		}
	}
	blocker := filepath.Join(t.TempDir(), "file-not-directory")
	os.WriteFile(blocker, []byte("x"), 0600)
	config.Directory = blocker
	reply := notesJSON(t, notesCall(t, s, session, "backup-settings", notesRequest{BackupSettings: config}))
	_ = reply
	s.notes.lastBackup = time.Time{}
	n.Markdown = "still saved when backup fails"
	reply = notesJSON(t, notesCall(t, s, session, "save", notesRequest{Note: n}))
	if len(reply["BackupWarning"]) < 3 {
		t.Fatal("backup failure not visible")
	}
	json.Unmarshal(reply["Note"], &n)
	if n.Markdown != "still saved when backup fails" {
		t.Fatal("backup failure lost successful save")
	}
}

func TestPrivateNotesDraftEncryptionAndCacheIsolation(t *testing.T) {
	s, session := notesNew(t)
	defer s.notes.lock()
	n := notesSave(t, s, session, privateNote{Title: "cache", Markdown: "original"})
	salt := base64.StdEncoding.EncodeToString(make([]byte, 16))
	keyReply := notesJSON(t, notesCall(t, s, session, "draft-key", notesRequest{Draft: json.RawMessage(`{"Salt":"` + salt + `"}`)}))
	var key []byte
	json.Unmarshal(keyReply["Key"], &key)
	if len(key) != 32 || bytes.Equal(key, s.notes.key) {
		t.Fatal("draft key not separated")
	}
	clear(key)
	env, _ := parseNotesEnvelope(notesDisk(t, s))
	p, err := openNotes(env, s.notes.key)
	if err != nil {
		t.Fatal(err)
	}
	draft, _ := json.Marshal(encryptedNotesDraft{Version: 2, VaultID: p.ID, Salt: salt, IV: base64.StdEncoding.EncodeToString(make([]byte, 12)), Ciphertext: base64.StdEncoding.EncodeToString(make([]byte, 32))})
	notesJSON(t, notesCall(t, s, session, "draft-put", notesRequest{Draft: draft}))
	before := notesDisk(t, s)
	reply := notesJSON(t, notesCall(t, s, session, "draft-get", notesRequest{}))
	if !bytes.Equal(reply["Draft"], draft) {
		t.Fatal("opaque encrypted draft lost")
	}
	if !bytes.Equal(before, notesDisk(t, s)) {
		t.Fatal("draft modified main vault")
	}
	if notesCall(t, s, session, "draft-put", notesRequest{Draft: json.RawMessage(`{"Version":2,"Title":"plaintext"}`)}).Code == 200 {
		t.Fatal("plaintext draft accepted")
	}
	cached := s.notes.cache
	for i := 0; i < 8; i++ {
		notesJSON(t, notesCall(t, s, session, "preview", notesRequest{Note: privateNote{Markdown: "# fast preview"}}))
	}
	if s.notes.cache != cached {
		t.Fatal("unchanged preview unnecessarily decrypted/rebuilt vault")
	}
	// Mutating a rejected clone must never change the authenticated cached payload.
	got, err := notesCachedPayload(s.notes, env, before)
	if err != nil {
		t.Fatal(err)
	}
	got.Notes[0].Markdown = "unsaved mutation"
	if s.notes.cache.Notes[0].Markdown != n.Markdown {
		t.Fatal("cache aliases mutation payload")
	}
	notesJSON(t, notesCall(t, s, session, "lock", notesRequest{}))
	if s.notes.cache != nil {
		t.Fatal("lock retained decrypted cache")
	}
	if notesCall(t, s, session, "draft-get", notesRequest{}).Code != 401 {
		t.Fatal("locked draft exposed")
	}
}

func TestPrivateNotesArchiveLimitsAndEditorPreferences(t *testing.T) {
	s, session := notesNew(t)
	defer s.notes.lock()
	n := notesSave(t, s, session, privateNote{Title: "settings", Markdown: "value"})
	editor := &notesEditorSettings{Mode: "rich", Sort: "name", Category: "examples", Positions: map[string]notesPosition{n.ID: {Cursor: 3, Scroll: 900.75}}}
	notesJSON(t, notesCall(t, s, session, "editor-settings", notesRequest{EditorSettings: editor}))
	other := &server{token: "other", dataDir: s.dataDir}
	reply := notesJSON(t, notesCall(t, other, "", "unlock", notesRequest{Password: testNotesPassword}))
	defer other.notes.lock()
	var settings notesEditorSettings
	json.Unmarshal(reply["EditorSettings"], &settings)
	if settings.Positions[n.ID].Scroll != 900.75 || settings.Mode != "rich" {
		t.Fatal("preferences lost across instances")
	}
	payload := notesPayload{ID: notesID(), Notes: []privateNote{n}}
	for i := 0; i < 50; i++ {
		notesAddVersion(&payload, n, true)
	}
	if len(payload.History) != 20 {
		t.Fatal("per-note retention ignored")
	}
	for i := 0; i < 230; i++ {
		note := n
		note.ID = notesID()
		payload.Trash = append(payload.Trash, notesDeleted{Note: note, Deleted: time.Now()})
	}
	notesTrimArchives(&payload)
	if len(payload.Trash) != 200 {
		t.Fatal("trash retention ignored")
	}
}

func BenchmarkPrivateNotesLargeVaultPreview(b *testing.B) {
	s := &server{token: "benchmark-owner", dataDir: b.TempDir(), notes: &notesMemory{idle: notesIdle}}
	key := make([]byte, 32)
	p := notesPayload{ID: notesID()}
	for i := 0; i < 1500; i++ {
		p.Notes = append(p.Notes, privateNote{ID: notesID(), Title: fmt.Sprint("Synthetic ", i), Markdown: strings.Repeat("x", 4096), Revision: 1})
	}
	h := newNotesHeader()
	dir, closeDir, e := s.notesDirectory()
	if e != nil {
		b.Fatal(e)
	}
	config := notesBackupSettings{Enabled: false, Keep: 2, IntervalMinutes: 30}
	p.BackupSettings = &config
	session := s.notes.activate(key, 10)
	if e = s.commitNotes(dir, s.notes, h, p, key); e != nil {
		b.Fatal(e)
	}
	closeDir()
	defer s.notes.lock()
	handler := s.guard(s.api)
	body := `{"Note":{"Markdown":"# Preview\nsmall paragraph"}}`
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := httptest.NewRequest("POST", "/api/notes/preview", strings.NewReader(body))
		r.Header.Set("X-Toolbox-Token", s.token)
		r.Header.Set("X-Notes-Session", session)
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != 200 {
			b.Fatal(w.Body.String())
		}
	}
}
