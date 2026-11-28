package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type schedulePathEntry struct {
	Name, Path string
	Directory  bool
}

func schedulePathList(path, mode, search string) (map[string]any, error) {
	if mode != "dir" && mode != "file" {
		return nil, errors.New("选择类型无效")
	}
	entries := []schedulePathEntry{}
	if path == "" {
		for _, root := range browserLocations() {
			entries = append(entries, schedulePathEntry{root, root, true})
		}
		return map[string]any{"Current": "", "Parent": "", "Entries": entries}, nil
	}
	path, e := libraryExistingPath(path, true)
	if e != nil {
		return nil, e
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	truncated := false
	search = strings.ToLower(search)
	for {
		batch, err := f.ReadDir(500)
		for _, entry := range batch {
			if !strings.Contains(strings.ToLower(entry.Name()), search) {
				continue
			}
			full := filepath.Join(path, entry.Name())
			st, e := os.Stat(full)
			if e != nil {
				continue
			}
			if !st.IsDir() && (mode == "dir" || !st.Mode().IsRegular()) {
				continue
			}
			entries = append(entries, schedulePathEntry{entry.Name(), full, st.IsDir()})
			if len(entries) >= 500 {
				truncated = true
				break
			}
		}
		if truncated || err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Directory != entries[j].Directory {
			return entries[i].Directory
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	parent := filepath.Dir(path)
	if parent == path {
		parent = ""
	}
	return map[string]any{"Current": path, "Parent": parent, "Entries": entries, "Truncated": truncated}, nil
}
