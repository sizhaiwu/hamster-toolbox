package toolbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const browserTextHistoryLimit = 20

// historyRoot is supplied by the server, never by a browser request.
func writeBrowserTextHistory(historyRoot, source string, data []byte) (string, error) {
	if !filepath.IsAbs(historyRoot) {
		return "", errors.New("编辑备份目录必须是完整路径；原文件未修改")
	}
	dir := filepath.Join(historyRoot, textDigest([]byte(filepath.Clean(source))))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("无法创建编辑备份，原文件未修改：%w", err)
	}
	// Record the source separately so a backup can be identified after restart.
	meta, err := os.OpenFile(filepath.Join(dir, "source.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err == nil {
		err = json.NewEncoder(meta).Encode(struct{ Path string }{source})
		if ce := meta.Close(); err == nil {
			err = ce
		}
	} else if errors.Is(err, os.ErrExist) {
		err = nil
	}
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, "version-*"+filepath.Ext(source))
	if err != nil {
		return "", err
	}
	name := f.Name()
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	if ce := f.Close(); err == nil {
		err = ce
	}
	if err != nil {
		os.Remove(name)
		return "", err
	}
	return name, nil
}

// Only prune our own regular version files after a successful save. Never
// traverse directories or touch old .toolbox-replaced folders in user resources.
func pruneBrowserTextHistory(dir, latest string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var old []os.FileInfo
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "version-") || filepath.Join(dir, entry.Name()) == latest {
			continue
		}
		if info, e := entry.Info(); e == nil && info.Mode().IsRegular() {
			old = append(old, info)
		}
	}
	sort.Slice(old, func(i, j int) bool {
		if old[i].ModTime().Equal(old[j].ModTime()) {
			return old[i].Name() < old[j].Name()
		}
		return old[i].ModTime().Before(old[j].ModTime())
	})
	for len(old) >= browserTextHistoryLimit {
		if os.Remove(filepath.Join(dir, old[0].Name())) != nil {
			return
		}
		old = old[1:]
	}
}
