package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopDropReferencesAndTransfer(t *testing.T) {
	b, e := newFileBrowser(t.TempDir(), "owner")
	if e != nil {
		t.Fatal(e)
	}
	s := &server{browser: b}
	source := filepath.Join(t.TempDir(), "example.txt")
	if e = os.WriteFile(source, []byte("synthetic example"), 0600); e != nil {
		t.Fatal(e)
	}
	body, _ := json.Marshal(map[string]any{"Action": "drop-items", "Paths": []string{source}})
	w := clientRequest(s, string(body))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var items []struct {
		Path, Name, Version string
		Dir                 bool
	}
	if e = json.Unmarshal(w.Body.Bytes(), &items); e != nil || len(items) != 1 || items[0].Version == "" || items[0].Path != source || items[0].Name != "example.txt" || items[0].Dir {
		t.Fatal(e, w.Body.String())
	}
	destination := filepath.Join(t.TempDir(), "copied.txt")
	transfer, _ := json.Marshal(map[string]string{"Action": "copy", "Path": source, "Target": destination, "Version": items[0].Version})
	r := httptest.NewRequest("POST", "http://localhost/files/api/transfer", bytes.NewReader(transfer))
	out := httptest.NewRecorder()
	b.transfer(out, r, true)
	if out.Code != 200 || !strings.Contains(out.Body.String(), `"Finished":true`) {
		t.Fatal(out.Code, out.Body.String())
	}
	if copied, e := os.ReadFile(destination); e != nil || string(copied) != "synthetic example" {
		t.Fatal(e, string(copied))
	}
	if _, e = os.Stat(source); e != nil {
		t.Fatal("copy removed source", e)
	}
	// The same reference cannot authorize a different file or a changed source.
	if e = os.WriteFile(source, []byte("changed after selection"), 0600); e != nil {
		t.Fatal(e)
	}
	out = httptest.NewRecorder()
	b.transfer(out, httptest.NewRequest("POST", "http://localhost/files/api/transfer", bytes.NewReader(transfer)), true)
	if out.Code == 200 {
		t.Fatal("changed source accepted", out.Body.String())
	}
}

func TestDesktopDropValidationAndAssets(t *testing.T) {
	b, e := newFileBrowser(t.TempDir(), "owner")
	if e != nil {
		t.Fatal(e)
	}
	s := &server{browser: b, token: "owner"}
	for _, body := range []string{`{"Action":"drop-items","Paths":[]}`, `{"Action":"drop-items","Paths":["relative.txt"]}`, `{"Action":"drop-items","Paths":["\u0000"]}`} {
		if w := clientRequest(s, body); w.Code == 200 {
			t.Fatal("invalid drop accepted", body)
		}
	}
	r := httptest.NewRequest("POST", "http://localhost/api/desktop-client", strings.NewReader(`{"Action":"drop-items","Paths":[]}`))
	r.RemoteAddr = "192.0.2.1:1234"
	w := httptest.NewRecorder()
	s.desktopClientAPI(w, r)
	if w.Code != 403 {
		t.Fatal("remote desktop access", w.Code)
	}
	for _, asset := range []string{"desktop-experience.js", "desktop-experience.css"} {
		if !browserStaticAsset(asset) {
			t.Fatal("missing explorer asset", asset)
		}
		w := httptest.NewRecorder()
		s.httpHandler().ServeHTTP(w, httptest.NewRequest("GET", "http://localhost/"+asset, nil))
		if w.Code != 200 {
			t.Fatal(asset, w.Code)
		}
	}
}
