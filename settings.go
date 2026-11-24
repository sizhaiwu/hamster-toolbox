package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
	"os"
	"path/filepath"
)

type preferences struct {
	CloseDialogOnBackdrop *bool `json:",omitempty"`
	BrowserAutoPlay       *bool `json:",omitempty"`
	BrowserMuted          *bool `json:",omitempty"`
	RememberPlayback      *bool `json:",omitempty"`
	RestoreTabs           *bool `json:",omitempty"`
	DefaultPlayer         string
	FontScale             int
	ThumbnailCacheMB      *int `json:",omitempty"`
	Version               int
	LastTool              string
	Workspaces            map[string]toolbox.Config
	SidebarCollapsed      bool
	ArchiveAdvancedOpen   bool
	TaskPopup             *bool `json:",omitempty"`
	TaskSound             *bool `json:",omitempty"`
}

func (s *server) loadPreferences() (preferences, error) {
	p := preferences{Version: 1, LastTool: "format", Workspaces: map[string]toolbox.Config{}}
	b, e := os.ReadFile(filepath.Join(s.dataDir, "settings.json"))
	if errors.Is(e, os.ErrNotExist) {
		return p, nil
	}
	if e != nil {
		return p, fmt.Errorf("读取设置失败：%w", e)
	}
	if e = json.Unmarshal(b, &p); e != nil {
		return p, fmt.Errorf("设置文件损坏，未覆盖原文件：%w", e)
	}
	if p.Workspaces == nil {
		p.Workspaces = map[string]toolbox.Config{}
	}
	if !toolbox.ToolExists(p.LastTool) {
		p.LastTool = "format"
	}
	if c, ok := p.Workspaces["image"]; ok {
		c.ImageFormats = toolbox.RestoreImageFormats(c.ImageFormats)
		p.Workspaces["image"] = c
	}
	for key, c := range p.Workspaces {
		if c.Workers == 0 && (key == "image" || key == "video" || key == "exif" || key == "thumbnail") {
			c.Workers = 1
		}
		if key == "format" && c.FormatDeleteMode == "" {
			c.FormatDeleteMode = "recycle"
		}
		// Existing users retain the H.264 output compatibility of their saved jobs.
		if key == "video" && c.VideoMode == "" {
			c.VideoMode = "compatible"
		}
		if key == "video" && c.VideoContainer == "" {
			c.VideoContainer = "original"
		}
		if key == "video" && c.VideoHDR == "" {
			c.VideoHDR = "preserve"
		}
		if key == "video" && c.VideoAcceleration == "" {
			c.VideoAcceleration = "auto"
		}
		if key == "extract" && c.ExtractDeleteMode == "" {
			c.ExtractDeleteMode = "recycle"
		}
		c.ArchivePassword = ""
		c.ExtractPassword = ""
		p.Workspaces[key] = toolbox.RestoreDependencyPaths(c)
	}
	return p, nil
}

func (s *server) saveWorkspace(c toolbox.Config) (preferences, error) {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	if !toolbox.ToolExists(c.Tool) {
		return preferences{}, fmt.Errorf("未知工具")
	}
	p, e := s.loadPreferences()
	if e != nil {
		return p, e
	}
	c.ArchivePassword = ""
	c.ExtractPassword = ""
	p.LastTool = c.Tool
	p.Workspaces[c.Tool] = toolbox.PortableConfig(toolbox.ScopeConfig(c))
	for key, saved := range p.Workspaces {
		saved.ArchivePassword = ""
		saved.ExtractPassword = ""
		p.Workspaces[key] = toolbox.PortableConfig(saved)
	}
	return p, s.writePreferences(p)
}

// The caller holds settingsMu so workspace and UI updates cannot overwrite each other.
func (s *server) writePreferences(p preferences) error {
	for key, c := range p.Workspaces {
		c.ArchivePassword = ""
		c.ExtractPassword = ""
		p.Workspaces[key] = toolbox.PortableConfig(c)
	}
	if e := os.MkdirAll(s.dataDir, 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(p, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(s.dataDir, ".settings-*.json")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = os.Rename(name, filepath.Join(s.dataDir, "settings.json")); e != nil {
		return e
	}
	return nil
}
