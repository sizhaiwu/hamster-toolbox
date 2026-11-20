package toolbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRemainingPlanHonorsScanBoundaries(t *testing.T) {
	c := DefaultConfig()
	c.Tool, c.MetadataTypes = "exif", "images"
	c.Root = t.TempDir()
	c.Output = filepath.Join(c.Root, "custom-output")
	c.Exiftool, _ = os.Executable()
	paths := []string{"root.jpg", "nested/wanted.jpg", "nested/completed.jpg", "unrelated/other.jpg", ".git/ignored.jpg", "toolbox-output/ignored.jpg", "custom-output/ignored.jpg"}
	for i, name := range paths {
		paths[i] = filepath.Join(c.Root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(paths[i]), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(paths[i], []byte("synthetic"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// The old record may mention paths that are now excluded or designated
	// as output. Targeted scanning must not bypass current scan boundaries.
	wanted := []string{paths[0], paths[1], paths[4], paths[5], paths[6]}
	for _, recursive := range []bool{true, false} {
		c.Recursive = recursive
		p, err := BuildRemainingPlan(context.Background(), c, wanted, func(string) {})
		if err != nil {
			t.Fatal(err)
		}
		expected := map[string]bool{paths[0]: true}
		if recursive {
			expected[paths[1]] = true
		}
		for _, item := range p.Items {
			if !expected[item.Source] {
				t.Fatalf("unexpected source %q", item.Source)
			}
			delete(expected, item.Source)
		}
		if len(expected) > 0 {
			t.Fatal("missing wanted sources", expected)
		}
	}
}
