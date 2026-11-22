package main

import (
	"bytes"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testWorkspacePassword = "synthetic-admin-only-passphrase"

type workspaceClient struct {
	b      *fileBrowser
	cookie *http.Cookie
	csrf   string
}

func workspaceFixture(t *testing.T) (*server, *fileBrowser, string) {
	t.Helper()
	owner, localSession := notesNew(t)
	b, e := newFileBrowser(owner.dataDir, owner.token)
	if e != nil {
		t.Fatal(e)
	}
	owner.browser = b
	salt := notesRandom(24)
	b.config.WorkspaceEnabled = true
	b.config.WorkspaceHTTP = true
	b.config.WorkspaceSalt = hex.EncodeToString(salt)
	b.config.WorkspaceHash = hex.EncodeToString(sharePassword(testWorkspacePassword, salt, 600000))
	b.workspace = newRemoteWorkspace(owner, owner.httpHandler())
	t.Cleanup(func() { b.workspace.revokeAll(); owner.notes.lock() })
	return owner, b, localSession
}
func workspaceCall(b *fileBrowser, c *workspaceClient, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	var data []byte
	if body != nil {
		data, _ = json.Marshal(body)
	}
	r := httptest.NewRequest(method, "http://workspace.test"+path, bytes.NewReader(data))
	r.RemoteAddr = "192.168.10.20:3333"
	r.Header.Set("Content-Type", "application/json")
	if c != nil && c.cookie != nil {
		r.AddCookie(c.cookie)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	b.remote(w, r)
	return w
}
func workspaceSignIn(t *testing.T, b *fileBrowser) *workspaceClient {
	t.Helper()
	w := workspaceCall(b, nil, "POST", "/admin/login", map[string]string{"Password": testWorkspacePassword}, map[string]string{"X-Hamster-Browser": "1"})
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var result struct{ URL string }
	json.Unmarshal(w.Body.Bytes(), &result)
	u, _ := url.Parse(result.URL)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("admin cookie policy")
	}
	if strings.Contains(result.URL, b.ownerToken) {
		t.Fatal("local token exposed")
	}
	return &workspaceClient{b, cookies[0], u.Query().Get("session")}
}
func (c *workspaceClient) post(path string, body any) *httptest.ResponseRecorder {
	return workspaceCall(c.b, c, "POST", path, body, map[string]string{"X-Toolbox-Token": c.csrf})
}
func (c *workspaceClient) notes(action string, body notesRequest, session string) *httptest.ResponseRecorder {
	return workspaceCall(c.b, c, "POST", "/api/notes/"+action, body, map[string]string{"X-Toolbox-Token": c.csrf, "X-Notes-Session": session})
}

func TestRemoteWorkspacePermissionAndTransport(t *testing.T) {
	owner, b, _ := workspaceFixture(t)
	owner.state = state{Status: "idle", Logs: []string{}}
	b.config.WorkspaceEnabled = false
	if w := workspaceCall(b, nil, "GET", "/admin/", nil, nil); w.Code != 404 {
		t.Fatal("disabled admin", w.Code)
	}
	b.config.WorkspaceEnabled = true
	b.config.WorkspaceHTTP = false
	for _, path := range []string{"/admin/", "/admin/login"} {
		if w := workspaceCall(b, nil, "POST", path, nil, map[string]string{"X-Forwarded-Proto": "https"}); w.Code != 403 {
			t.Fatal("insecure transport accepted", path, w.Code)
		}
	}
	r := httptest.NewRequest("GET", "https://workspace.test/admin/", nil)
	r.RemoteAddr = "192.168.10.20:3000"
	r.TLS = &tls.ConnectionState{}
	w := httptest.NewRecorder()
	b.remote(w, r)
	if w.Code != 200 {
		t.Fatal("HTTPS gate unavailable", w.Code)
	}
	b.config.WorkspaceHTTP = true
	b.sessions["synthetic-file-guest"] = guestSession{Expires: time.Now().Add(time.Hour)}
	guest := &workspaceClient{b: b, cookie: &http.Cookie{Name: "hamster_guest_" + b.cookieTag, Value: "synthetic-file-guest"}}
	if w := workspaceCall(b, guest, "POST", "/api/defaults", nil, map[string]string{"X-Hamster-Browser": "1"}); w.Code == 200 {
		t.Fatal("ordinary file guest obtained administrator API")
	}
	if w := workspaceCall(b, nil, "POST", "/admin/login", map[string]string{"Password": "file-guest-password"}, map[string]string{"X-Hamster-Browser": "1"}); w.Code != 401 {
		t.Fatal("bad administrator password", w.Code)
	}
	c := workspaceSignIn(t, b)
	if w := workspaceCall(b, nil, "POST", "/api/defaults", nil, map[string]string{"X-Toolbox-Token": owner.token}); w.Code == 200 {
		t.Fatal("local token bypassed remote gate")
	}
	if w := workspaceCall(b, nil, "POST", "/api/defaults", nil, map[string]string{"X-Toolbox-Token": c.csrf}); w.Code == 200 {
		t.Fatal("CSRF capability without cookie")
	}
	if w := workspaceCall(b, c, "POST", "/api/defaults", nil, nil); w.Code != 403 {
		t.Fatal("cookie without CSRF", w.Code)
	}
	if w := workspaceCall(b, c, "POST", "/api/defaults", nil, map[string]string{"X-Toolbox-Token": c.csrf, "Origin": "https://evil.test"}); w.Code != 403 {
		t.Fatal("cross-origin admin", w.Code)
	}
	if w := c.post("/api/defaults", nil); w.Code != 200 {
		t.Fatal("toolbox API", w.Code, w.Body.String())
	}
	for _, path := range []string{"/?session=" + c.csrf, "/files/?session=" + c.csrf, "/notes/?session=" + c.csrf} {
		w := workspaceCall(b, c, "GET", path, nil, nil)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "remote-workspace.js") || strings.Contains(w.Body.String(), owner.token) {
			t.Fatal("remote page", path, w.Code)
		}
		if len(w.Result().Cookies()) != 0 {
			t.Fatal("internal owner cookie leaked")
		}
	}
	w = workspaceCall(b, c, "POST", "/files/api/boot", nil, map[string]string{"X-Hamster-Browser": "1"})
	if w.Code != 200 || strings.Contains(w.Body.String(), owner.token) || !strings.Contains(w.Body.String(), c.csrf) {
		t.Fatal("owner boot capability leak", w.Code, w.Body.String())
	}
	for _, path := range []string{"/style.css", "/files/explorer.css", "/notes/notes.js", "/app.js", "/remote-workspace.js", "/remote-workspace.css"} {
		w := workspaceCall(b, c, "GET", path, nil, nil)
		if w.Code != 200 {
			t.Fatal("missing remote asset", path, w.Code)
		}
	}
	w = workspaceCall(b, c, "GET", "/files", nil, nil)
	if (w.Code != 301 && w.Code != 307) || strings.Contains(w.Header().Get("Location"), owner.token) || !strings.Contains(w.Header().Get("Location"), c.csrf) {
		t.Fatal("canonical redirect exposed local token", w.Code, w.Header().Get("Location"))
	}
	file := filepath.Join(t.TempDir(), "synthetic.txt")
	os.WriteFile(file, []byte("0123456789"), 0600)
	w = workspaceCall(b, c, "GET", "/files/raw?path="+url.QueryEscape(file), nil, map[string]string{"Range": "bytes=2-5"})
	if w.Code != 206 || w.Body.String() != "2345" {
		t.Fatal("remote range stream", w.Code, w.Body.String())
	}
	if w := c.post("/admin/logout", nil); w.Code != 200 {
		t.Fatal("logout", w.Code)
	}
	if w := c.post("/api/defaults", nil); w.Code == 200 {
		t.Fatal("logout retained admin access")
	}
}
func TestRemoteWorkspaceNotesIsolationEditingAndAttachments(t *testing.T) {
	owner, b, localSession := workspaceFixture(t)
	a, c := workspaceSignIn(t, b), workspaceSignIn(t, b)
	if w := a.notes("list", notesRequest{}, localSession); w.Code != 401 {
		t.Fatal("local notes unlock leaked to phone", w.Code)
	}
	av := notesJSON(t, a.notes("unlock", notesRequest{Password: testNotesPassword}, ""))
	as := notesString(t, av, "Session")
	if w := c.notes("list", notesRequest{}, as); w.Code != 401 {
		t.Fatal("one browser used another notes session", w.Code)
	}
	cv := notesJSON(t, c.notes("unlock", notesRequest{Password: testNotesPassword}, ""))
	cs := notesString(t, cv, "Session")
	if cs == as || cs == localSession {
		t.Fatal("notes session reused")
	}
	value := notesJSON(t, a.notes("save", notesRequest{Note: privateNote{Title: "Phone synthetic note", Markdown: "# phone body"}}, as))
	var note privateNote
	json.Unmarshal(value["Note"], &note)
	content := []byte("SYNTHETIC PHONE ATTACHMENT")
	value = notesJSON(t, a.notes("attachment-add", notesRequest{Note: note, Name: "phone.txt", Content: content}, as))
	json.Unmarshal(value["Note"], &note)
	value = notesJSON(t, c.notes("list", notesRequest{}, cs))
	var all []privateNote
	json.Unmarshal(value["Notes"], &all)
	if len(all) != 1 || all[0].ID != note.ID {
		t.Fatal("phone changes not visible to other device")
	}
	stale := note
	note.Markdown = "edited from phone"
	value = notesJSON(t, a.notes("save", notesRequest{Note: note}, as))
	json.Unmarshal(value["Note"], &note)
	stale.Markdown = "stale overwrite"
	if w := c.notes("save", notesRequest{Note: stale}, cs); w.Code == 200 {
		t.Fatal("stale device overwrote newer content")
	}
	value = notesJSON(t, c.notes("attachment-read", notesRequest{Note: note, AttachmentID: note.Attachments[0].ID}, cs))
	var actual []byte
	json.Unmarshal(value["Content"], &actual)
	if !bytes.Equal(actual, content) {
		t.Fatal("attachment download differs")
	}
	value = notesJSON(t, notesCall(t, owner, localSession, "list", notesRequest{}))
	json.Unmarshal(value["Notes"], &all)
	if len(all) != 1 || all[0].Markdown != note.Markdown {
		t.Fatal("local session lost or phone edits not persisted")
	}
	if w := a.post("/admin/logout", nil); w.Code != 200 {
		t.Fatal(w.Code)
	}
	notesJSON(t, c.notes("list", notesRequest{}, cs))
	notesJSON(t, notesCall(t, owner, localSession, "list", notesRequest{}))
	if bytes.Contains(notesDisk(t, owner), content) || bytes.Contains(notesDisk(t, owner), []byte(note.Markdown)) {
		t.Fatal("remote note stored plaintext")
	}
	b.workspace.revokeAll()
	if w := c.notes("list", notesRequest{}, cs); w.Code == 200 {
		t.Fatal("revocation retained access")
	}
}
func TestRemoteWorkspaceSessionExpiryAndSavedConfig(t *testing.T) {
	_, b, _ := workspaceFixture(t)
	c := workspaceSignIn(t, b)
	b.workspace.mu.Lock()
	session := b.workspace.sessions[c.cookie.Value]
	polledAt := session.last
	b.workspace.mu.Unlock()
	c.post("/api/state", nil)
	b.workspace.mu.Lock()
	renewed := !session.last.Equal(polledAt)
	session.last = time.Now().Add(-31 * time.Minute)
	b.workspace.mu.Unlock()
	if renewed {
		t.Fatal("background task polling renewed admin idle timeout")
	}
	if w := c.post("/api/defaults", nil); w.Code == 200 {
		t.Fatal("expired session accepted")
	}
	req := httptest.NewRequest("POST", "http://local/api/sharing", strings.NewReader(`{"Enabled":false,"WorkspaceEnabled":true,"WorkspaceHTTP":true,"WorkspacePassword":"new-synthetic-admin-passphrase","Port":18788,"Roots":[]}`))
	if e := b.saveSharing(req); e != nil {
		t.Fatal(e)
	}
	loaded, e := newFileBrowser(b.dataDir, b.ownerToken)
	if e != nil || !loaded.config.WorkspaceEnabled || !loaded.config.WorkspaceHTTP || validateWorkspaceConfig(loaded.config) != nil {
		t.Fatal("workspace preference not saved", e)
	}
	data, _ := os.ReadFile(filepath.Join(b.dataDir, "sharing.json"))
	if bytes.Contains(data, []byte("new-synthetic-admin-passphrase")) {
		t.Fatal("admin password saved as plaintext")
	}
	// A saved administrator-only workspace can start with no guest password or
	// shared folders. The listener remains loopback-only in this test.
	loaded.config.Enabled = true
	loaded.listen = func(string, string) (net.Listener, error) { return net.Listen("tcp4", "127.0.0.1:0") }
	if err := loaded.startSavedSharing(); err != nil {
		t.Fatal("administrator-only restart failed", err)
	}
	if loaded.server == nil {
		t.Fatal("saved workspace listener did not start")
	}
	t.Cleanup(func() { loaded.server.Close() })
}
func TestRemoteWorkspaceNotesRequireSecondFactor(t *testing.T) {
	owner, b, localSession := workspaceFixture(t)
	enrollment := notesJSON(t, notesCall(t, owner, localSession, "2fa-start", notesRequest{}))
	secret := notesString(t, enrollment, "Secret")
	notesJSON(t, notesCall(t, owner, localSession, "2fa-enable", notesRequest{Code: notesTOTP(secret, time.Now().Unix()/30)}))
	c := workspaceSignIn(t, b)
	if w := c.notes("unlock", notesRequest{Password: testNotesPassword}, ""); w.Code == 200 {
		t.Fatal("remote notes bypassed second factor")
	}
	notesJSON(t, c.notes("unlock", notesRequest{Password: testNotesPassword, Code: notesTOTP(secret, time.Now().Unix()/30+1)}, ""))
}

// Optional end-to-end browser fixture uses synthetic data and no real settings.
func TestRemoteWorkspaceBrowserAcceptance(t *testing.T) {
	output := os.Getenv("TOOLBOX_REMOTE_QA")
	if output == "" {
		t.Skip("optional browser fixture")
	}
	owner, b, _ := workspaceFixture(t)
	_ = notesSave(t, owner, owner.notes.session, privateNote{Title: "手机远程笔记示例", Markdown: "# 合成测试笔记\n\n可以从手机编辑和添加附件。"})
	// Display the guest/admin choice on the login card without sharing user files.
	b.config.Roots = []sharedFolder{{ID: "synthetic-login", Name: "演示共享", Path: t.TempDir()}}
	owner.state = state{Status: "idle", Logs: []string{}}
	srv := httptest.NewServer(http.HandlerFunc(b.remote))
	defer srv.Close()
	data, _ := json.Marshal(map[string]string{"URL": srv.URL + "/admin/", "Password": testWorkspacePassword, "NotesPassword": testNotesPassword, "StopFile": output + ".stop"})
	if e := os.WriteFile(output, data, 0600); e != nil {
		t.Fatal(e)
	}
	for end := time.Now().Add(8 * time.Minute); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
		if f, e := os.Open(output + ".stop"); e == nil {
			io.Copy(io.Discard, f)
			f.Close()
			return
		}
	}
	t.Fatal("browser acceptance timeout")
}
