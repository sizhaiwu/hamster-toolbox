package main

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"runtime"
	"strings"
)

// The GUI launcher is built from the same application, with the Windows GUI
// subsystem. CLI users keep the ordinary console executable.
var desktopMode = "false"

func desktopInstanceName(data string) string {
	p, err := filepath.Abs(data)
	if err != nil {
		p = filepath.Clean(data)
	}
	if physical, err := filepath.EvalSymlinks(p); err == nil {
		p = physical
	}
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	sum := sha256.Sum256([]byte(p))
	return "HamsterToolbox.Desktop." + hex.EncodeToString(sum[:12])
}

func (s *server) desktopCanExit() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.Status != "running" && s.state.Status != "scanning"
}

func (s *server) beginDesktopExit() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy() {
		return false
	}
	s.desktopClosing = true
	return true
}
