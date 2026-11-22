package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSharedSettingsPagesAndPermissions(t *testing.T) {
	s, _ := notesNew(t)
	b, err := newFileBrowser(s.dataDir, s.token)
	if err != nil {
		t.Fatal(err)
	}
	s.browser = b
	h := s.httpHandler()
	for _, path := range []string{"/", "/files/", "/notes/"} {
		r := httptest.NewRequest("GET", "http://local"+path+"?session="+s.token, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		page := w.Body.String()
		if w.Code != 200 || strings.Count(page, `id="generalSettingsDialog"`) != 1 || strings.Count(page, `id="sharingDialog"`) != 1 || !strings.Contains(page, `id="generalTabNetwork"`) {
			t.Fatalf("%s: missing or duplicated common settings (%d)", path, w.Code)
		}
		if strings.Contains(page, `id="sharing"`) {
			t.Fatalf("%s: obsolete Explorer-only entry", path)
		}
	}
	for _, route := range []string{"sharing-status", "sharing-save", "sharing-folders"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", "http://local/api/"+route, strings.NewReader(`{}`)))
		if w.Code != 403 {
			t.Fatalf("unprivileged %s: %d", route, w.Code)
		}
	}
	call := func(route, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://local/api/"+route, strings.NewReader(body))
		r.Header.Set("X-Toolbox-Token", s.token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, route := range []string{"sharing-status", "sharing-folders"} {
		if w := call(route, `{}`); w.Code != 200 {
			t.Fatalf("owner %s: %d %s", route, w.Code, w.Body.String())
		}
	}
	w := call("sharing-save", `{"Enabled":false,"Port":18788,"Roots":[]}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"Port":18788`) {
		t.Fatal(w.Code, w.Body.String())
	}
	reloaded, err := newFileBrowser(s.dataDir, s.token)
	if err != nil || reloaded.config.Port != 18788 {
		t.Fatal("common settings did not persist", err)
	}
}

func TestRemoteSharedSettingsRequireAdministrator(t *testing.T) {
	_, b, _ := workspaceFixture(t)
	for _, route := range []string{"sharing-status", "sharing-save", "sharing-folders"} {
		if w := workspaceCall(b, nil, "POST", "/api/"+route, map[string]any{}, nil); w.Code == 200 {
			t.Fatal("guest reached common configuration", route)
		}
	}
	c := workspaceSignIn(t, b)
	w := c.post("/api/sharing-status", map[string]any{})
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"WorkspaceEnabled":true`) {
		t.Fatal(w.Code, w.Body.String())
	}
}
