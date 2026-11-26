package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

func TestCLIArchiveIndependentStarts(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "album"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "item.txt"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	program, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		args      []string
		dir, file string
	}{
		{"legacy", []string{"--archive-start", "29"}, "D_029.7z", "F_029.7z"},
		{"independent", []string{"--archive-start", "29", "--archive-dir-start", "99", "--archive-file-start", "3"}, "D_099.7z", "F_003.7z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"archive", "--root", root, "--seven-zip", program, "--archive-split", "none"}, tc.args...)
			var stdout, stderr bytes.Buffer
			if code := runCLI(context.Background(), args, nil, &stdout, &stderr); code != 0 {
				t.Fatal(code, stderr.String())
			}
			var plan toolbox.Plan
			if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil {
				t.Fatal(err)
			}
			got := map[string]string{}
			for _, item := range plan.Items {
				got[filepath.Base(item.Source)] = filepath.Base(item.Target)
			}
			if len(got) != 2 || got["album"] != tc.dir || got["item.txt"] != tc.file {
				t.Fatal(got)
			}
		})
	}
}
