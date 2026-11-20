package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func explorerSettingsRequest(b *fileBrowser, route string, body any, owner bool) *httptest.ResponseRecorder {
	data, _ := json.Marshal(body)
	r := httptest.NewRequest("POST", "/files/api/"+route, bytes.NewReader(data))
	r.Header.Set("X-Hamster-Browser", "1")
	w := httptest.NewRecorder()
	if owner {
		r.AddCookie(&http.Cookie{Name: "hamster_files_" + b.cookieTag, Value: b.localCookie})
		b.local(w, r)
	} else {
		b.dispatch(w, r, false, "api/"+route)
	}
	return w
}

func TestExplorerGridGapPersistenceAndCompatibility(t *testing.T) {
	dir := t.TempDir()
	b, _ := newFileBrowser(dir, "owner")
	// Existing version-1 settings without a gap remain valid.
	if w := explorerSettingsRequest(b, "preferences", map[string]any{"tileSize": 220}, true); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	for _, gap := range []int{0, 6, 16, 32} {
		w := explorerSettingsRequest(b, "preferences", map[string]any{"tileGap": gap}, true)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		restarted, err := newFileBrowser(dir, "new-owner")
		if err != nil {
			t.Fatal(err)
		}
		p, err := restarted.loadExplorerPreferences()
		if err != nil || p.TileGap == nil || *p.TileGap != gap || p.TileSize != 220 {
			t.Fatal("gap or independent size lost", p, err)
		}
	}
	before, err := os.ReadFile(filepath.Join(dir, "explorer.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, gap := range []int{-1, 33} {
		if w := explorerSettingsRequest(b, "preferences", map[string]any{"tileGap": gap}, true); w.Code != 400 {
			t.Fatal("invalid gap accepted", w.Code)
		}
	}
	after, err := os.ReadFile(filepath.Join(dir, "explorer.json"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("invalid gap changed stored settings", err)
	}
}

func TestExplorerPreferencesSurviveRestartAndPartialUpdates(t *testing.T) {
	data := t.TempDir()
	b, _ := newFileBrowser(data, "first-owner-token")
	if p, err := b.loadExplorerPreferences(); p != nil || err != nil {
		t.Fatal("new installations must permit legacy migration", p, err)
	}
	// A currently missing disk/directory must still be saved as a favorite.
	favorite := filepath.Join(t.TempDir(), "offline-library")
	w := explorerSettingsRequest(b, "preferences", map[string]any{"favorites": []string{favorite}, "view": "list", "tileSize": 320}, true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	restarted, _ := newFileBrowser(data, "new-token-and-port")
	w = explorerSettingsRequest(restarted, "boot", nil, true)
	var boot struct{ Preferences *explorerPreferences }
	if err := json.Unmarshal(w.Body.Bytes(), &boot); err != nil || boot.Preferences == nil {
		t.Fatal("settings missing after restart", err, w.Body.String())
	}
	p := boot.Preferences
	if p.View != "list" || p.TileSize != 320 || len(p.Favorites) != 1 || p.Favorites[0] != favorite {
		t.Fatal("restart lost preferences", p)
	}
	// Independent browser tabs can update different fields without clearing others.
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w := explorerSettingsRequest(restarted, "preferences", map[string]any{"tileSize": 200 + i}, true)
			if w.Code != 200 {
				t.Errorf("concurrent update: %d %s", w.Code, w.Body.String())
			}
		}(i)
	}
	wg.Wait()
	p, err := restarted.loadExplorerPreferences()
	if err != nil || p.View != "list" || len(p.Favorites) != 1 || p.Favorites[0] != favorite {
		t.Fatal("partial update cleared other fields", p, err)
	}
	w = explorerSettingsRequest(restarted, "preferences", map[string]any{"favorites": []string{}}, true)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	third, _ := newFileBrowser(data, "third-owner")
	p, err = third.loadExplorerPreferences()
	if err != nil || p == nil || len(p.Favorites) != 0 {
		t.Fatal("intentionally empty favorites must remain saved", p, err)
	}
	isolated, _ := newFileBrowser(t.TempDir(), "other-data-directory")
	if p, err := isolated.loadExplorerPreferences(); p != nil || err != nil {
		t.Fatal("different data directories share preferences", p, err)
	}
}

func TestExplorerHoverPreferenceSurvivesRestart(t *testing.T) {
	data := t.TempDir()
	b, _ := newFileBrowser(data, "owner")
	for _, enabled := range []bool{false, true} {
		w := explorerSettingsRequest(b, "preferences", map[string]any{"hoverPreview": enabled}, true)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		w = explorerSettingsRequest(b, "preferences", map[string]any{"view": "list"}, true)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		restarted, _ := newFileBrowser(data, "new-owner")
		p, err := restarted.loadExplorerPreferences()
		if err != nil || p.HoverPreview == nil || *p.HoverPreview != enabled {
			t.Fatal("hover preference lost", p, err)
		}
	}
}

func TestExplorerPreferencesGuestIsolationAndValidation(t *testing.T) {
	b, _ := newFileBrowser(t.TempDir(), "private-owner")
	favorite := filepath.Join(t.TempDir(), "private-favorite")
	if w := explorerSettingsRequest(b, "preferences", map[string]any{"favorites": []string{favorite, favorite}}, true); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	p, _ := b.loadExplorerPreferences()
	if len(p.Favorites) != 1 {
		t.Fatal("duplicate favorites not removed")
	}
	guest := explorerSettingsRequest(b, "boot", nil, false)
	if strings.Contains(guest.Body.String(), "Preferences") || strings.Contains(guest.Body.String(), favorite) {
		t.Fatal("guest received private owner settings", guest.Body.String())
	}
	if w := explorerSettingsRequest(b, "preferences", map[string]any{"favorites": []string{}}, false); w.Code != 404 {
		t.Fatal("guest can modify owner preferences", w.Code)
	}
	r := httptest.NewRequest("POST", "/files/api/preferences", strings.NewReader(`{"favorites":[]}`))
	r.Header.Set("X-Hamster-Browser", "1")
	w := httptest.NewRecorder()
	b.local(w, r)
	if w.Code != 403 {
		t.Fatal("unauthenticated owner endpoint accepted", w.Code)
	}
	for _, body := range []any{
		map[string]any{"view": "invalid"}, map[string]any{"tileSize": 421},
		map[string]any{"favorites": []string{"relative-directory"}},
		map[string]any{"favorites": []string{favorite + "\x00"}},
		map[string]any{"favorites": make([]string, 21)}, map[string]any{"unknown": true},
	} {
		if w := explorerSettingsRequest(b, "preferences", body, true); w.Code != 400 {
			t.Fatal("invalid settings accepted", body, w.Code)
		}
	}
	p, _ = b.loadExplorerPreferences()
	if len(p.Favorites) != 1 || p.Favorites[0] != favorite {
		t.Fatal("failed updates destroyed saved favorites", p)
	}
}

func TestExplorerPreferencesKeepDamagedFileAndReportSaveFailure(t *testing.T) {
	data := t.TempDir()
	b, _ := newFileBrowser(data, "owner")
	file := filepath.Join(data, "explorer.json")
	damaged := []byte("{broken-settings")
	if err := os.WriteFile(file, damaged, 0600); err != nil {
		t.Fatal(err)
	}
	boot := explorerSettingsRequest(b, "boot", nil, true)
	if boot.Code != 200 || !strings.Contains(boot.Body.String(), "PreferencesError") || !strings.Contains(boot.Body.String(), "损坏") {
		t.Fatal("damaged settings must not prevent browsing or hide the error", boot.Body.String())
	}
	if w := explorerSettingsRequest(b, "preferences", map[string]any{"favorites": []string{}}, true); w.Code != 400 {
		t.Fatal("damaged original was overwritten", w.Code)
	}
	got, _ := os.ReadFile(file)
	if !bytes.Equal(got, damaged) {
		t.Fatal("original damaged file was changed")
	}
	// A regular file in place of the data directory reliably simulates save failure.
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	b.dataDir = blocked
	if w := explorerSettingsRequest(b, "preferences", map[string]any{"view": "list"}, true); w.Code != 400 {
		t.Fatal("unwritable settings reported success", w.Code)
	}
}
