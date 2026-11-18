package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPasswordVaultAPIAndEncryption(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows DPAPI")
	}
	s := &server{token: "test-session", dataDir: t.TempDir()}
	save := requestAPI(t, s, "/api/password-save", savedPassword{Name: "演示密码", Password: "Secret-Test-Only!"})
	if save.Code != 200 {
		t.Fatal(save.Body.String())
	}
	b, e := os.ReadFile(filepath.Join(s.dataDir, "passwords.dat"))
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(b, []byte("Secret-Test-Only!")) || bytes.Contains(b, []byte("演示密码")) {
		t.Fatal("vault stored plaintext")
	}
	listing := requestAPI(t, s, "/api/passwords", map[string]string{})
	if bytes.Contains(listing.Body.Bytes(), []byte("Secret-Test-Only!")) {
		t.Fatal("listing leaks secret")
	}
	var choices []passwordChoice
	if e = json.Unmarshal(listing.Body.Bytes(), &choices); e != nil || len(choices) != 1 {
		t.Fatal(listing.Body.String())
	}
	id := choices[0].ID
	restarted := &server{token: "test-session", dataDir: s.dataDir}
	got := requestAPI(t, restarted, "/api/password-get", savedPassword{ID: id})
	var value map[string]string
	if e = json.Unmarshal(got.Body.Bytes(), &value); e != nil || value["Password"] != "Secret-Test-Only!" {
		t.Fatal("password did not survive restart")
	}
	save = requestAPI(t, s, "/api/password-save", savedPassword{ID: id, Name: "新名称", Password: "Edited-Secret!"})
	if save.Code != 200 {
		t.Fatal(save.Body.String())
	}
	if w := requestAPI(t, s, "/api/password-save", savedPassword{Name: "新名称", Password: "other"}); w.Code == 200 {
		t.Fatal("duplicate name accepted")
	}
	if w := requestAPI(t, s, "/api/password-delete", savedPassword{ID: id}); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	listing = requestAPI(t, s, "/api/passwords", map[string]string{})
	if listing.Body.String() != "[]\n" {
		t.Fatal(listing.Body.String())
	}
	// Corrupt vault data must not be silently replaced by an empty list.
	damaged := []byte("HTPW1-damaged")
	if e = os.WriteFile(filepath.Join(s.dataDir, "passwords.dat"), damaged, 0600); e != nil {
		t.Fatal(e)
	}
	if w := requestAPI(t, s, "/api/password-save", savedPassword{Name: "new", Password: "test"}); w.Code == 200 {
		t.Fatal("corrupt vault overwritten")
	}
	b, _ = os.ReadFile(filepath.Join(s.dataDir, "passwords.dat"))
	if !bytes.Equal(b, damaged) {
		t.Fatal("corrupt vault changed")
	}
}
