package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

// A real child executable verifies argument delivery without depending on a GUI app.
func init() {
	if output := os.Getenv("HAMSTER_OPEN_WITH_PROBE_OUTPUT"); output != "" {
		data, err := os.ReadFile(os.Args[1])
		if err != nil {
			os.Exit(2)
		}
		proof, _ := json.Marshal(map[string]any{"args": os.Args[1:], "data": string(data)})
		if os.WriteFile(output, proof, 0600) != nil {
			os.Exit(3)
		}
		os.Exit(0)
	}
}

func TestOpenWithLaunchesSelectedProgram(t *testing.T) {
	b, file, version := openWithFixture(t)
	program, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "launch-proof.json")
	t.Setenv("HAMSTER_OPEN_WITH_PROBE_OUTPUT", output)
	w := explorerSettingsRequest(b, "open-with", map[string]string{"Path": file, "Version": version, "Program": program}, true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var data []byte
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		data, err = os.ReadFile(output)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	var proof struct {
		Args []string `json:"args"`
		Data string   `json:"data"`
	}
	if err != nil || json.Unmarshal(data, &proof) != nil || len(proof.Args) != 1 || proof.Data != "Synthetic editor sample" {
		t.Fatal("native process did not receive the original file", err, string(data))
	}
	actual, _ := os.Stat(proof.Args[0])
	want, _ := os.Stat(file)
	if actual == nil || want == nil || !os.SameFile(actual, want) {
		t.Fatal("wrong file passed to program", proof.Args)
	}
}

func openWithFixture(t *testing.T) (*fileBrowser, string, string) {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "sample & spaces, 中文 ' 01.txt")
	if err := os.WriteFile(file, []byte("Synthetic editor sample"), 0600); err != nil {
		t.Fatal(err)
	}
	b, _ := newFileBrowser(t.TempDir(), "owner")
	w := explorerSettingsRequest(b, "list", map[string]any{"Path": dir}, true)
	var listing struct{ Entries []browserEntry }
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &listing) != nil || len(listing.Entries) != 1 || listing.Entries[0].Version == "" {
		t.Fatal(w.Code, w.Body.String())
	}
	return b, file, listing.Entries[0].Version
}

func TestOpenWithOwnerValidationAndCancellation(t *testing.T) {
	b, file, version := openWithFixture(t)
	calls := 0
	b.openFileWith = func(ctx context.Context, path, program string) error {
		calls++
		a, _ := os.Stat(path)
		want, _ := os.Stat(file)
		if a == nil || want == nil || !os.SameFile(a, want) || program != "" {
			t.Errorf("launcher arguments %q %q", path, program)
		}
		return nil
	}
	body := map[string]string{"Path": file, "Version": version}
	if w := explorerSettingsRequest(b, "open-with", body, true); w.Code != 200 || calls != 1 {
		t.Fatal(w.Code, w.Body.String(), calls)
	}
	if w := explorerSettingsRequest(b, "open-with", body, false); w.Code != 404 || calls != 1 {
		t.Fatal("guest launched a host application", w.Code, calls)
	}
	for _, bad := range []map[string]string{{"Path": file}, {"Path": file, "Version": "forged"}, {"Path": ".", "Version": version}, {"Path": "https://example.com/sample.txt", "Version": version}, {"Path": file + "\n", "Version": version}, {"Path": filepath.Dir(file), "Version": version}, {"Path": file, "Version": version, "Args": "--execute"}, {"Path": file, "Version": version, "Program": "https://example.com/app"}} {
		if w := explorerSettingsRequest(b, "open-with", bad, true); w.Code != 400 || calls != 1 {
			t.Fatal("invalid launch accepted", bad, w.Code, w.Body.String(), calls)
		}
	}
	b.openFileWith = func(context.Context, string, string) error { return errOpenWithCancelled }
	if w := explorerSettingsRequest(b, "open-with", body, true); w.Code != 200 || !strings.Contains(w.Body.String(), `"Cancelled":true`) {
		t.Fatal("cancellation is not a launch error", w.Code, w.Body.String())
	}
	b.openFileWith = func(context.Context, string, string) error { return errors.New("desktop unavailable") }
	if w := explorerSettingsRequest(b, "open-with", body, true); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if err := os.WriteFile(file, []byte("Changed after directory preview"), 0600); err != nil {
		t.Fatal(err)
	}
	b.openFileWith = func(context.Context, string, string) error { t.Fatal("changed file reached launcher"); return nil }
	if w := explorerSettingsRequest(b, "open-with", body, true); w.Code != 400 {
		t.Fatal("changed file accepted", w.Code)
	}
}

func TestOpenWithExpiredReferencesAndConcurrency(t *testing.T) {
	b, file, version := openWithFixture(t)
	body := map[string]string{"Path": file, "Version": version}
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan *httptest.ResponseRecorder, 1)
	b.openFileWith = func(context.Context, string, string) error { close(entered); <-release; return errOpenWithCancelled }
	go func() { done <- explorerSettingsRequest(b, "open-with", body, true) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("launcher did not start")
	}
	w := explorerSettingsRequest(b, "open-with", body, true)
	close(release)
	first := <-done
	if w.Code != 400 || first.Code != 200 {
		t.Fatal("native chooser concurrency guard failed", w.Code, first.Code)
	}
	b.mu.Lock()
	ref := b.references[version]
	ref.At = time.Now().Add(-16 * time.Minute)
	b.references[version] = ref
	b.mu.Unlock()
	b.openFileWith = func(context.Context, string, string) error { t.Fatal("expired item launched"); return nil }
	if w := explorerSettingsRequest(b, "open-with", body, true); w.Code != 400 {
		t.Fatal(w.Code)
	}
}

func TestOpenWithProgramArgumentsAndSpecialPaths(t *testing.T) {
	b, file, version := openWithFixture(t)
	program := filepath.Join(t.TempDir(), "Viewer & Editor.exe")
	if runtime.GOOS != "windows" {
		program = filepath.Join(filepath.Dir(program), "viewer-and-editor")
	}
	if err := os.WriteFile(program, []byte("synthetic program; never executed"), 0700); err != nil {
		t.Fatal(err)
	}
	b.openFileWith = func(_ context.Context, path, app string) error {
		a, _ := os.Stat(path)
		want, _ := os.Stat(file)
		if a == nil || want == nil || !os.SameFile(a, want) || app != program {
			t.Errorf("paths changed: %q %q", path, app)
		}
		return nil
	}
	if w := explorerSettingsRequest(b, "open-with", map[string]string{"Path": file, "Version": version, "Program": program}, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := explorerSettingsRequest(b, "open-with", map[string]string{"Path": file, "Version": version, "Program": "\"" + program + "\""}, true); w.Code != 200 {
		t.Fatal("quoted shortcut path", w.Code, w.Body.String())
	}
	for _, platform := range []string{"windows", "linux"} {
		cmd := externalFileCommand(platform, program, file)
		if !reflect.DeepEqual(cmd.Args, []string{program, file}) {
			t.Fatal("file became command arguments", cmd.Args)
		}
	}
	app := "/Applications/Example Editor.app"
	cmd := externalFileCommand("darwin", app, "/tmp/photo & name; test.txt")
	if !reflect.DeepEqual(cmd.Args, []string{"/usr/bin/open", "-a", app, "--", "/tmp/photo & name; test.txt"}) {
		t.Fatal(cmd.Args)
	}
	for _, bad := range []string{"", "relative.exe", program + " --flag", program + "\x00", "file:///tmp/app"} {
		if validateExternalProgram(bad) == nil {
			t.Fatal("invalid program accepted", bad)
		}
	}
}

func TestOpenWithRejectsLinksAndCrossSiteRequests(t *testing.T) {
	b, file, version := openWithFixture(t)
	b.openFileWith = func(context.Context, string, string) error { t.Fatal("unsafe request reached launcher"); return nil }
	link := filepath.Join(filepath.Dir(file), "alias.txt")
	if err := os.Symlink(file, link); err == nil {
		if w := explorerSettingsRequest(b, "open-with", map[string]string{"Path": link, "Version": version}, true); w.Code != 400 {
			t.Fatal("link accepted", w.Code)
		}
	}
	r := httptest.NewRequest("POST", "/files/api/open-with", strings.NewReader(`{}`))
	r.Header.Set("X-Hamster-Browser", "1")
	r.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	b.dispatch(w, r, true, "api/open-with")
	if w.Code != 403 {
		t.Fatal("cross-site native launch accepted", w.Code)
	}
}
