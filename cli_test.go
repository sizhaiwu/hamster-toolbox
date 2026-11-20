package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

func TestCLIPreviewExecuteAndConfigOverride(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "旧名字.txt")
	os.WriteFile(source, []byte("fixture"), 0600)
	var out, err bytes.Buffer
	args := []string{"rename", "--root", root, "--prefix", "new-"}
	if code := runCLI(context.Background(), args, strings.NewReader(""), &out, &err); code != 0 {
		t.Fatal(code, err.String())
	}
	if _, e := os.Stat(source); e != nil {
		t.Fatal("preview changed source", e)
	}
	var p toolbox.Plan
	if e := json.Unmarshal(out.Bytes(), &p); e != nil || len(p.Items) != 1 {
		t.Fatal(e, out.String())
	}
	config, _ := json.Marshal(map[string]any{"Tool": "rename", "Root": root, "Prefix": "wrong-"})
	out.Reset()
	err.Reset()
	if code := runCLI(context.Background(), []string{"rename", "--config", "-", "--prefix", "new-", "--execute"}, bytes.NewReader(config), &out, &err); code != 0 {
		t.Fatal(code, err.String())
	}
	if _, e := os.Stat(filepath.Join(root, "new-旧名字.txt")); e != nil {
		t.Fatal(e)
	}
}

func TestCLIAllToolHelpAndInvalidOptions(t *testing.T) {
	for _, tool := range []string{"format", "rename", "number", "describe", "exif", "pdf", "image", "video", "thumbnail", "archive", "extract", "dedupe"} {
		var out, err bytes.Buffer
		if code := runCLI(context.Background(), []string{tool, "--help"}, nil, &out, &err); code != 0 {
			t.Fatal(tool, code, err.String())
		}
	}
	for _, args := range [][]string{{"unknown"}, {"rename", "--video-mode", "efficient"}, {"rename", "--config", "-"}, {"format", "--root", filepath.VolumeName(os.TempDir()) + string(filepath.Separator), "--execute"}} {
		var out, err bytes.Buffer
		if code := runCLI(context.Background(), args, strings.NewReader(`{"UnknownField":true}`), &out, &err); code == 0 {
			t.Fatal("invalid command accepted", args)
		}
	}
}

func TestCLICommandsNoPasswordAndQuotes(t *testing.T) {
	c := toolbox.DefaultConfig()
	c.Tool, c.Root, c.ArchivePassword = "archive", `E:\资源 ' $()`, "secret-do-not-export"
	commands, e := buildCLICommands(c, `E:\工具箱\hamster-toolbox.exe`)
	if e != nil {
		t.Fatal(e)
	}
	all := commands.Preview + commands.Execute + commands.Notice
	if strings.Contains(all, c.ArchivePassword) || strings.Contains(commands.Preview, "--execute") {
		t.Fatal("unsafe export", all)
	}
	if !strings.Contains(commands.Execute, "--execute") || !strings.Contains(all, "TOOLBOX_ARCHIVE_PASSWORD") {
		t.Fatal(all)
	}
	if runtime.GOOS != "windows" {
		if !strings.Contains(commands.Preview, "printf") {
			t.Fatal(commands.Preview)
		}
		return
	}
	encoded := strings.Split(strings.Split(commands.Preview, "\n@'\n")[1], "\n'@")[0]
	var got toolbox.Config
	if e := json.Unmarshal([]byte(encoded), &got); e != nil || got.Root != c.Root || got.ArchivePassword != "" {
		t.Fatal(e, got)
	}
}
