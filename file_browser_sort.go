package main

import (
	"errors"
	"sort"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

func browserPageSizeValid(size int) bool {
	switch size {
	case 50, 100, 200, 500, 1000:
		return true
	}
	return false
}
func browserListOptions(key, order string, size int) (string, string, int, error) {
	if key == "" {
		key = "name"
	}
	if key != "name" && key != "modified" && key != "size" {
		return "", "", 0, errors.New("排序方式无效")
	}
	if order == "" {
		order = "asc"
		if key != "name" {
			order = "desc"
		}
	}
	if order != "asc" && order != "desc" {
		return "", "", 0, errors.New("排序方向无效")
	}
	if size == 0 {
		size = 200
	}
	if !browserPageSizeValid(size) {
		return "", "", 0, errors.New("每页条数请选择 50、100、200、500 或 1000")
	}
	return key, order, size, nil
}
func sortBrowserEntries(items []browserEntry, key, order string) {
	sort.Slice(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Dir != b.Dir {
			return a.Dir
		}
		compare := 0
		if key == "size" && a.Size != b.Size {
			if a.Size < b.Size {
				compare = -1
			} else {
				compare = 1
			}
		}
		if key == "modified" && !a.Modified.Equal(b.Modified) {
			if a.Modified.Before(b.Modified) {
				compare = -1
			} else {
				compare = 1
			}
		}
		if compare == 0 {
			if toolbox.BrowserNameLess(a.Name, b.Name) {
				compare = -1
			} else if toolbox.BrowserNameLess(b.Name, a.Name) {
				compare = 1
			}
		}
		if order == "desc" {
			return compare > 0
		}
		return compare < 0
	})
}
