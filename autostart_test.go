package main

import (
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type memoryAutostartStore struct {
	value                string
	writes, deletes      int
	readError, saveError error
}

func (s *memoryAutostartStore) Read() (string, error) { return s.value, s.readError }
func (s *memoryAutostartStore) Write(v string) error {
	if s.saveError != nil {
		return s.saveError
	}
	s.value = v
	s.writes++
	return nil
}
func (s *memoryAutostartStore) Delete() error {
	if s.saveError != nil {
		return s.saveError
	}
	s.value = ""
	s.deletes++
	return nil
}
func testAutostartManager(t *testing.T) (*autostartManager, *memoryAutostartStore) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "hamster-toolbox-desktop.exe")
	if err := os.WriteFile(p, []byte("synthetic executable"), 0600); err != nil {
		t.Fatal(err)
	}
	store := &memoryAutostartStore{}
	return &autostartManager{program: p, command: `"` + p + `" -tray -no-open`, store: store}, store
}
func TestAutostartExplicitChoiceAndOwnership(t *testing.T) {
	m, store := testAutostartManager(t)
	status, err := m.read()
	if err != nil || status.Enabled || !status.Supported || !status.CanEnable || store.writes != 0 {
		t.Fatal(status, err, store)
	}
	if status, err = m.set(true); err != nil || !status.Enabled || store.writes != 1 {
		t.Fatal(status, err)
	}
	if status, err = m.set(false); err != nil || status.Enabled || store.deletes != 1 {
		t.Fatal(status, err)
	}
	store.value = `"E:\another installation\hamster-toolbox-desktop.exe" -tray -no-open`
	foreign := store.value
	if status, err = m.read(); err != nil || !status.OtherLocation || status.Enabled {
		t.Fatal(status, err)
	}
	if _, err = m.set(false); err != nil || store.value != foreign || store.deletes != 1 {
		t.Fatal("foreign startup removed", err)
	}
	if status, err = m.set(true); err != nil || !status.Enabled || status.OtherLocation {
		t.Fatal(status, err)
	}
	store.saveError = errors.New("access denied")
	if _, err = m.set(false); err == nil || store.value != m.command {
		t.Fatal("save failure lost enabled state")
	}
	store.readError = errors.New("cannot read registry")
	if _, err = m.set(true); err == nil || store.writes != 2 {
		t.Fatal("read error ignored")
	}
}
func TestAutostartMissingProgramAndUnsupported(t *testing.T) {
	m, store := testAutostartManager(t)
	if err := os.Remove(m.program); err != nil {
		t.Fatal(err)
	}
	if _, err := m.set(true); err == nil || store.writes != 0 {
		t.Fatal("enabled missing executable")
	}
	// A missing executable must not prevent removal of this exact registration.
	store.value = m.command
	if status, err := m.set(false); err != nil || status.Enabled || store.deletes != 1 {
		t.Fatal(status, err)
	}
	m.store = nil
	if status, err := m.read(); err != nil || status.Supported {
		t.Fatal(status, err)
	}
	if _, err := m.set(true); err == nil {
		t.Fatal("enabled unsupported platform")
	}
}
func TestAutostartAPIRequiresSessionAndExplicitBoolean(t *testing.T) {
	m, store := testAutostartManager(t)
	s := &server{token: "startup-test", dataDir: t.TempDir(), autostart: m}
	for _, body := range []string{`{}`, `{"Enabled":null}`} {
		r := httptest.NewRequest("POST", "/api/autostart", strings.NewReader(body))
		r.Header.Set("X-Toolbox-Token", s.token)
		w := httptest.NewRecorder()
		s.guard(s.api)(w, r)
		if w.Code != 200 || store.writes != 0 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	for _, body := range []string{`{"Enabled":"true"}`, `{"Unexpected":true}`, `{} {"Enabled":true}`, `{`} {
		r := httptest.NewRequest("POST", "/api/autostart", strings.NewReader(body))
		r.Header.Set("X-Toolbox-Token", s.token)
		w := httptest.NewRecorder()
		s.guard(s.api)(w, r)
		if w.Code != 400 || store.writes != 0 {
			t.Fatal("malformed body accepted", w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest("POST", "/api/autostart", strings.NewReader(`{"Enabled":true}`))
	w := httptest.NewRecorder()
	s.guard(s.api)(w, r)
	if w.Code != 403 || store.writes != 0 {
		t.Fatal("anonymous mutation accepted")
	}
	if w = requestAPI(t, s, "/api/autostart", map[string]bool{"Enabled": true}); w.Code != 200 || store.writes != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(s.dataDir, "settings.json")); !os.IsNotExist(err) {
		t.Fatal("startup leaked into persisted settings")
	}
}
