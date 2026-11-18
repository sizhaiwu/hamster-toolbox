package toolbox

import (
	"context"
	"path/filepath"
	"testing"
)

func TestStripDirectoryStatsSuffix(t *testing.T) {
	for _, name := range []string{
		"Sample作者 [70P2V-517MB]",
		"Sample作者 [70P-2V-517MB]",
		"Sample作者 [70p2v-517mb]",
		"Sample作者 [70P 2V - 517 MB]",
		"Sample作者 [70P2V3A-0.5GiB]",
		"Sample作者 [517MB]",
		"Sample作者 [70P2V-517MB] [70P-2V-518.80MB]",
		"Sample作者 [46P6V-343M]",
		"Sample作者 [43P 2V-194M]",
		"Sample作者 [53P 2V]",
		"Sample作者 [35P]",
		"Sample作者 [70P2V]",
	} {
		if got := stripDirectoryStatsSuffix(name); got != "Sample作者" {
			t.Fatal(name, got)
		}
	}
	for _, name := range []string{"Sample作者 [4K]", "Sample作者 [1080P]", "Sample作者 [HDR]", "Sample作者 [第70期]", "Sample作者 [V2-517MB]"} {
		if got := stripDirectoryStatsSuffix(name); got != name {
			t.Fatal("descriptive name removed", name, got)
		}
	}
	if got := stripDirectoryStatsSuffix("Sample作者 [4K] [70P2V-517MB] [70P-2V-518.80MB]"); got != "Sample作者 [4K]" {
		t.Fatal(got)
	}
	for name, want := range map[string]string{
		"Sample作者合集[58P-5V-259M] [58P-5V-258.63MB]": "Sample作者合集",
		"Sample作者 [96P 6V] ✦标签✦ [96P-6V-594.80MB]":  "Sample作者 ✦标签✦",
		"Sample作者 [70P2V-517MB] 花絮":                 "Sample作者 花絮",
	} {
		if got := stripDirectoryStatsSuffix(name); got != want {
			t.Fatal(name, got, want)
		}
	}
}

func TestFormatLegacyStatisticsAndRepeatedSuffix(t *testing.T) {
	for _, suffix := range []string{"[70P2V-517MB]", "[70P2V-517MB] [70P-2V-518.80MB]", "[70p2v-517mb]", "[53P 2V]", "[35P]", "[46P6V-343M]", "[43P 2V-194M]"} {
		root := t.TempDir()
		dir := filepath.Join(root, "Sample作者 "+suffix)
		file(t, filepath.Join(dir, "image.jpg"), []byte("a"))
		file(t, filepath.Join(dir, "video.mp4"), []byte("bc"))
		c := DefaultConfig()
		c.Tool, c.Root, c.Recursive = "format", root, false
		p, err := BuildPlan(context.Background(), c, quiet)
		if err != nil || len(p.Items) != 1 {
			t.Fatal(p, err)
		}
		if p.Items[0].Target != filepath.Join(root, "Sample作者 [1P-1V-3B]") || p.Items[0].Conflict != "" {
			t.Fatal(p.Items)
		}
		r := Execute(context.Background(), p, quiet, progress)
		if r.Success != 1 || r.Failed != 0 {
			t.Fatal(r)
		}
		p, err = BuildPlan(context.Background(), c, quiet)
		if err != nil || len(p.Items) != 0 {
			t.Fatal("formatting is not idempotent", p, err)
		}
	}
}
