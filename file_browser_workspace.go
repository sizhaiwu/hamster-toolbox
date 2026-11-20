package main

import (
	"errors"
	"path/filepath"
	"regexp"
)

// Persist navigation only. Credentials, selection versions and mutations have no fields here.
type explorerSavedWorkspace struct {
	Version int                `json:"version"`
	Active  string             `json:"active"`
	Tabs    []explorerSavedTab `json:"tabs"`
}
type explorerSavedTab struct {
	ID     string  `json:"id"`
	Root   string  `json:"root"`
	Path   string  `json:"path"`
	Search string  `json:"search"`
	Sort   string  `json:"sort"`
	Order  string  `json:"order"`
	View   string  `json:"view"`
	Offset int     `json:"offset"`
	Scroll float64 `json:"scroll"`
}

func validateSavedWorkspace(w *explorerSavedWorkspace) error {
	if w.Version != 1 || len(w.Tabs) < 1 || len(w.Tabs) > 20 {
		return errors.New("标签恢复记录无效")
	}
	validID := regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)
	seen := map[string]bool{}
	active := false
	for _, tab := range w.Tabs {
		if !validID.MatchString(tab.ID) || seen[tab.ID] || tab.Root != "" || len(tab.Path) > 32768 || tab.Path != "" && !filepath.IsAbs(tab.Path) || len(tab.Search) > 1024 || tab.Offset < 0 || tab.Scroll < 0 || tab.Scroll > 20000000 {
			return errors.New("标签恢复记录包含无效目录或状态")
		}
		seen[tab.ID] = true
		if tab.ID == w.Active {
			active = true
		}
		if tab.Sort != "name" && tab.Sort != "modified" && tab.Sort != "size" || tab.Order != "asc" && tab.Order != "desc" || tab.View != "grid" && tab.View != "list" {
			return errors.New("标签显示状态无效")
		}
	}
	if !active {
		return errors.New("当前标签不存在")
	}
	return nil
}
