package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type diskThumbnailEntry struct {
	Size int64
	At   time.Time
}

func (b *fileBrowser) loadThumbnailIndex(dir string) {
	if b.diskThumbIndex != nil {
		return
	}
	b.diskThumbIndex = map[string]diskThumbnailEntry{}
	b.diskThumbBytes = 0
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if len(entry.Name()) != 68 || !strings.HasSuffix(entry.Name(), ".jpg") {
			continue
		}
		st, err := entry.Info()
		if err != nil || !st.Mode().IsRegular() {
			continue
		}
		b.diskThumbIndex[entry.Name()] = diskThumbnailEntry{st.Size(), st.ModTime()}
		b.diskThumbBytes += st.Size()
	}
}
func (b *fileBrowser) trimThumbnailIndex(dir string, limit int64) {
	for b.diskThumbBytes > limit || len(b.diskThumbIndex) > 10000 {
		var oldest string
		var at time.Time
		for name, entry := range b.diskThumbIndex {
			if oldest == "" || entry.At.Before(at) {
				oldest, at = name, entry.At
			}
		}
		if oldest == "" {
			break
		}
		entry := b.diskThumbIndex[oldest]
		if err := os.Remove(filepath.Join(dir, oldest)); err != nil && !os.IsNotExist(err) {
			break
		}
		delete(b.diskThumbIndex, oldest)
		b.diskThumbBytes -= entry.Size
	}
}
func (b *fileBrowser) thumbnailCacheLimit() int64 {
	p, err := (&server{dataDir: b.dataDir}).loadPreferences()
	if err != nil {
		return 0
	}
	if p.ThumbnailCacheMB == nil {
		return 256 << 20
	}
	return int64(*p.ThumbnailCacheMB) << 20
}
func (b *fileBrowser) thumbnailCacheDirectory() (string, error) {
	dir := filepath.Join(b.dataDir, "thumbnails")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	st, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return "", os.ErrPermission
	}
	return dir, nil
}
func diskThumbnailName(key string) string {
	sum := sha256.Sum256([]byte("thumbnail-v1\x00" + key))
	return hex.EncodeToString(sum[:]) + ".jpg"
}
func (b *fileBrowser) diskThumbnail(key string) ([]byte, bool) {
	if b.thumbnailCacheLimit() == 0 {
		return nil, false
	}
	b.diskCacheMu.Lock()
	defer b.diskCacheMu.Unlock()
	dir, err := b.thumbnailCacheDirectory()
	if err != nil {
		return nil, false
	}
	path := filepath.Join(dir, diskThumbnailName(key))
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() || st.Size() > 2<<20 {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) < 3 || data[0] != 0xff || data[1] != 0xd8 {
		return nil, false
	}
	os.Chtimes(path, time.Now(), time.Now())
	b.loadThumbnailIndex(dir)
	name := diskThumbnailName(key)
	b.diskThumbIndex[name] = diskThumbnailEntry{st.Size(), time.Now()}
	return data, true
}
func (b *fileBrowser) storeDiskThumbnail(key string, data []byte) {
	limit := b.thumbnailCacheLimit()
	if limit == 0 || len(data) > 2<<20 {
		return
	}
	b.diskCacheMu.Lock()
	defer b.diskCacheMu.Unlock()
	dir, err := b.thumbnailCacheDirectory()
	if err != nil {
		return
	}
	b.loadThumbnailIndex(dir)
	f, err := os.CreateTemp(dir, ".thumb-*")
	if err != nil {
		return
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	ce := f.Close()
	if err != nil || ce != nil {
		return
	}
	if err = os.Rename(f.Name(), filepath.Join(dir, diskThumbnailName(key))); err != nil {
		return
	}
	b.loadThumbnailIndex(dir)
	name := diskThumbnailName(key)
	if previous, ok := b.diskThumbIndex[name]; ok {
		b.diskThumbBytes -= previous.Size
	}
	b.diskThumbIndex[name] = diskThumbnailEntry{int64(len(data)), time.Now()}
	b.diskThumbBytes += int64(len(data))
	b.trimThumbnailIndex(dir, limit)
}
func (b *fileBrowser) clearDiskThumbnails() error {
	b.diskCacheMu.Lock()
	defer b.diskCacheMu.Unlock()
	dir, err := b.thumbnailCacheDirectory()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if len(entry.Name()) == 68 && strings.HasSuffix(entry.Name(), ".jpg") {
			if err = os.Remove(filepath.Join(dir, entry.Name())); err != nil {
				return err
			}
		}
	}
	b.diskThumbIndex = nil
	b.diskThumbBytes = 0
	b.mediaState().thumbnails.clear()
	return nil
}
