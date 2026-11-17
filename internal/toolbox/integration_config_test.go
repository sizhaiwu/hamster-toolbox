package toolbox

import (
	"path/filepath"
	"testing"
)

// Go tests run from the package directory, while their executable lives in a
// temporary build directory. Locate the repository bundle explicitly for tests.
func integrationConfig(t *testing.T) Config {
	t.Helper()
	c := DefaultConfig()
	root, e := filepath.Abs(filepath.Join("..", ".."))
	if e != nil {
		t.Fatal(e)
	}
	for _, key := range []string{"FFmpeg", "FFprobe", "Magick", "Exiftool", "Font"} {
		if path := bundledDependency(root, key); path != "" {
			*dependencyField(&c, key) = path
		}
	}
	return c
}
