package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
)

// Explorer settings belong to the local owner, never to a shared-folder guest.
// They live alongside the other settings, independently of the browser's port.
type explorerPreferences struct {
	Workspace        *explorerSavedWorkspace `json:"workspace,omitempty"`
	Version          int                     `json:"version"`
	View             string                  `json:"view"`
	TileSize         int                     `json:"tileSize"`
	TileGap          *int                    `json:"tileGap,omitempty"`
	Favorites        []string                `json:"favorites"`
	PageSize         int                     `json:"pageSize"`
	Sort             string                  `json:"sort"`
	Order            string                  `json:"order"`
	SidebarCollapsed bool                    `json:"sidebarCollapsed"`
	HoverPreview     *bool                   `json:"hoverPreview,omitempty"`
}

type explorerPreferenceChange struct {
	Workspace        *explorerSavedWorkspace `json:"workspace"`
	View             *string                 `json:"view"`
	TileSize         *int                    `json:"tileSize"`
	TileGap          *int                    `json:"tileGap"`
	Favorites        *[]string               `json:"favorites"`
	PageSize         *int                    `json:"pageSize"`
	Sort             *string                 `json:"sort"`
	Order            *string                 `json:"order"`
	SidebarCollapsed *bool                   `json:"sidebarCollapsed"`
	HoverPreview     *bool                   `json:"hoverPreview"`
}

func (b *fileBrowser) readExplorerPreferences() (*explorerPreferences, error) {
	data, err := os.ReadFile(filepath.Join(b.dataDir, "explorer.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取资源管理器设置失败：%w", err)
	}
	var p explorerPreferences
	if err = json.Unmarshal(data, &p); err != nil || p.Version != 1 {
		return nil, errors.New("资源管理器设置文件损坏或版本不支持，已保留原文件")
	}
	if err = validateExplorerPreferences(&p); err != nil {
		return nil, fmt.Errorf("资源管理器设置无效，已保留原文件：%w", err)
	}
	return &p, nil
}

func (b *fileBrowser) loadExplorerPreferences() (*explorerPreferences, error) {
	b.explorerSettings.Lock()
	defer b.explorerSettings.Unlock()
	return b.readExplorerPreferences()
}

func validateExplorerPreferences(p *explorerPreferences) error {
	// Version 1 files created before these controls used the defaults below.
	var err error
	p.Sort, p.Order, p.PageSize, err = browserListOptions(p.Sort, p.Order, p.PageSize)
	if err != nil {
		return err
	}
	if p.View != "grid" && p.View != "list" {
		return errors.New("视图应为图标或列表")
	}
	if p.TileSize < 140 || p.TileSize > 420 {
		return errors.New("图标大小应在 140 到 420 之间")
	}
	if p.TileGap != nil && (*p.TileGap < 0 || *p.TileGap > 32) {
		return errors.New("图标间距应在 0 到 32 之间")
	}
	if len(p.Favorites) > 20 {
		return errors.New("快速访问最多添加 20 个目录")
	}
	seen := map[string]bool{}
	favorites := []string{}
	for _, path := range p.Favorites {
		if len(path) > 32768 || !filepath.IsAbs(path) || strings.ContainsFunc(path, unicode.IsControl) {
			return errors.New("快速访问目录需要完整的本机路径")
		}
		// Do not stat: an unplugged disk or temporarily offline share is still a favorite.
		path = filepath.Clean(path)
		key := path
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if !seen[key] {
			seen[key] = true
			favorites = append(favorites, path)
		}
	}
	p.Favorites = favorites
	return nil
}

func (b *fileBrowser) saveExplorerPreferences(r *http.Request) (*explorerPreferences, error) {
	var change explorerPreferenceChange
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&change); err != nil {
		return nil, errors.New("资源管理器设置格式不正确")
	}
	b.explorerSettings.Lock()
	defer b.explorerSettings.Unlock()
	p, err := b.readExplorerPreferences()
	if err != nil {
		return nil, err // Never overwrite a damaged settings file with empty favorites.
	}
	if p == nil {
		p = &explorerPreferences{Version: 1, View: "grid", TileSize: 180, Favorites: []string{}}
	}
	// Partial updates prevent a view change in another tab from clearing favorites.
	if change.Workspace != nil {
		if err = validateSavedWorkspace(change.Workspace); err != nil {
			return nil, err
		}
		p.Workspace = change.Workspace
	}
	if change.View != nil {
		p.View = *change.View
	}
	if change.TileSize != nil {
		p.TileSize = *change.TileSize
	}
	if change.TileGap != nil {
		p.TileGap = change.TileGap
	}
	if change.Favorites != nil {
		p.Favorites = *change.Favorites
	}
	if change.PageSize != nil {
		if !browserPageSizeValid(*change.PageSize) {
			return nil, errors.New("每页条数无效")
		}
		p.PageSize = *change.PageSize
	}
	if change.Sort != nil {
		p.Sort = *change.Sort
	}
	if change.Order != nil {
		p.Order = *change.Order
	}
	if change.SidebarCollapsed != nil {
		p.SidebarCollapsed = *change.SidebarCollapsed
	}
	if change.HoverPreview != nil {
		p.HoverPreview = change.HoverPreview
	}
	if err = validateExplorerPreferences(p); err != nil {
		return nil, err
	}
	if err = os.MkdirAll(b.dataDir, 0700); err != nil {
		return nil, fmt.Errorf("保存资源管理器设置失败：%w", err)
	}
	f, err := os.CreateTemp(b.dataDir, ".explorer-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("保存资源管理器设置失败：%w", err)
	}
	defer os.Remove(f.Name())
	data, err := json.MarshalIndent(p, "", "  ")
	if err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(f.Name(), filepath.Join(b.dataDir, "explorer.json"))
	}
	if err != nil {
		return nil, fmt.Errorf("保存资源管理器设置失败：%w", err)
	}
	return p, nil
}
