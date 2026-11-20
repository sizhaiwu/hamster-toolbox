package toolbox

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestDedupeDetectionAndCommand(t *testing.T) {
	base := t.TempDir()
	apps := t.TempDir()
	name := "VDF.GUI.exe"
	if runtime.GOOS != "windows" {
		name = "VDF.GUI"
	}
	program := filepath.Join(apps, "VideoDuplicateFinder", name)
	file(t, program, []byte("test fixture"))
	if p := detectDedupeAt(base, []string{apps}); p != program {
		t.Fatal(p)
	}
	info := InspectDedupeProgram(`"` + program + `"`)
	if !info.Available || info.Path != program {
		t.Fatal(info)
	}
	cmd, e := DedupeLaunchCommand(program)
	if e != nil {
		t.Fatal(e)
	}
	if cmd.Path != program || cmd.Dir != filepath.Dir(program) || len(cmd.Args) != 1 || cmd.Args[0] != program {
		t.Fatal("launcher must use GUI directly with no scan/delete arguments", cmd)
	}
	if _, e = DedupeLaunchCommand(filepath.Join(apps, "VDF.CLI.exe")); e == nil {
		t.Fatal("CLI accepted as GUI")
	}
	if _, e = DedupeLaunchCommand(filepath.Join(apps, "missing", "VDF.GUI.exe")); e == nil {
		t.Fatal("missing executable accepted")
	}
}

func TestDedupeSettingsScopeAndRelocation(t *testing.T) {
	oldBase, newBase := t.TempDir(), t.TempDir()
	rel := filepath.Join("tools", "videoduplicatefinder", "VDF.GUI.exe")
	file(t, filepath.Join(newBase, rel), []byte("fixture"))
	c := Config{Tool: "dedupe", DedupeProgram: filepath.Join(oldBase, rel), Root: "unused", ArchivePassword: "unused", FFmpeg: "unused"}
	c = ScopeConfig(c)
	if c.Root != "" || c.ArchivePassword != "" || c.FFmpeg != "" {
		t.Fatal("unrelated settings leaked")
	}
	stored := portableConfigAt(c, oldBase)
	if stored.DedupeProgram != "tools/videoduplicatefinder/VDF.GUI.exe" {
		t.Fatal(stored)
	}
	restored := dependencyPathsAt(stored, newBase, true)
	if restored.DedupeProgram != filepath.Join(newBase, rel) {
		t.Fatal(restored)
	}
	if !ToolExists("dedupe") || len(RequiredDependencies(c)) != 0 {
		t.Fatal("optional launcher became a required dependency")
	}
}
