package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivateNotesInitialExampleOnlyOnce(t *testing.T) {
	s := &server{token: "example-owner", dataDir: t.TempDir()}
	data := notesJSON(t, notesCall(t, s, "", "create", notesRequest{Password: testNotesPassword}))
	defer s.notes.lock()
	session := notesString(t, data, "Session")
	var notes []privateNote
	json.Unmarshal(data["Notes"], &notes)
	if len(notes) != 1 || !strings.Contains(notes[0].Markdown, "富文本") || !notes[0].Pinned {
		t.Fatal("missing initial example")
	}
	notesJSON(t, notesCall(t, s, session, "delete", notesRequest{Note: notes[0], Confirm: true}))
	notesJSON(t, notesCall(t, s, session, "lock", notesRequest{}))
	data = notesJSON(t, notesCall(t, s, "", "unlock", notesRequest{Password: testNotesPassword}))
	json.Unmarshal(data["Notes"], &notes)
	if len(notes) != 0 {
		t.Fatal("deleted example recreated")
	}
}
func TestPrivateNotesAttachmentsEncryptedAndPortable(t *testing.T) {
	s, session := notesNew(t)
	defer s.notes.lock()
	n := notesSave(t, s, session, privateNote{Title: "Attachment example", Markdown: "body"})
	image, e := os.ReadFile("samples/assets/transparent-card.png")
	if e != nil {
		t.Fatal(e)
	}
	data := notesJSON(t, notesCall(t, s, session, "attachment-add", notesRequest{Note: n, Name: "sample-image.png", Content: image}))
	json.Unmarshal(data["Note"], &n)
	if len(n.Attachments) != 1 || n.Attachments[0].Mime != "image/png" {
		t.Fatal("image not attached")
	}
	first := n.Attachments[0]
	doc := []byte("SYNTHETIC-ATTACHMENT-PRIVATE-BODY")
	data = notesJSON(t, notesCall(t, s, session, "attachment-add", notesRequest{Note: n, Name: "示例说明.txt", Content: doc}))
	json.Unmarshal(data["Note"], &n)
	if len(n.Attachments) != 2 {
		t.Fatal("second file lost")
	}
	second := n.Attachments[1]
	env, _ := parseNotesEnvelope(notesDisk(t, s))
	p, e := openNotes(env, s.notes.key)
	if e != nil {
		t.Fatal(e)
	}
	files, _ := filepath.Glob(filepath.Join(s.dataDir, "private-notes", "attachments", "*.bin"))
	if len(files) != 2 {
		t.Fatal("missing encrypted files")
	}
	for _, file := range files {
		b, _ := os.ReadFile(file)
		if bytes.Contains(b, doc) || bytes.Contains(b, image) || bytes.Contains(b, []byte(second.Name)) {
			t.Fatal("attachment stored as plaintext")
		}
	}
	data = notesJSON(t, notesCall(t, s, session, "attachment-read", notesRequest{Note: n, AttachmentID: second.ID}))
	var downloaded []byte
	json.Unmarshal(data["Content"], &downloaded)
	if !bytes.Equal(downloaded, doc) {
		t.Fatal("download changed bytes")
	}
	bad := privateNote{ID: notesID()}
	if notesCall(t, s, session, "attachment-read", notesRequest{Note: bad, AttachmentID: second.ID}).Code == 200 {
		t.Fatal("unrelated note read attachment")
	}
	locked := true
	data = notesJSON(t, notesCall(t, s, session, "note-lock", notesRequest{Note: n, Locked: &locked}))
	json.Unmarshal(data["Note"], &n)
	if notesCall(t, s, session, "attachment-add", notesRequest{Note: n, Name: "bad.txt", Content: doc}).Code == 200 {
		t.Fatal("upload bypassed edit lock")
	}
	data = notesJSON(t, notesCall(t, s, session, "export", notesRequest{}))
	var backup []byte
	json.Unmarshal(data["Backup"], &backup)
	if !bytes.HasPrefix(backup, []byte("PK")) {
		t.Fatal("backup omitted attachment container")
	}
	bundle, e := unpackNotesBackup(backup)
	if e != nil || len(bundle.Files) != 2 {
		t.Fatal("bad complete backup", e)
	}
	// Changing the master password re-wraps the attachment key without touching blobs.
	notesJSON(t, notesCall(t, s, session, "password", notesRequest{Password: testNotesPassword, NewPassword: "different-master"}))
	data = notesJSON(t, notesCall(t, s, session, "attachment-read", notesRequest{Note: n, AttachmentID: first.ID}))
	json.Unmarshal(data["Content"], &downloaded)
	if !bytes.Equal(downloaded, image) {
		t.Fatal("password change broke attachments")
	}
	other := &server{token: "attachment-migration", dataDir: t.TempDir()}
	data = notesJSON(t, notesCall(t, other, "", "restore", notesRequest{Password: testNotesPassword, Confirm: true, Backup: backup}))
	defer other.notes.lock()
	newSession := notesString(t, data, "Session")
	data = notesJSON(t, notesCall(t, other, newSession, "attachment-read", notesRequest{Note: n, AttachmentID: second.ID}))
	json.Unmarshal(data["Content"], &downloaded)
	if !bytes.Equal(downloaded, doc) {
		t.Fatal("migration lost files")
	}
	// Backup rotation also includes pending latest attachments when locking.
	notesJSON(t, notesCall(t, s, session, "lock", notesRequest{}))
	paths, _ := filepath.Glob(filepath.Join(s.dataDir, "private-notes", "auto-backups", "*.hamster-notes"))
	found := false
	for _, path := range paths {
		b, _ := os.ReadFile(path)
		if b, e := unpackNotesBackup(b); e == nil && len(b.Files) == 2 {
			found = true
		}
	}
	if !found {
		t.Fatal("automatic backup omitted files")
	}
	_ = p
}
func TestPrivateNotesAttachmentBackupRejectsMissingAndTraversal(t *testing.T) {
	s, session := notesNew(t)
	defer s.notes.lock()
	n := notesSave(t, s, session, privateNote{Title: "source", Markdown: "kept"})
	data := notesJSON(t, notesCall(t, s, session, "attachment-add", notesRequest{Note: n, Name: "file.txt", Content: []byte("sample")}))
	json.Unmarshal(data["Note"], &n)
	rawVault := notesDisk(t, s)
	before := append([]byte(nil), rawVault...)
	if notesCall(t, s, session, "restore", notesRequest{Backup: rawVault, Password: testNotesPassword, Confirm: true}).Code == 200 {
		t.Fatal("incomplete vault-only backup restored with missing files")
	}
	if !bytes.Equal(before, notesDisk(t, s)) {
		t.Fatal("failed import modified vault")
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, _ := writer.Create("vault.json")
	entry.Write(rawVault)
	entry, _ = writer.Create("../escape.txt")
	entry.Write([]byte("invalid"))
	writer.Close()
	if _, e := unpackNotesBackup(buffer.Bytes()); e == nil {
		t.Fatal("traversal accepted")
	}
	if notesCall(t, s, session, "attachment-add", notesRequest{Note: n, Name: "../bad.txt", Content: []byte("bad")}).Code == 200 {
		t.Fatal("unsafe name accepted")
	}
}
func TestPrivateNotesCompleteBackupMultipartRestore(t *testing.T) {
	source, session := notesNew(t)
	defer source.notes.lock()
	n := notesSave(t, source, session, privateNote{Title: "multipart", Markdown: "body"})
	notesJSON(t, notesCall(t, source, session, "attachment-add", notesRequest{Note: n, Name: "data.txt", Content: []byte("encrypted file data")}))
	data := notesJSON(t, notesCall(t, source, session, "export", notesRequest{}))
	var backup []byte
	json.Unmarshal(data["Backup"], &backup)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	writer.WriteField("Password", testNotesPassword)
	writer.WriteField("Confirm", "true")
	writer.WriteField("Code", "")
	file, _ := writer.CreateFormFile("Backup", "example.hamster-notes")
	file.Write(backup)
	writer.Close()
	target := &server{token: "multipart-owner", dataDir: t.TempDir()}
	r := httptest.NewRequest("POST", "/api/notes/restore", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	r.Header.Set("X-Toolbox-Token", target.token)
	w := httptest.NewRecorder()
	target.guard(target.api)(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	defer target.notes.lock()
}
