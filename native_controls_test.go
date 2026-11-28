package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeControlModes(t *testing.T) {
	dir := t.TempDir()
	m := &nativePlayerManager{dataDir: dir, sessions: map[string]*nativePlayback{}}
	for _, mode := range []string{"full"} {
		if e := m.setControlsMode(mode); e != nil {
			t.Fatal(e)
		}
		args := strings.Join(m.mediaPlaybackArgs(nativePlayerArgs("movie.mp4", "pipe", 0, "compact"), "video"), " ")
		expected := "hamster-mode=" + mode
		if !strings.Contains(args, "--osc=no") {
			t.Fatal("large built-in controller is still enabled", args)
		}
		if !strings.Contains(args, expected) {
			t.Fatal(mode, args)
		}
		reloaded := &nativePlayerManager{dataDir: dir}
		reloaded.loadMediaPreferences()
		if reloaded.mediaPreferences.Controls != mode {
			t.Fatal("lost mode", mode)
		}
	}
	before, _ := os.ReadFile(filepath.Join(dir, "native-media.json"))
	if m.setControlsMode("--bad") == nil || m.setControlsMode("hidden") == nil || m.setControlsMode("simple") == nil {
		t.Fatal("invalid mode accepted")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "native-media.json"))
	if string(before) != string(after) {
		t.Fatal("invalid mode changed settings")
	}
	m.dataDir = filepath.Join(dir, "native-media.json")
	if m.setControlsMode("full") == nil || m.mediaPreferences.Controls != "full" {
		t.Fatal("failed write changed mode")
	}
}

func TestNativeLegacyControlsMigration(t *testing.T) {
	for _, mode := range []string{"simple", "hidden", "", "full"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			data := `{"Controls":"` + mode + `","Remember":true,"FocusAudio":true}`
			if err := os.WriteFile(filepath.Join(dir, "native-media.json"), []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			m := &nativePlayerManager{dataDir: dir}
			m.loadMediaPreferences()
			if m.mediaPreferences.Controls != "full" || !m.mediaPreferences.Remember || !m.mediaPreferences.FocusAudio {
				t.Fatal(m.mediaPreferences)
			}
			if nativeControlsMode("--bad") != "full" || validNativeControls("hidden") || validNativeControls("simple") {
				t.Fatal("invalid or removed mode accepted")
			}
			if !strings.Contains(nativeOSCOptions(m.mediaPreferences.Controls), "hamster-mode=full") {
				t.Fatal("legacy mode did not restore unified controls")
			}
		})
	}
}

func TestNativeControllerMaterialization(t *testing.T) {
	file, dir, err := prepareNativeController("")
	if err != nil {
		t.Fatal(err)
	}
	defer removeNativeController(dir)
	data, err := os.ReadFile(file)
	if err != nil || string(data) != string(nativeController) {
		t.Fatal("controller source did not match embedded content", err)
	}
	removeNativeController(dir)
	if _, err = os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("controller temporary files were left behind", err)
	}
}
