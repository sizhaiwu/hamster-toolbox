//go:build !windows

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

func TestPOSIXCLIExportCanExecuteWithLiteralPaths(t *testing.T) {
	dir := t.TempDir()
	program := filepath.Join(dir, "tool box ' test")
	if e := os.WriteFile(program, []byte("#!/bin/sh\ncat >\"$TOOLBOX_TEST_CAPTURE\"\nprintf '%s\\n' \"$@\" >\"$TOOLBOX_TEST_ARGS\"\n"), 0700); e != nil {
		t.Fatal(e)
	}
	c := toolbox.DefaultConfig()
	c.Tool = "rename"
	c.Root = filepath.Join(dir, "literal ' $(touch forbidden) path")
	c.Prefix = "测试-"
	commands, e := buildCLICommands(c, program)
	if e != nil {
		t.Fatal(e)
	}
	capture, args := filepath.Join(dir, "config.json"), filepath.Join(dir, "args.txt")
	cmd := exec.Command("sh", "-c", commands.Execute)
	cmd.Env = append(os.Environ(), "TOOLBOX_TEST_CAPTURE="+capture, "TOOLBOX_TEST_ARGS="+args)
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatal(e, string(out))
	}
	var decoded toolbox.Config
	data, _ := os.ReadFile(capture)
	if e := json.Unmarshal(data, &decoded); e != nil || decoded.Root != c.Root || decoded.Prefix != c.Prefix {
		t.Fatal("literal config damaged", e, string(data))
	}
	argv, _ := os.ReadFile(args)
	if !strings.Contains(string(argv), "--execute") || !strings.Contains(string(argv), "--config\n-\n") {
		t.Fatal(string(argv))
	}
}
