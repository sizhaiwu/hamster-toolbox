package toolbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSelectedExtractIntegration(t *testing.T) {
	c := extractFixture(t)
	c.ExtractSource = filepath.Join(c.Root, "selected.zip")
	writeExtractZIP(t, c.ExtractSource, map[string]string{"result.txt": "selected only"})
	writeArchiveFixture(t, filepath.Join(c.Root, "other.zip"), []byte("broken archive must not be scanned"))
	p, err := BuildPlan(context.Background(), c, quiet)
	if err != nil || len(p.Items) != 1 || p.Items[0].Source != c.ExtractSource || p.Items[0].Conflict != "" {
		t.Fatalf("selected plan: %+v %v", p, err)
	}
	if r := Execute(context.Background(), p, quiet, progress); r.Success != 1 || r.Failed != 0 {
		t.Fatal(r)
	}
	b, err := os.ReadFile(filepath.Join(p.Items[0].Target, "result.txt"))
	if err != nil || string(b) != "selected only" {
		t.Fatal(string(b), err)
	}
	c.ExtractSource = filepath.Join(t.TempDir(), "outside.zip")
	writeExtractZIP(t, c.ExtractSource, map[string]string{"outside.txt": "not selected"})
	if _, err = BuildPlan(context.Background(), c, quiet); err == nil {
		t.Fatal("cross-directory source accepted")
	}
}

func TestSelectedExtractReadOnlyParentScope(t *testing.T) {
	f, err := os.CreateTemp("", "hamster-selected-*.zip")
	if err != nil {
		t.Fatal(err)
	}
	path := f.Name()
	f.Close()
	defer os.Remove(path)
	c := DefaultConfig()
	c.Tool = "extract"
	c.Root = filepath.Dir(path)
	c.ExtractSource = path
	c.SevenZip, _ = os.Executable()
	if _, err = Normalize(c); err != nil {
		t.Fatal("explicit file rejected in read-only parent scope", err)
	}
	c.ExtractSource = ""
	if _, err = Normalize(c); err == nil {
		t.Fatal("whole temporary root became scannable")
	}
	c.ExtractSource = path
	c.Tool = "format"
	if _, err = Normalize(c); err == nil {
		t.Fatal("single-archive scope relaxed another tool")
	}
}
