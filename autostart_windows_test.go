//go:build windows

package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func TestAutostartWindowsCommand(t *testing.T) {
	program := `E:\程序 files\仓鼠工具箱\hamster-toolbox-desktop.exe`
	data := `E:\数据 files\profiles\`
	command, err := autostartCommand(program, data, `E:\default`)
	if err != nil {
		t.Fatal(err)
	}
	args, err := windows.DecomposeCommandLine(command)
	if err != nil || len(args) != 5 || args[0] != program || args[1] != "-tray" || args[2] != "-no-open" || args[3] != "-data" || args[4] != filepath.Clean(data) {
		t.Fatal(args, err)
	}
	if !autostartCommandsEqual(command, strings.ReplaceAll(command, "E:", "e:")) {
		t.Fatal("equivalent paths differ")
	}
	other, _ := autostartCommand(program, `E:\other`, ``)
	if autostartCommandsEqual(command, other) {
		t.Fatal("different profiles conflated")
	}
	command, err = autostartCommand(program, `E:\DEFAULT`, `e:\default`)
	if err != nil || strings.Contains(command, "-data") || !strings.HasPrefix(command, `"`) {
		t.Fatal(command, err)
	}
	if _, err = autostartCommand(`E:\`+strings.Repeat("long", 70)+`\desktop.exe`, "", ""); err == nil {
		t.Fatal("overlong Run command accepted")
	}
}
func TestAutostartWindowsIsolatedRegistry(t *testing.T) {
	// Never access the user's real Run key. Parameterize a unique disposable key.
	path := fmt.Sprintf(`Software\HamsterToolbox\AutostartTests-%d`, time.Now().UnixNano())
	s := registryAutostartStore{registry.CURRENT_USER, path, "test-startup"}
	t.Cleanup(func() { registry.DeleteKey(registry.CURRENT_USER, path) })
	if value, err := s.Read(); err != nil || value != "" {
		t.Fatal(value, err)
	}
	if err := s.Delete(); err != nil {
		t.Fatal(err)
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	defer key.Close()
	if err = key.SetStringValue("other-app", "preserve"); err != nil {
		t.Fatal(err)
	}
	command := `"E:\synthetic\desktop.exe" -tray -no-open`
	if err = s.Write(command); err != nil {
		t.Fatal(err)
	}
	if value, err := s.Read(); err != nil || value != command {
		t.Fatal(value, err)
	}
	if err = s.Delete(); err != nil {
		t.Fatal(err)
	}
	if value, err := s.Read(); err != nil || value != "" {
		t.Fatal(value, err)
	}
	if value, _, err := key.GetStringValue("other-app"); err != nil || value != "preserve" {
		t.Fatal("other startup changed", value, err)
	}
}
