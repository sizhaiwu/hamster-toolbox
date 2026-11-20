package toolbox

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestUnicodeFontPathIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") != "1" {
		t.Skip("requires ImageMagick and a CJK font")
	}
	c := integrationConfig(t)
	dir := filepath.Join(t.TempDir(), "中文 空格")
	if e := os.MkdirAll(dir, 0755); e != nil {
		t.Fatal(e)
	}
	font := filepath.Join(dir, "中文字体"+filepath.Ext(c.Font))
	if e := publishContext(context.Background(), c.Font, font); e != nil {
		t.Fatal(e)
	}
	output := filepath.Join(dir, "output.png")
	_, e := runWithFont(context.Background(), c.Magick, "-size", "400x100", "xc:white", "-font", font, "-pointsize", "24", "-fill", "black", "-annotate", "+10+10", "中文测试", output)
	if e != nil {
		t.Fatal(e)
	}
	if !regularFile(output) {
		t.Fatal("font rendering output missing")
	}
}

func TestPortableBundleVersionChecks(t *testing.T) {
	base := t.TempDir()
	manifest := []byte(`{"FormatVersion":1}`)
	hash := fmt.Sprintf("%x", sha256.Sum256(manifest))
	if portableBundleWarningAt(base, "") != "" {
		t.Fatal("development build blocked")
	}
	if portableBundleWarningAt(base, hash) == "" {
		t.Fatal("missing manifest accepted")
	}
	file(t, filepath.Join(base, "tools-manifest.json"), manifest)
	if portableBundleWarningAt(base, hash) == "" {
		t.Fatal("missing dependency marker accepted")
	}
	marker := append([]byte{0xef, 0xbb, 0xbf}, []byte(fmt.Sprintf(`{"ManifestSHA256":%q}`, hash))...)
	file(t, filepath.Join(base, "tools", "bundle-files.json"), marker)
	if warning := portableBundleWarningAt(base, hash); warning != "" {
		t.Fatal(warning)
	}
	if warning := portableBundleWarningAt(base, "different-version"); warning == "" {
		t.Fatal("incompatible update accepted")
	}
}

func TestPortableDependenciesSurviveMovingAndDoNotRelocateUserFiles(t *testing.T) {
	oldBase, newBase := t.TempDir(), t.TempDir()
	rel := filepath.Join("tools", "imagemagick", programName("magick"))
	for _, base := range []string{oldBase, newBase} {
		file(t, filepath.Join(base, rel), []byte("program"))
		os.Chmod(filepath.Join(base, rel), 0700)
	}
	c := Config{Tool: "image", Root: "D:/photos", Output: "D:/results", Magick: filepath.Join(oldBase, rel), Quality: 70}
	stored := portableConfigAt(c, oldBase)
	if stored.Magick != filepath.ToSlash(rel) {
		t.Fatal(stored)
	}
	restored := dependencyPathsAt(stored, newBase, true)
	if restored.Magick != filepath.Join(newBase, rel) || restored.Root != c.Root || restored.Output != c.Output || restored.Quality != 70 {
		t.Fatal(restored)
	}
	// Both old and new installations can exist after an update.
	if got := dependencyPathsAt(c, newBase, true); got.Magick != filepath.Join(newBase, rel) {
		t.Fatal(got)
	}
	custom := filepath.Join(t.TempDir(), "custom-magick.exe")
	file(t, custom, []byte("custom"))
	os.Chmod(custom, 0700)
	c.Magick = custom
	if got := dependencyPathsAt(c, newBase, true); got.Magick != custom {
		t.Fatal("valid custom choice replaced", got)
	}
	if got := portableConfigAt(c, newBase); got.Magick != custom {
		t.Fatal(got)
	}
	if e := os.Remove(custom); e != nil {
		t.Fatal(e)
	}
	if got := dependencyPathsAt(c, newBase, true); got.Magick != filepath.Join(newBase, rel) {
		t.Fatal("missing program not repaired", got)
	}
	c.Magick = "missing.exe"
	if got := dependencyPathsAt(c, newBase, false); got.Magick != "missing.exe" {
		t.Fatal("explicit invalid selection masked", got)
	}
}

func TestPortablePathTraversalIsNotRebound(t *testing.T) {
	base := t.TempDir()
	file(t, filepath.Join(base, "outside.exe"), []byte("file"))
	c := Config{Tool: "image", Magick: filepath.Join(t.TempDir(), "old") + string(filepath.Separator) + "tools" + string(filepath.Separator) + ".." + string(filepath.Separator) + "outside.exe"}
	if got := dependencyPathsAt(c, base, true); got.Magick != c.Magick {
		t.Fatal(got)
	}
}
