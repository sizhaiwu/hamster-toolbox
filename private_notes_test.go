package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const testNotesPassword = "test-only-long-passphrase-123"

func TestPrivateNotesUserChosenPasswords(t *testing.T) {
	for _, password := range []string{"1", "短", " 12 34 "} {
		t.Run(password, func(t *testing.T) {
			s := &server{token: "test-owner", dataDir: t.TempDir()}
			if w := notesCall(t, s, "", "create", notesRequest{}); w.Code == 200 {
				t.Fatal("empty master password accepted")
			}
			created := notesJSON(t, notesCall(t, s, "", "create", notesRequest{Password: password, NoExample: true}))
			defer s.notes.lock()
			session := notesString(t, created, "Session")
			notesSave(t, s, session, privateNote{Title: "Short password test", Markdown: "synthetic content"})
			before := notesDisk(t, s)
			if w := notesCall(t, s, session, "password", notesRequest{Password: password}); w.Code == 200 {
				t.Fatal("empty replacement password accepted")
			}
			if !bytes.Equal(before, notesDisk(t, s)) {
				t.Fatal("rejected password change modified vault")
			}
			notesJSON(t, notesCall(t, s, session, "password", notesRequest{Password: password, NewPassword: "2"}))
			notesJSON(t, notesCall(t, s, session, "lock", notesRequest{}))
			unlocked := notesJSON(t, notesCall(t, s, "", "unlock", notesRequest{Password: "2"}))
			var saved []privateNote
			json.Unmarshal(unlocked["Notes"], &saved)
			if len(saved) != 1 || saved[0].Markdown != "synthetic content" {
				t.Fatal("short password change lost notes")
			}
			// Existing backups retain their original, exact password, including spaces.
			env, e := parseNotesEnvelope(before)
			if e != nil {
				t.Fatal(e)
			}
			key := notesKey(password, env.notesHeader)
			defer clear(key)
			if _, e := openNotes(env, key); e != nil {
				t.Fatal("original backup password no longer works", e)
			}
		})
	}
}

func TestPrivateNotesAutoLockPersistenceAndMigration(t *testing.T) {
	s, session := notesNew(t)
	defer s.notes.lock()
	notesSave(t, s, session, privateNote{Title: "Timeout test", Markdown: "unchanged synthetic note"})
	status := notesJSON(t, notesCall(t, s, "", "status", notesRequest{}))
	if _, leaked := status["AutoLockMinutes"]; leaked {
		t.Fatal("encrypted preferences exposed while locked")
	}
	if s.notes.idle != notesIdle {
		t.Fatal("legacy default not retained")
	}
	minutes := 30
	response := notesJSON(t, notesCall(t, s, session, "auto-lock", notesRequest{AutoLockMinutes: &minutes}))
	if string(response["AutoLockMinutes"]) != "30" || s.notes.idle != 30*time.Minute {
		t.Fatal("timeout not applied")
	}
	before := notesDisk(t, s)
	for _, body := range []any{notesRequest{}, map[string]any{"AutoLockMinutes": -1}, map[string]any{"AutoLockMinutes": 10081}, map[string]any{"AutoLockMinutes": 1.5}} {
		if w := notesCall(t, s, session, "auto-lock", body); w.Code == 200 {
			t.Fatal("invalid timeout accepted")
		}
		if !bytes.Equal(before, notesDisk(t, s)) || s.notes.idle != 30*time.Minute {
			t.Fatal("rejected timeout changed preferences")
		}
	}
	restarted := &server{token: s.token, dataDir: s.dataDir}
	loaded := notesJSON(t, notesCall(t, restarted, "", "unlock", notesRequest{Password: testNotesPassword}))
	defer restarted.notes.lock()
	if string(loaded["AutoLockMinutes"]) != "30" || restarted.notes.idle != 30*time.Minute {
		t.Fatal("restart lost timeout")
	}
	migrated := &server{token: "migration-owner", dataDir: t.TempDir()}
	loaded = notesJSON(t, notesCall(t, migrated, "", "restore", notesRequest{Backup: before, Password: testNotesPassword, Confirm: true}))
	defer migrated.notes.lock()
	if string(loaded["AutoLockMinutes"]) != "30" {
		t.Fatal("backup migration lost timeout")
	}
	var saved []privateNote
	json.Unmarshal(loaded["Notes"], &saved)
	if len(saved) != 1 || saved[0].Markdown != "unchanged synthetic note" {
		t.Fatal("changing preferences changed note")
	}
	minutes = 0
	notesJSON(t, notesCall(t, s, session, "auto-lock", notesRequest{AutoLockMinutes: &minutes}))
	s.notes.mu.Lock()
	timerActive := s.notes.timer != nil
	s.notes.last = time.Now().Add(-24 * time.Hour)
	s.notes.mu.Unlock()
	if timerActive {
		t.Fatal("disabled timeout still schedules a timer")
	}
	notesJSON(t, notesCall(t, s, session, "keepalive", notesRequest{}))
	if s.notes.session == "" || s.notes.timer != nil {
		t.Fatal("never timeout locked a session")
	}
	minutes = 1
	notesJSON(t, notesCall(t, s, session, "auto-lock", notesRequest{AutoLockMinutes: &minutes}))
	s.notes.mu.Lock()
	s.notes.last = time.Now().Add(-61 * time.Second)
	s.notes.mu.Unlock()
	if w := notesCall(t, s, session, "list", notesRequest{}); w.Code != 401 {
		t.Fatal("configured server expiry not enforced")
	}
	if len(s.notes.key) != 0 {
		t.Fatal("expired key retained")
	}
}

func TestPrivateNotesIdleTimerClearsMemory(t *testing.T) {
	m := &notesMemory{key: make([]byte, 32), session: "synthetic-session", idle: 20 * time.Millisecond}
	m.mu.Lock()
	m.touch()
	m.mu.Unlock()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		m.mu.Lock()
		locked := m.session == "" && len(m.key) == 0
		m.mu.Unlock()
		if locked {
			return
		}
	}
	m.mu.Lock()
	m.lock()
	m.mu.Unlock()
	t.Fatal("background timer did not clear unlocked memory")
}

func TestPrivateNotesFontAndEditLocks(t *testing.T) {
	s, session := notesNew(t)
	note := notesSave(t, s, session, privateNote{Title: "Protected example", Markdown: "# Preserved content"})
	defaults := notesJSON(t, notesCall(t, s, session, "list", notesRequest{}))
	if string(defaults["FontSize"]) != "14" || note.Locked {
		t.Fatal("legacy defaults changed")
	}
	size := 22
	font := notesJSON(t, notesCall(t, s, session, "font-size", notesRequest{FontSize: &size}))
	if string(font["FontSize"]) != "22" {
		t.Fatal("font not saved")
	}
	before := notesDisk(t, s)
	for _, body := range []any{notesRequest{}, map[string]any{"FontSize": 9}, map[string]any{"FontSize": 41}, map[string]any{"FontSize": 14.5}} {
		if w := notesCall(t, s, session, "font-size", body); w.Code == 200 {
			t.Fatal("invalid font accepted")
		}
		if !bytes.Equal(before, notesDisk(t, s)) {
			t.Fatal("invalid font changed vault")
		}
	}
	preview := notesJSON(t, notesCall(t, s, session, "preview", notesRequest{Note: privateNote{Markdown: "# Example"}}))
	if !strings.Contains(notesString(t, preview, "HTML"), `class="notes-font-22"`) {
		t.Fatal("preview ignored saved font")
	}
	for _, fontSize := range []int{10, 14, 40} {
		html, e := notesMarkdownSized("# Example", fontSize)
		if e != nil || !strings.Contains(html, "default-src 'none'") {
			t.Fatal("font renderer lost isolation", e)
		}
	}
	if _, e := notesMarkdownSized("content", 1000); e == nil {
		t.Fatal("invalid preview font accepted")
	}
	locked := true
	v := notesJSON(t, notesCall(t, s, session, "note-lock", notesRequest{Note: note, Locked: &locked}))
	var protected privateNote
	json.Unmarshal(v["Note"], &protected)
	if !protected.Locked || protected.Revision != note.Revision+1 || protected.Markdown != note.Markdown || protected.Title != note.Title || protected.Updated != note.Updated {
		t.Fatal("locking changed content or lost revision")
	}
	before = notesDisk(t, s)
	forged := protected
	forged.Locked = false
	forged.Markdown = "unwanted overwrite"
	for _, action := range []string{"save", "delete"} {
		if w := notesCall(t, s, session, action, notesRequest{Note: forged, Confirm: true}); w.Code == 200 {
			t.Fatal("locked note mutated", action)
		}
		if !bytes.Equal(before, notesDisk(t, s)) {
			t.Fatal("locked note operation changed file")
		}
	}
	locked = false
	if w := notesCall(t, s, session, "note-lock", notesRequest{Note: note, Locked: &locked}); w.Code == 200 {
		t.Fatal("stale revision unlocked note")
	}
	// Both settings and locks survive restart and portable encrypted backups.
	for _, migrate := range []bool{false, true} {
		target := &server{token: "another-owner", dataDir: s.dataDir}
		action := "unlock"
		req := notesRequest{Password: testNotesPassword}
		if migrate {
			target.dataDir = t.TempDir()
			action = "restore"
			req.Backup = before
			req.Confirm = true
		}
		loaded := notesJSON(t, notesCall(t, target, "", action, req))
		var items []privateNote
		json.Unmarshal(loaded["Notes"], &items)
		if string(loaded["FontSize"]) != "22" || len(items) != 1 || !items[0].Locked || items[0].Markdown != note.Markdown {
			t.Fatal("restart/migration lost lock, font or content")
		}
		target.notes.mu.Lock()
		target.notes.lock()
		target.notes.mu.Unlock()
	}
	v = notesJSON(t, notesCall(t, s, session, "note-lock", notesRequest{Note: protected, Locked: &locked}))
	json.Unmarshal(v["Note"], &note)
	if note.Locked {
		t.Fatal("unlock did not release edit protection")
	}
	note.Markdown = "changed intentionally"
	note = notesSave(t, s, session, note)
	if note.Locked || note.Markdown != "changed intentionally" {
		t.Fatal("unlocked note not editable")
	}
	notesJSON(t, notesCall(t, s, session, "delete", notesRequest{Note: note, Confirm: true}))
}

func notesCall(t *testing.T, s *server, session, action string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, e := json.Marshal(body)
	if e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest("POST", "/api/notes/"+action, bytes.NewReader(b))
	r.Header.Set("X-Toolbox-Token", s.token)
	r.Header.Set("X-Notes-Session", session)
	w := httptest.NewRecorder()
	s.guard(s.api)(w, r)
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("confidential response is cacheable")
	}
	return w
}
func notesJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]json.RawMessage {
	t.Helper()
	if w.Code != 200 {
		t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
	}
	var v map[string]json.RawMessage
	if e := json.Unmarshal(w.Body.Bytes(), &v); e != nil {
		t.Fatal(e)
	}
	return v
}
func notesString(t *testing.T, v map[string]json.RawMessage, name string) string {
	t.Helper()
	var s string
	if e := json.Unmarshal(v[name], &s); e != nil {
		t.Fatal(e)
	}
	return s
}
func notesNew(t *testing.T) (*server, string) {
	t.Helper()
	s := &server{token: "test-owner", dataDir: t.TempDir()}
	v := notesJSON(t, notesCall(t, s, "", "create", notesRequest{Password: testNotesPassword, NoExample: true}))
	t.Cleanup(func() {
		if s.notes != nil {
			s.notes.mu.Lock()
			s.notes.lock()
			s.notes.mu.Unlock()
		}
	})
	return s, notesString(t, v, "Session")
}
func notesSave(t *testing.T, s *server, session string, n privateNote) privateNote {
	t.Helper()
	v := notesJSON(t, notesCall(t, s, session, "save", notesRequest{Note: n}))
	if e := json.Unmarshal(v["Note"], &n); e != nil {
		t.Fatal(e)
	}
	return n
}
func notesDisk(t *testing.T, s *server) []byte {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(s.dataDir, "private-notes", "vault.json"))
	if e != nil {
		t.Fatal(e)
	}
	return b
}

func TestPrivateNotesEncryptionLifecycle(t *testing.T) {
	s, session := notesNew(t)
	n := notesSave(t, s, session, privateNote{Title: "私密标题_TEST", Markdown: "# 密码\nsecret-body-test-123"})
	before := notesDisk(t, s)
	for _, sensitive := range []string{n.Title, n.Markdown, testNotesPassword, "secret-body-test-123"} {
		if bytes.Contains(before, []byte(sensitive)) {
			t.Fatal("plaintext leaked to disk")
		}
	}
	n.Markdown += "\nchanged"
	n = notesSave(t, s, session, n)
	after := notesDisk(t, s)
	if bytes.Equal(before, after) {
		t.Fatal("save did not use a new nonce")
	}
	for _, file := range []string{"vault.json", "vault.lock"} {
		st, e := os.Stat(filepath.Join(s.dataDir, "private-notes", file))
		if e != nil {
			t.Fatal(e)
		}
		if st.Size() > notesMaxFile {
			t.Fatal("unbounded vault")
		}
	}
	notesJSON(t, notesCall(t, s, session, "lock", notesRequest{}))
	if len(s.notes.key) != 0 {
		t.Fatal("lock retains derived key")
	}
	if w := notesCall(t, s, session, "list", notesRequest{}); w.Code != 401 {
		t.Fatal("old session works after lock")
	}
	if w := notesCall(t, s, "", "unlock", notesRequest{Password: "wrong-password"}); w.Code == 200 {
		t.Fatal("wrong password unlocked")
	}
	// A new process/server can read the same encrypted data; no machine key.
	restarted := &server{token: s.token, dataDir: s.dataDir}
	v := notesJSON(t, notesCall(t, restarted, "", "unlock", notesRequest{Password: testNotesPassword}))
	defer restarted.notes.lock()
	var got []privateNote
	json.Unmarshal(v["Notes"], &got)
	if len(got) != 1 || !reflect.DeepEqual(got[0], n) {
		t.Fatal("restart lost note", got)
	}
	old := n
	n.Title = "new title"
	n = notesSave(t, restarted, notesString(t, v, "Session"), n)
	if w := notesCall(t, restarted, notesString(t, v, "Session"), "save", notesRequest{Note: old}); w.Code == 200 {
		t.Fatal("stale edit overwrote current revision")
	}
	if w := notesCall(t, restarted, "wrong-session", "list", notesRequest{}); w.Code != 401 {
		t.Fatal("owner token alone reads plaintext")
	}
}
func TestPrivateNotesTamperAndMigration(t *testing.T) {
	s, session := notesNew(t)
	n := notesSave(t, s, session, privateNote{Title: "portable note", Markdown: "encrypted migration"})
	backup := notesDisk(t, s)
	env, e := parseNotesEnvelope(backup)
	if e != nil {
		t.Fatal(e)
	}
	key := notesKey(testNotesPassword, env.notesHeader)
	defer clear(key)
	for _, change := range []func(*notesEnvelope){func(e *notesEnvelope) { e.Ciphertext[0] ^= 1 }, func(e *notesEnvelope) { e.TwoFA = true }, func(e *notesEnvelope) { e.Nonce[0] ^= 1 }} {
		var copy notesEnvelope
		json.Unmarshal(backup, &copy)
		change(&copy)
		if _, err := openNotes(copy, key); err == nil {
			t.Fatal("tampered authenticated envelope accepted")
		}
	}
	malicious := env
	malicious.Memory = 1 << 30
	b, _ := json.Marshal(malicious)
	if _, e := parseNotesEnvelope(b); e == nil {
		t.Fatal("unbounded KDF accepted")
	}
	for _, req := range []notesRequest{{Backup: backup, Password: "wrong-password", Confirm: true}, {Backup: []byte("broken"), Password: testNotesPassword, Confirm: true}, {Backup: backup, Password: testNotesPassword}} {
		if w := notesCall(t, s, session, "restore", req); w.Code == 200 {
			t.Fatal("invalid restore succeeded")
		}
		if !bytes.Equal(backup, notesDisk(t, s)) {
			t.Fatal("failed restore changed original")
		}
	}
	if w := notesCall(t, s, "", "restore", notesRequest{Backup: backup, Password: testNotesPassword, Confirm: true}); w.Code != 401 {
		t.Fatal("locked vault replaced")
	}
	target := &server{token: "migration-owner", dataDir: t.TempDir()}
	v := notesJSON(t, notesCall(t, target, "", "restore", notesRequest{Backup: backup, Password: testNotesPassword, Confirm: true}))
	defer target.notes.lock()
	var got []privateNote
	json.Unmarshal(v["Notes"], &got)
	if len(got) != 1 || !reflect.DeepEqual(got[0], n) {
		t.Fatal("migration lost contents")
	}
	notesSave(t, s, session, privateNote{Title: "second", Markdown: "before replace"})
	before := notesDisk(t, s)
	v = notesJSON(t, notesCall(t, s, session, "restore", notesRequest{Backup: backup, Password: testNotesPassword, Confirm: true}))
	if len(v["Session"]) == 0 {
		t.Fatal("restore did not unlock")
	}
	prev, e := os.ReadFile(filepath.Join(s.dataDir, "private-notes", "before-import.hamster-notes"))
	if e != nil || !bytes.Equal(prev, before) {
		t.Fatal("replace did not preserve encrypted previous vault")
	}
}
func TestPrivateNotesTOTPAndRecovery(t *testing.T) {
	// RFC 6238 Appendix B, SHA-1 vector truncated to six digits.
	if got := notesTOTP("GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", 59/30); got != "287082" {
		t.Fatal("RFC TOTP vector", got)
	}
	s, session := notesNew(t)
	start := notesJSON(t, notesCall(t, s, session, "2fa-start", notesRequest{}))
	secret := notesString(t, start, "Secret")
	if secret == "" || len(start["QR"]) < 100 {
		t.Fatal("missing local enrollment QR")
	}
	if w := notesCall(t, s, session, "2fa-enable", notesRequest{Code: "invalid"}); w.Code == 200 {
		t.Fatal("invalid enrollment accepted")
	}
	code := notesTOTP(secret, time.Now().Unix()/30)
	v := notesJSON(t, notesCall(t, s, session, "2fa-enable", notesRequest{Code: code}))
	var recovery []string
	json.Unmarshal(v["RecoveryCodes"], &recovery)
	if len(recovery) != 8 {
		t.Fatal("missing recovery codes")
	}
	key := s.notes.key
	env, _ := parseNotesEnvelope(notesDisk(t, s))
	p, e := openNotes(env, key)
	if e != nil || p.OTP != secret || len(p.Recovery) != 8 {
		t.Fatal("2FA not persisted")
	}
	if strings.Contains(strings.Join(p.Recovery, ""), recovery[0]) {
		t.Fatal("recovery code stored unhashed")
	}
	notesJSON(t, notesCall(t, s, session, "lock", notesRequest{}))
	if w := notesCall(t, s, "", "unlock", notesRequest{Password: testNotesPassword, Code: code}); w.Code == 200 {
		t.Fatal("replayed enrollment step unlocked")
	}
	if w := notesCall(t, s, "", "unlock", notesRequest{Password: testNotesPassword}); w.Code == 200 {
		t.Fatal("2FA bypassed")
	}
	v = notesJSON(t, notesCall(t, s, "", "unlock", notesRequest{Password: testNotesPassword, Code: recovery[0]}))
	session = notesString(t, v, "Session")
	notesJSON(t, notesCall(t, s, session, "lock", notesRequest{}))
	if w := notesCall(t, s, "", "unlock", notesRequest{Password: testNotesPassword, Code: recovery[0]}); w.Code == 200 {
		t.Fatal("recovery code reused")
	}
	v = notesJSON(t, notesCall(t, s, "", "unlock", notesRequest{Password: testNotesPassword, Code: recovery[1]}))
	session = notesString(t, v, "Session")
	newPassword := "new-test-only-passphrase-456"
	notesJSON(t, notesCall(t, s, session, "password", notesRequest{Password: testNotesPassword, Code: recovery[2], NewPassword: newPassword}))
	notesJSON(t, notesCall(t, s, session, "lock", notesRequest{}))
	if w := notesCall(t, s, "", "unlock", notesRequest{Password: testNotesPassword, Code: recovery[3]}); w.Code == 200 {
		t.Fatal("old password accepted")
	}
	v = notesJSON(t, notesCall(t, s, "", "unlock", notesRequest{Password: newPassword, Code: recovery[3]}))
	session = notesString(t, v, "Session")
	notesJSON(t, notesCall(t, s, session, "2fa-disable", notesRequest{Password: newPassword, Code: recovery[4]}))
	notesJSON(t, notesCall(t, s, session, "lock", notesRequest{}))
	notesJSON(t, notesCall(t, s, "", "unlock", notesRequest{Password: newPassword}))
}
func TestPrivateNotesLimitsLockAndPermissions(t *testing.T) {
	s, session := notesNew(t)
	before := notesDisk(t, s)
	for _, n := range []privateNote{{Title: " ", Markdown: "x"}, {Title: strings.Repeat("x", 201)}, {Title: "large", Markdown: strings.Repeat("x", notesMaxText+1)}} {
		if w := notesCall(t, s, session, "save", notesRequest{Note: n}); w.Code == 200 {
			t.Fatal("invalid note accepted")
		}
	}
	if !bytes.Equal(before, notesDisk(t, s)) {
		t.Fatal("invalid notes changed original")
	}
	dir, closeDir, e := s.notesDirectory()
	if e != nil {
		t.Fatal(e)
	}
	defer dir.Close()
	if _, closeSecond, e := s.notesDirectory(); e == nil {
		closeSecond()
		t.Fatal("concurrent writer lock not enforced")
	}
	closeDir()
	s.notes.mu.Lock()
	s.notes.last = time.Now().Add(-notesIdle - time.Second)
	s.notes.mu.Unlock()
	if w := notesCall(t, s, session, "list", notesRequest{}); w.Code != 401 {
		t.Fatal("idle session remained usable")
	}
	r := httptest.NewRequest("POST", "/api/notes/list", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	s.guard(s.api)(w, r)
	if w.Code != 403 {
		t.Fatal("owner guard bypassed")
	}
	for _, path := range []string{"/notes/", "/notes/notes.html", "/notes/../vault.json", "/notes/vault.json"} {
		w = httptest.NewRecorder()
		s.notesPage(w, httptest.NewRequest("GET", path, nil))
		if w.Code == 200 {
			t.Fatal("private page or data exposed", path)
		}
	}
	w = httptest.NewRecorder()
	s.notesPage(w, httptest.NewRequest("GET", "/notes/?session="+s.token, nil))
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatal("owner page missing security policy")
	}
}
func TestPrivateNotesImportAtomicAndMarkdownIsolation(t *testing.T) {
	s, session := notesNew(t)
	before := notesDisk(t, s)
	req := notesRequest{Notes: []privateNote{{Title: "valid", Markdown: "content"}, {Title: "", Markdown: "bad"}}}
	if w := notesCall(t, s, session, "import-markdown", req); w.Code == 200 {
		t.Fatal("partial invalid import accepted")
	}
	if !bytes.Equal(before, notesDisk(t, s)) {
		t.Fatal("invalid import partially saved")
	}
	for _, source := range []string{"# Heading\n**bold**\n\n| A | B |\n|---|---|\n|1|2|", `<script>alert(1)</script><img src="https://example.com/track"><iframe src="https://example.com"></iframe>`, "[leak](https://example.com/?secret=test) ![track](https://example.com/track) <https://example.com> [bad](javascript:alert(1))"} {
		html, e := notesMarkdown(source)
		if e != nil {
			t.Fatal(e)
		}
		if strings.Count(html, "<script>") != 1 || !strings.Contains(html, "<script>"+notesPreviewScript+"</script>") {
			t.Fatal("unexpected preview script")
		}
		html = strings.Replace(html, "<script>"+notesPreviewScript+"</script>", "", 1)
		for _, needle := range []string{"<script", "<img", "<iframe", "href=", "src=", "javascript:"} {
			if strings.Contains(html, needle) {
				t.Fatal("active content in preview", needle, html)
			}
		}
		if !strings.Contains(html, "default-src 'none'") {
			t.Fatal("preview has no deny-all CSP")
		}
	}
	v := notesJSON(t, notesCall(t, s, session, "import-markdown", notesRequest{Notes: []privateNote{{Title: "Notion export", Markdown: "# Local"}}}))
	var imported []privateNote
	json.Unmarshal(v["Notes"], &imported)
	if len(imported) != 1 || imported[0].Revision != 1 {
		t.Fatal("import failed")
	}
	// Rate limiting rejects the next attempt before deriving another key.
	s.notes.mu.Lock()
	s.notes.failures = 5
	s.notes.nextTry = time.Now().Add(time.Minute)
	s.notes.mu.Unlock()
	if w := notesCall(t, s, "", "unlock", notesRequest{Password: testNotesPassword}); w.Code != http.StatusTooManyRequests {
		t.Fatal("unlock rate limit missing")
	}
}
