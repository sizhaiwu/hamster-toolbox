package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestScheduleLibraryCatalogAndBuild(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "资源's [files] $(ignored) &")
	out := filepath.Join(dir, "output")
	os.Mkdir(root, 0700)
	os.Mkdir(out, 0700)
	file := filepath.Join(root, "hello.txt")
	os.WriteFile(file, []byte("hello"), 0600)
	seen := map[string]bool{}
	for _, spec := range scheduleLibraryCatalog() {
		if seen[spec.ID] || spec.Name == "" || spec.Description == "" || spec.Effect == "" {
			t.Fatal("invalid catalog", spec)
		}
		seen[spec.ID] = true
		params := map[string]string{}
		for _, f := range spec.Fields {
			switch f.Key {
			case "root":
				params[f.Key] = root
			case "output":
				params[f.Key] = out
			case "file":
				params[f.Key] = file
			case "url":
				params[f.Key] = "https://example.invalid/api"
			default:
				params[f.Key] = f.Default
			}
		}
		j, e := buildLibraryJob(spec.ID, params, runtime.GOOS, filepath.Join(dir, "hamster-toolbox.exe"))
		if e != nil {
			t.Fatal(spec.ID, e)
		}
		if j.Enabled || j.ID != "" {
			t.Fatal("template executed/created")
		}
		if e = validateScheduledJob(&j); e != nil {
			t.Fatal(spec.ID, e)
		}
		if spec.Category == "工具箱自动化" {
			if strings.Contains(j.Script, "--execute") {
				t.Fatal("default executes")
			}
			params["mode"] = "execute"
			run, e := buildLibraryJob(spec.ID, params, runtime.GOOS, "hamster-toolbox")
			if e != nil || !strings.Contains(run.Script, "--execute") {
				t.Fatal("execute opt-in", spec.ID, e)
			}
		}
	}
	if len(seen) < 12 {
		t.Fatal("catalog incomplete")
	}
	for _, test := range []struct {
		id string
		p  map[string]string
	}{{"missing", nil}, {"inventory", map[string]string{"root": "relative"}}, {"backup", map[string]string{"root": root, "output": root}}, {"large", map[string]string{"root": root, "minMB": "1; touch x"}}, {"old", map[string]string{"root": root, "days": "0"}}, {"post", map[string]string{"url": "https://example.invalid", "body": "invalid"}}, {"system", map[string]string{"unknown": "x"}}} {
		if _, e := buildLibraryJob(test.id, test.p, runtime.GOOS, "toolbox"); e == nil {
			t.Fatal("invalid params accepted", test.id)
		}
	}
}
func TestScheduleLibraryScriptsRunWithLiteralPaths(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "资料's [files] $(oops) &")
	out := filepath.Join(base, "backups")
	os.Mkdir(root, 0700)
	os.Mkdir(out, 0700)
	data := []byte("library test content")
	file := filepath.Join(root, "test.txt")
	os.WriteFile(file, data, 0600)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, id := range []string{"inventory", "hash", "backup"} {
		params := map[string]string{}
		if id == "hash" {
			params["file"] = file
		} else {
			params["root"] = root
		}
		if id == "backup" {
			params["output"] = out
		}
		j, e := buildLibraryJob(id, params, runtime.GOOS, "toolbox")
		if e != nil {
			t.Fatal(e)
		}
		output, e := executeScheduledJob(ctx, j, t.TempDir())
		if e != nil {
			t.Fatal(id, output, e)
		}
		if id == "inventory" && !strings.Contains(output, "test.txt") {
			t.Fatal(output)
		}
		if id == "hash" {
			sum := sha256.Sum256(data)
			if !strings.Contains(strings.ToLower(output), hex.EncodeToString(sum[:])) {
				t.Fatal("hash incomplete", output)
			}
		}
	}
	found := false
	filepath.WalkDir(out, func(path string, d os.DirEntry, e error) error {
		if e == nil && !d.IsDir() {
			b, _ := os.ReadFile(path)
			if string(b) == string(data) {
				found = true
			}
		}
		return e
	})
	if !found {
		t.Fatal("backup missing")
	}
	b, _ := os.ReadFile(file)
	if string(b) != string(data) {
		t.Fatal("source changed")
	}
}
func TestScheduleLibraryReadOnlyAPI(t *testing.T) {
	m := newScheduleManager(t.TempDir())
	defer m.stop()
	s := &server{scheduler: m}
	w := scheduleRequest(t, s, map[string]string{"Action": "library"})
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Templates") {
		t.Fatal(w.Body.String())
	}
	w = scheduleRequest(t, s, map[string]any{"Action": "template-build", "TemplateID": "system", "Params": map[string]string{}})
	var result struct{ Job scheduledJob }
	if json.Unmarshal(w.Body.Bytes(), &result) != nil || w.Code != 200 || result.Job.Enabled || result.Job.Script == "" {
		t.Fatal(w.Body.String())
	}
	if len(m.store.Jobs) != 0 || len(m.store.Runs) != 0 {
		t.Fatal("template saved a job")
	}
	if _, e := os.Stat(filepath.Join(m.dir, "schedules.json")); !os.IsNotExist(e) {
		t.Fatal("template wrote data")
	}
}
