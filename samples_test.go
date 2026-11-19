package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

type sampleAsset struct {
	Name, SHA256 string
	Bytes        int64
}

func readSampleJSON(t *testing.T, path string, result any) {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf}), result); e != nil {
		t.Fatal(path, e)
	}
}

func sampleHashes(t *testing.T) map[string]string {
	t.Helper()
	var manifest struct{ Files []sampleAsset }
	readSampleJSON(t, "samples/assets/manifest.json", &manifest)
	r := map[string]string{}
	for _, asset := range manifest.Files {
		if asset.Name != filepath.Base(asset.Name) || strings.Contains(asset.Name, "..") {
			t.Fatal("asset name must be an ordinary basename", asset.Name)
		}
		b, e := os.ReadFile(filepath.Join("samples", "assets", asset.Name))
		if e != nil {
			t.Fatal(e)
		}
		sum := sha256.Sum256(b)
		hash := hex.EncodeToString(sum[:])
		if int64(len(b)) != asset.Bytes || hash != asset.SHA256 {
			t.Fatal("sample integrity mismatch", asset.Name)
		}
		if _, exists := r[asset.Name]; exists {
			t.Fatal("duplicate sample asset", asset.Name)
		}
		r[asset.Name] = hash
	}
	return r
}

func TestSampleAssetsAndScenarioCoverage(t *testing.T) {
	assets := sampleHashes(t)
	var scenarios []struct {
		Tool, Directory, Hint, Expected string
		Files                           []struct{ Asset, Path string }
	}
	readSampleJSON(t, "samples/scenarios.json", &scenarios)
	seen := map[string]bool{}
	for _, scenario := range scenarios {
		if !toolbox.ToolExists(scenario.Tool) || seen[scenario.Tool] || scenario.Hint == "" || scenario.Expected == "" {
			t.Fatal("invalid or undocumented sample scenario", scenario.Tool)
		}
		seen[scenario.Tool] = true
		if len(scenario.Files) == 0 || scenario.Directory != filepath.Base(scenario.Directory) {
			t.Fatal("invalid scenario layout", scenario.Tool)
		}
		for _, file := range scenario.Files {
			clean := filepath.Clean(filepath.FromSlash(file.Path))
			if _, ok := assets[file.Asset]; !ok || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
				t.Fatal("unsafe or missing scenario file", file)
			}
		}
	}
	for _, tool := range []string{"format", "rename", "number", "describe", "exif", "pdf", "image", "video", "thumbnail", "archive", "extract", "dedupe"} {
		if !seen[tool] {
			t.Error("missing hands-on scenario", tool)
		}
	}
}

func TestSamplePlaygroundExecutableIntegration(t *testing.T) {
	exe := os.Getenv("HAMSTER_CLI_TEST_EXE")
	if exe == "" {
		t.Skip("set HAMSTER_CLI_TEST_EXE to verify hands-on samples with a built toolbox")
	}
	exe, e := filepath.Abs(exe)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	before := sampleHashes(t)
	var session struct {
		Workspace string
		Scenarios []struct{ Tool, Input, Config string }
	}
	prepare := func(parent string) []byte {
		t.Helper()
		cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "samples/create-playground.ps1", "-OutputParent", parent, "-NoOpen", "-Json")
		var out, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &stderr
		if e := cmd.Run(); e != nil {
			t.Fatal("prepare sample playground", e, stderr.String())
		}
		return out.Bytes()
	}
	parent := filepath.Join(t.TempDir(), "中文 ' [100%] playground")
	if e := json.Unmarshal(prepare(parent), &session); e != nil || len(session.Scenarios) != 12 {
		t.Fatal("invalid prepared sample session", e, session)
	}
	var second struct{ Workspace string }
	if e := json.Unmarshal(prepare(parent), &second); e != nil || second.Workspace == session.Workspace {
		t.Fatal("sample reset should create a separate copy", e)
	}
	bad := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "samples/create-playground.ps1", "-OutputParent", "samples", "-NoOpen")
	if e := bad.Run(); e == nil {
		t.Fatal("sample script accepted the protected source repository as output")
	}
	for _, scenario := range session.Scenarios {
		if scenario.Tool == "dedupe" {
			continue // Never launch third-party GUIs from an automated test.
		}
		t.Run(scenario.Tool, func(t *testing.T) {
			run := func(execute bool) []byte {
				args := []string{"cli", scenario.Tool, "--config", scenario.Config}
				if execute {
					args = append(args, "--execute")
				}
				cmd := exec.CommandContext(ctx, exe, args...)
				var out, stderr bytes.Buffer
				cmd.Stdout, cmd.Stderr = &out, &stderr
				if e := cmd.Run(); e != nil {
					t.Fatal("hands-on sample command", e, out.String(), stderr.String())
				}
				return out.Bytes()
			}
			var p toolbox.Plan
			if e := json.Unmarshal(run(false), &p); e != nil || len(p.Items) == 0 {
				t.Fatal("sample preview is empty or invalid", e, p)
			}
			for _, item := range p.Items {
				if item.Conflict != "" {
					t.Fatal("sample has a preview conflict", item)
				}
			}
			var result toolbox.Result
			if e := json.Unmarshal(run(true), &result); e != nil || result.Success == 0 || result.Failed != 0 {
				t.Fatal("sample execution did not complete", e, result)
			}
			for _, item := range p.Items {
				if scenario.Tool == "image" && result.Skipped != 0 {
					continue // An already compact image may intentionally be skipped.
				}
				if _, e := os.Stat(item.Target); e != nil {
					t.Fatal("expected hands-on result missing", e)
				}
			}
			if scenario.Tool == "number" {
				for _, name := range []string{"028 - 已有资源", "029 - 资源1", "030 - 资源2", "031 - 资源10"} {
					if _, e := os.Stat(filepath.Join(scenario.Input, name)); e != nil {
						t.Fatal("numbering guide does not match sample output", e)
					}
				}
			}
			if scenario.Tool == "extract" && len(p.Items) != 4 {
				t.Fatal("split volumes should form one task, four archives in total", len(p.Items))
			}
		})
	}
	for name, hash := range sampleHashes(t) {
		if before[name] != hash {
			t.Fatal("hands-on execution changed original repository samples", name)
		}
	}
}
