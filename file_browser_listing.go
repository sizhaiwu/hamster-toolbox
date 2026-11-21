package main

import (
	"fmt"
	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type directoryListingSnapshot struct {
	Items    []browserEntry
	At       time.Time
	Modified time.Time
}

func (b *fileBrowser) readDirectoryListing(r *http.Request, path string, scope *toolbox.BrowserScope, search string, dirs, recursive, force bool) ([]browserEntry, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	revision := b.revision
	b.mu.Unlock()
	scopeKey := "local"
	if scope != nil {
		scopeKey = scope.Root
	}
	b.listingMu.Lock()
	epoch := b.listingEpoch
	key := fmt.Sprintf("%s|%s|%s|%t|%t|%d|%d", scopeKey, path, search, dirs, recursive, revision, epoch)
	cached, ok := b.listings[key]
	b.listingMu.Unlock()
	if !force && ok && time.Since(cached.At) < 5*time.Second && cached.Modified.Equal(info.ModTime()) {
		return append([]browserEntry(nil), cached.Items...), nil
	}
	items := []browserEntry{}
	needle := strings.ToLower(search)
	var scan func(string, int) error
	scan = func(dir string, depth int) error {
		if err := r.Context().Err(); err != nil {
			return err
		}
		if depth > 128 {
			return nil
		}
		f, err := toolbox.BrowseOpen(dir)
		if err != nil {
			if depth == 0 {
				return err
			}
			return nil
		}
		defer f.Close()
		for {
			entries, readErr := f.ReadDir(256)
			for _, d := range entries {
				if err := r.Context().Err(); err != nil {
					return err
				}
				if strings.HasPrefix(d.Name(), ".toolbox-") || d.Type()&os.ModeSymlink != 0 {
					continue
				}
				child := filepath.Join(dir, d.Name())
				if !toolbox.BrowserReadableChild(child) {
					continue
				}
				display := child
				if scope != nil {
					rel, err := filepath.Rel(scope.Root, child)
					if err != nil {
						continue
					}
					display = filepath.ToSlash(rel)
					if _, err = scope.Resolve(display, false); err != nil {
						continue
					}
				}
				if recursive && d.IsDir() {
					if err := scan(child, depth+1); err != nil {
						return err
					}
				}
				if needle != "" && !strings.Contains(strings.ToLower(d.Name()), needle) {
					continue
				}
				st, err := d.Info()
				if err != nil || (!st.IsDir() && !st.Mode().IsRegular()) || dirs && !st.IsDir() {
					continue
				}
				items = append(items, browserEntry{Name: d.Name(), Path: display, Dir: st.IsDir(), Kind: browserKind(d.Name()), Size: st.Size(), Modified: st.ModTime()})
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				return readErr
			}
		}
		return nil
	}
	if err = scan(path, 0); err != nil {
		return nil, err
	}
	b.listingMu.Lock()
	if epoch != b.listingEpoch {
		b.listingMu.Unlock()
		return items, nil
	}
	if b.listings == nil {
		b.listings = map[string]directoryListingSnapshot{}
	}
	if len(items) <= 100000 {
		count := len(items)
		for _, v := range b.listings {
			count += len(v.Items)
		}
		if len(b.listings) >= 8 || count > 200000 {
			b.listings = map[string]directoryListingSnapshot{}
		}
		b.listings[key] = directoryListingSnapshot{append([]browserEntry(nil), items...), time.Now(), info.ModTime()}
	}
	b.listingMu.Unlock()
	return items, nil
}

// Filesystem directory timestamps may remain unchanged after rapid mutations.
// Advance the epoch so an older in-flight scan cannot repopulate stale entries.
func (b *fileBrowser) invalidateDirectoryListings() {
	b.listingMu.Lock()
	b.listingEpoch++
	b.listings = nil
	b.listingMu.Unlock()
}
