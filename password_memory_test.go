package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPasswordMemory(t *testing.T) {
	s := &server{token: "test-session", dataDir: t.TempDir()}
	read := func(tool string) struct {
		Supported bool
		rememberedPassword
	} {
		t.Helper()
		w := requestAPI(t, s, "/api/password-memory", map[string]string{"Tool": tool})
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		var result struct {
			Supported bool
			rememberedPassword
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	if runtime.GOOS != "windows" {
		if read("archive").Supported {
			t.Fatal("unsupported platform claimed encrypted memory")
		}
		return
	}
	if !read("archive").Remember {
		t.Fatal("memory should start enabled")
	}
	write := func(tool, password, id string, remember bool) {
		t.Helper()
		w := requestAPI(t, s, "/api/password-memory-save", map[string]any{"Tool": tool, "Password": password, "SavedID": id, "Remember": remember})
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	write("archive", "Archive-Only-Secret!", "", true)
	write("extract", "Extract-Only-Secret!", "", true)
	b, err := os.ReadFile(filepath.Join(s.dataDir, "password-memory.dat"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("Archive-Only-Secret!")) || bytes.Contains(b, []byte("Extract-Only-Secret!")) {
		t.Fatal("password memory stored plaintext")
	}
	s = &server{token: "test-session", dataDir: s.dataDir}
	if read("archive").Password != "Archive-Only-Secret!" || read("extract").Password != "Extract-Only-Secret!" {
		t.Fatal("restart lost or mixed per-tool passwords")
	}
	write("archive", "ignored-on-opt-out", "", false)
	if got := read("archive"); got.Remember || got.Password != "" {
		t.Fatal("opt out retained secret")
	}
	if read("extract").Password != "Extract-Only-Secret!" {
		t.Fatal("opting out cleared other tool")
	}
	write("extract", "", "", true)
	if read("extract").Password != "" {
		t.Fatal("cleared input restored old password")
	}
	if w := requestAPI(t, s, "/api/password-save", savedPassword{Name: "示例", Password: "Saved-Secret"}); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	vault, err := s.loadPasswords()
	if err != nil || len(vault) != 1 {
		t.Fatal(vault, err)
	}
	id := vault[0].ID
	write("archive", "Saved-Secret", id, true)
	if w := requestAPI(t, s, "/api/password-save", savedPassword{ID: id, Name: "示例", Password: "Edited-Secret"}); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if got := read("archive"); got.SavedID != id || got.Password != "Edited-Secret" {
		t.Fatal("remembered choice did not follow edited saved password")
	}
	if w := requestAPI(t, s, "/api/password-delete", savedPassword{ID: id}); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if got := read("archive"); got.Password != "" || got.SavedID != "" {
		t.Fatal("deleted saved password still restored")
	}
	if w := requestAPI(t, s, "/api/password-memory-save", map[string]any{"Tool": "video", "Remember": true, "Password": "secret"}); w.Code == 200 {
		t.Fatal("unrelated tool accepted memory")
	}
	damaged := []byte("HTPM1-damaged")
	if err := os.WriteFile(filepath.Join(s.dataDir, "password-memory.dat"), damaged, 0600); err != nil {
		t.Fatal(err)
	}
	if w := requestAPI(t, s, "/api/password-memory-save", map[string]any{"Tool": "archive", "Remember": true, "Password": "new"}); w.Code == 200 {
		t.Fatal("damaged password memory overwritten")
	}
	if current, _ := os.ReadFile(filepath.Join(s.dataDir, "password-memory.dat")); !bytes.Equal(current, damaged) {
		t.Fatal("damaged memory changed")
	}
}

func TestPasswordMemoryConcurrentTools(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows account encryption")
	}
	s := &server{token: "test-session", dataDir: t.TempDir()}
	results := make(chan int, 2)
	for _, tool := range []string{"archive", "extract"} {
		go func(tool string) {
			results <- requestAPI(t, s, "/api/password-memory-save", map[string]any{"Tool": tool, "Remember": true, "Password": "synthetic-" + tool}).Code
		}(tool)
	}
	for range 2 {
		if code := <-results; code != 200 {
			t.Fatal("concurrent memory save failed", code)
		}
	}
	values, err := s.loadPasswordMemory()
	if err != nil || values["archive"].Password != "synthetic-archive" || values["extract"].Password != "synthetic-extract" {
		t.Fatal("one tool overwrote the other tool's remembered password", err)
	}
}
