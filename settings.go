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
	Version    int
	LastTool   string
	Workspaces map[string]toolbox.Config
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
		if key == "format" && c.FormatDeleteMode == "" {
			c.FormatDeleteMode = "recycle"
		}
		// Existing users retain the H.264 output compatibility of their saved jobs.
		if key == "video" && c.VideoMode == "" {
			c.VideoMode = "compatible"
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
	if e = os.MkdirAll(s.dataDir, 0700); e != nil {
		return p, e
	}
	b, e := json.MarshalIndent(p, "", "  ")
	if e != nil {
		return p, e
	}
	f, e := os.CreateTemp(s.dataDir, ".settings-*.json")
	if e != nil {
		return p, e
	}
	name := f.Name()
	defer os.Remove(name)
	if _, e = f.Write(b); e != nil {
		f.Close()
		return p, e
	}
	if e = f.Close(); e != nil {
		return p, e
	}
	if e = os.Rename(name, filepath.Join(s.dataDir, "settings.json")); e != nil {
		return p, e
	}
	return p, nil
}
