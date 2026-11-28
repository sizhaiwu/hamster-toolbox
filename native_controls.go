package main

import (
	_ "embed"
	"os"
	"path/filepath"
)

// This controller is our own small script, executed only by the separate mpv
// process. It does not load user scripts or access the current resource folder.
//
//go:embed native_controls.lua
var nativeController []byte

func prepareNativeController(dir string) (string, string, error) {
	if dir == "" {
		var err error
		dir, err = os.MkdirTemp("", "hamster-mpv-")
		if err != nil {
			return "", "", err
		}
	}
	file := filepath.Join(dir, "hamster-controls.lua")
	if err := os.WriteFile(file, nativeController, 0600); err != nil {
		removeNativeController(dir)
		return "", "", err
	}
	return file, dir, nil
}

func removeNativeController(dir string) {
	if dir != "" {
		os.Remove(filepath.Join(dir, "hamster-controls.lua"))
		os.Remove(dir)
	}
}
