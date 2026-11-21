package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func browserLocations() []string {
	paths := []string{}
	if runtime.GOOS == "windows" {
		for c := 'C'; c <= 'Z'; c++ {
			paths = append(paths, string(c)+`:\`)
		}
	} else {
		paths = append(paths, "/")
		if home, e := os.UserHomeDir(); e == nil {
			paths = append(paths, home)
		}
		for _, base := range []string{"/Volumes", "/mnt", "/media", "/run/media"} {
			if st, e := os.Stat(base); e == nil && st.IsDir() {
				paths = append(paths, base)
			}
		}
	}
	result := []string{}
	seen := map[string]bool{}
	for _, p := range paths {
		if st, e := os.Stat(p); e == nil && st.IsDir() && !seen[p] {
			result = append(result, p)
			seen[p] = true
		}
	}
	return result
}

func openBrowserCommand(address string) *exec.Cmd {
	return browserCommandFor(runtime.GOOS, address, exec.LookPath)
}

func browserCommandFor(platform, address string, lookup func(string) (string, error)) *exec.Cmd {
	switch platform {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", address)
	case "darwin":
		return exec.Command("open", address)
	default:
		if path, err := lookup("xdg-open"); err == nil {
			return exec.Command(path, address)
		}
		if path, err := lookup("gio"); err == nil {
			return exec.Command(path, "open", address)
		}
		return exec.Command("xdg-open", address)
	}
}

func selectableProgram(path string, info os.FileInfo) bool {
	if runtime.GOOS == "windows" {
		return filepath.Ext(path) == ".exe"
	}
	return info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0
}
