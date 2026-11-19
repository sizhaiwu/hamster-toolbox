package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
)

// A new CLI option should not ship without a description in both terminal help
// and the matching tool's reference section.
func TestCLIReferenceAndHelpCoverSupportedParameters(t *testing.T) {
	data, e := os.ReadFile("docs/cli-reference.md")
	if e != nil {
		t.Fatal(e)
	}
	doc := strings.ReplaceAll(string(data), "\r\n", "\n")
	for _, tool := range []string{"format", "rename", "number", "describe", "exif", "pdf", "image", "video", "thumbnail", "archive", "extract", "dedupe"} {
		t.Run(tool, func(t *testing.T) {
			_, section, found := strings.Cut(doc, "\n## "+tool+" ")
			if !found {
				t.Fatal("missing tool reference section")
			}
			section, _, _ = strings.Cut(section, "\n## ")
			var out, stderr bytes.Buffer
			if code := runCLI(context.Background(), []string{tool, "--help"}, nil, &out, &stderr); code != 0 {
				t.Fatal(code, stderr.String())
			}
			for field := range cliFields(tool) {
				if field == "ArchivePassword" || field == "ExtractPassword" {
					continue // Passwords use --password-env, not plaintext flags.
				}
				flag := cliFlagName(field)
				if !strings.Contains(section, "`--"+flag+"`") {
					t.Errorf("--%s missing from %s reference", flag, tool)
				}
				help := cliFieldHelp[field]
				if help == "" || !strings.Contains(stderr.String(), help) {
					t.Errorf("--%s lacks descriptive terminal help", flag)
				}
			}
		})
	}
	for _, field := range []string{"FFmpeg", "FFprobe"} {
		if !cliFields("exif")[field] {
			t.Errorf("video metadata cleanup must accept a custom %s location", field)
		}
	}
}
