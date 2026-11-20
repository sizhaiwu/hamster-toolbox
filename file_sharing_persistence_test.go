package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loopbackSharing(t *testing.T, b *fileBrowser) *string {
	t.Helper()
	address := new(string)
	b.listen = func(network, requested string) (net.Listener, error) {
		if network != "tcp4" || requested != "0.0.0.0:8788" {
			t.Fatal("unexpected sharing listener", network, requested)
		}
		listener, e := net.Listen("tcp4", "127.0.0.1:0")
		if e == nil {
			*address = "http://" + listener.Addr().String()
		}
		return listener, e
	}
	t.Cleanup(func() {
		b.mu.Lock()
		srv := b.server
		b.mu.Unlock()
		if srv != nil {
			srv.Close()
		}
	})
	return address
}

func saveTestSharing(t *testing.T, b *fileBrowser, enabled bool, password string, roots []sharedFolder) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"Enabled": enabled, "Port": 8788, "Password": password, "Roots": roots})
	if e := b.saveSharing(httptest.NewRequest("POST", "/", bytes.NewReader(body))); e != nil {
		t.Fatal(e)
	}
}

func TestSharingPersistsMultipleRootsAndAutostarts(t *testing.T) {
	data, first, second := t.TempDir(), t.TempDir(), t.TempDir()
	b, e := newFileBrowser(data, "test-owner")
	if e != nil {
		t.Fatal(e)
	}
	loopbackSharing(t, b)
	saveTestSharing(t, b, true, "Synthetic-Share-Password!", []sharedFolder{{Name: "Photos", Path: first}, {Name: "Videos", Path: second, Writable: true}})
	original := append([]sharedFolder{}, b.config.Roots...)
	saveTestSharing(t, b, true, "", []sharedFolder{{ID: "client-must-not-choose-id", Name: "Photos renamed", Path: first}, {Name: "Videos", Path: second, Writable: true}})
	if b.config.Roots[0].ID != original[0].ID || b.config.Roots[1].ID != original[1].ID {
		t.Fatal("saving settings changed existing root IDs")
	}
	restarted, e := newFileBrowser(data, "new-owner")
	if e != nil {
		t.Fatal(e)
	}
	address := loopbackSharing(t, restarted)
	if e := restarted.startSavedSharing(); e != nil {
		t.Fatal(e)
	}
	if restarted.status()["Enabled"] != true || restarted.status()["StartupEnabled"] != true || len(restarted.scopes) != 2 || restarted.config.Roots[0].ID != original[0].ID {
		t.Fatal("saved sharing failed to resume both roots")
	}
	if e := restarted.startSavedSharing(); e != nil {
		t.Fatal("starting an already running share must be harmless", e)
	}
	if restarted.config.Roots[0].Writable || !restarted.config.Roots[1].Writable || restarted.config.Roots[0].Name != "Photos renamed" {
		t.Fatal("saved names/permissions were lost")
	}
	request := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"Password":"Synthetic-Share-Password!"}`))
	request.RemoteAddr = "127.0.0.1:1234"
	request.Header.Set("X-Hamster-Browser", "1")
	login := httptest.NewRecorder()
	restarted.remote(login, request)
	if login.Code != 200 {
		t.Fatal("saved password no longer works", login.Body.String())
	}
	for _, root := range original {
		r, _ := http.NewRequest("POST", *address+"/api/list?root="+root.ID, strings.NewReader(`{"Path":""}`))
		r.Header.Set("X-Hamster-Browser", "1")
		r.AddCookie(login.Result().Cookies()[0])
		response, e := http.DefaultClient.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatal("cannot browse resumed root", response.StatusCode)
		}
	}
	saveTestSharing(t, restarted, false, "", restarted.config.Roots)
	off, e := newFileBrowser(data, "third-owner")
	if e != nil {
		t.Fatal(e)
	}
	off.listen = func(string, string) (net.Listener, error) {
		t.Fatal("disabled sharing tried to listen")
		return nil, nil
	}
	if e := off.startSavedSharing(); e != nil || off.status()["Enabled"] != false || off.config.Enabled || len(off.config.Roots) != 2 || off.config.Hash == "" {
		t.Fatal("disabled state must persist without forgetting roots/password", e)
	}
}

func TestSharingAutostartHandlesUnavailableRootsAndPort(t *testing.T) {
	data, first, second := t.TempDir(), t.TempDir(), t.TempDir()
	b, _ := newFileBrowser(data, "test-owner")
	loopbackSharing(t, b)
	saveTestSharing(t, b, true, "Synthetic-Share-Password!", []sharedFolder{{Name: "Available", Path: first}, {Name: "Offline", Path: second}})
	if e := os.Remove(second); e != nil {
		t.Fatal(e)
	}
	restarted, _ := newFileBrowser(data, "new-owner")
	loopbackSharing(t, restarted)
	if e := restarted.startSavedSharing(); e != nil || len(restarted.config.Roots) != 2 || len(restarted.scopes) != 1 || len(restarted.unavailable) != 1 {
		t.Fatal("offline root should stay saved while available root works", e)
	}
	bootReq := httptest.NewRequest("POST", "/api/boot", strings.NewReader(`{}`))
	bootReq.Header.Set("X-Hamster-Browser", "1")
	boot := httptest.NewRecorder()
	restarted.dispatch(boot, bootReq, false, "api/boot")
	if strings.Contains(boot.Body.String(), "Offline") || !strings.Contains(boot.Body.String(), "Available") {
		t.Fatal("guest sees unavailable root", boot.Body.String())
	}
	saveTestSharing(t, restarted, false, "", restarted.config.Roots)
	if len(restarted.config.Roots) != 2 {
		t.Fatal("saving offline settings lost the missing directory")
	}
	if e := os.Mkdir(second, 0700); e != nil {
		t.Fatal(e)
	}
	saveTestSharing(t, restarted, true, "", restarted.config.Roots)
	blocked, _ := newFileBrowser(data, "blocked-owner")
	blocked.listen = func(string, string) (net.Listener, error) { return nil, errors.New("test port busy") }
	if e := blocked.startSavedSharing(); e == nil || blocked.status()["Enabled"] != false || blocked.status()["StartupEnabled"] != true || blocked.status()["Error"] == "" {
		t.Fatal("port failure must be visible and preserve autostart preference")
	}
	loopbackSharing(t, blocked)
	if e := blocked.startSavedSharing(); e != nil || len(blocked.scopes) != 2 {
		t.Fatal("retry after port recovery failed", e)
	}
	stored, _ := os.ReadFile(filepath.Join(data, "sharing.json"))
	if bytes.Contains(stored, []byte("Synthetic-Share-Password!")) {
		t.Fatal("plaintext password persisted")
	}
}

func TestSharingOldConfigDoesNotOptIntoAutostart(t *testing.T) {
	data := t.TempDir()
	if e := os.WriteFile(filepath.Join(data, "sharing.json"), []byte(`{"Port":8788,"Roots":[]}`), 0600); e != nil {
		t.Fatal(e)
	}
	b, e := newFileBrowser(data, "owner")
	if e != nil {
		t.Fatal(e)
	}
	b.listen = func(string, string) (net.Listener, error) {
		t.Fatal("legacy settings exposed a listener")
		return nil, nil
	}
	if e := b.startSavedSharing(); e != nil || b.config.Enabled {
		t.Fatal("old settings should stay disabled until explicitly saved", e)
	}
}

func TestSharingStartupRejectsInvalidPasswordAndUnavailableRoots(t *testing.T) {
	for _, invalidPassword := range []bool{true, false} {
		t.Run(map[bool]string{true: "password", false: "missing roots"}[invalidPassword], func(t *testing.T) {
			data := t.TempDir()
			cfg := sharingConfig{Enabled: true, Port: 8788, Salt: strings.Repeat("01", 24), Hash: strings.Repeat("02", 32), Roots: []sharedFolder{{ID: randomID(), Name: "Unavailable", Path: filepath.Join(t.TempDir(), "not-present")}}}
			if invalidPassword {
				cfg.Hash = "invalid"
			}
			v, _ := json.Marshal(cfg)
			os.WriteFile(filepath.Join(data, "sharing.json"), v, 0600)
			b, e := newFileBrowser(data, "owner")
			if e != nil {
				t.Fatal(e)
			}
			b.listen = func(string, string) (net.Listener, error) {
				t.Fatal("invalid saved settings opened a port")
				return nil, nil
			}
			if e := b.startSavedSharing(); e == nil || b.status()["Error"] == "" || b.status()["Enabled"] != false || !b.config.Enabled || len(b.config.Roots) != 1 {
				t.Fatal("unsafe/unavailable startup should fail visibly while preserving saved settings")
			}
		})
	}
}
