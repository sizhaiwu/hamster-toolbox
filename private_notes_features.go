package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type notesVersion struct {
	ID   string
	Note privateNote
	At   time.Time
}
type notesDeleted struct {
	Note    privateNote
	Deleted time.Time
}
type notesBackupSettings struct {
	Enabled               bool
	Directory             string
	IntervalMinutes, Keep int
}
type notesEditorSettings struct {
	Mode, Sort, Category, View string
	Positions                  map[string]notesPosition `json:",omitempty"`
}
type notesPosition struct {
	Cursor int
	Scroll float64
}

func notesBackupConfig(p notesPayload) notesBackupSettings {
	if p.BackupSettings != nil {
		return *p.BackupSettings
	}
	return notesBackupSettings{Enabled: true, IntervalMinutes: 30, Keep: 10}
}
func validNotesMetadata(n privateNote) bool {
	if !utf8.ValidString(n.Category) || utf8.RuneCountInString(n.Category) > 60 || len(n.Tags) > 12 || strings.ContainsAny(n.Category, "\x00\r\n") {
		return false
	}
	for _, tag := range n.Tags {
		if !utf8.ValidString(tag) || utf8.RuneCountInString(tag) > 30 || strings.ContainsAny(tag, "\x00\r\n") {
			return false
		}
	}
	return true
}
func notesLiveSize(notes []privateNote) int { b, _ := json.Marshal(notes); return len(b) }
func notesTrimArchives(p *notesPayload) {
	for len(p.History) > 200 {
		p.History = p.History[1:]
	}
	for len(p.Trash) > 200 {
		p.Trash = p.Trash[1:]
	}
	for len(p.History) > 0 {
		b, _ := json.Marshal(p.History)
		if len(b) <= 8<<20 {
			break
		}
		p.History = p.History[1:]
	}
	for len(p.Trash) > 0 {
		b, _ := json.Marshal(p.Trash)
		if len(b) <= 8<<20 {
			break
		}
		p.Trash = p.Trash[1:]
	}
}
func notesAddVersion(p *notesPayload, n privateNote, force bool) {
	count := 0
	for i := len(p.History) - 1; i >= 0; i-- {
		if p.History[i].Note.ID == n.ID {
			count++
			if count == 1 && !force && time.Since(p.History[i].At) < time.Minute {
				return
			}
		}
	}
	if count >= 20 {
		for i, v := range p.History {
			if v.Note.ID == n.ID {
				p.History = append(p.History[:i], p.History[i+1:]...)
				break
			}
		}
	}
	p.History = append(p.History, notesVersion{ID: notesID(), Note: n, At: time.Now().UTC()})
	notesTrimArchives(p)
}
func cloneNotesPayload(p notesPayload) notesPayload {
	p.Notes = append([]privateNote(nil), p.Notes...)
	p.History = append([]notesVersion(nil), p.History...)
	p.Trash = append([]notesDeleted(nil), p.Trash...)
	p.Recovery = append([]string(nil), p.Recovery...)
	return p
}
func notesCachedPayload(m *notesMemory, env notesEnvelope, b []byte) (notesPayload, error) {
	sum := sha256.Sum256(b)
	if m.cache != nil && sum == m.cacheHash {
		return cloneNotesPayload(*m.cache), nil
	}
	p, e := openNotes(env, m.key)
	if e == nil {
		copy := cloneNotesPayload(p)
		m.cache = &copy
		m.cacheHash = sum
	}
	return p, e
}
func (s *server) notesSessionResult(m *notesMemory, key []byte, p notesPayload, twoFA bool) map[string]any {
	m.activate(key, notesLockMinutes(p))
	cached := cloneNotesPayload(p)
	m.cache = &cached
	return notesUnlockedResult(m, p, twoFA)
}

func notesUnlockedResult(m *notesMemory, p notesPayload, twoFA bool) map[string]any {
	return map[string]any{"Session": m.session, "Notes": p.Notes, "TwoFA": twoFA, "AutoLockMinutes": notesLockMinutes(p), "FontSize": notesFontSize(p), "VaultID": p.ID, "EditorSettings": p.EditorSettings, "BackupSettings": notesBackupConfig(p)}
}

// Independent lock/draft actions remain usable when the main vault cannot be saved.
// Only opaque, browser-encrypted draft envelopes are accepted here.
func (s *server) notesFastAction(w http.ResponseWriter, r *http.Request, m *notesMemory, action string, req notesRequest) bool {
	if action != "draft-key" && action != "draft-get" && action != "draft-put" && action != "draft-clear" && action != "preview" && action != "keepalive" {
		return false
	}
	if !m.authenticated(r) {
		http.Error(w, errNotesLocked.Error(), 401)
		return true
	}
	dir, closeDir, e := s.notesDirectory()
	if e != nil {
		fail(w, e)
		return true
	}
	defer closeDir()
	if action == "draft-key" {
		var request struct{ Salt string }
		if json.Unmarshal(req.Draft, &request) != nil || m.cache == nil {
			fail(w, errors.New("草稿密钥请求无效"))
			return true
		}
		salt, err := base64.StdEncoding.DecodeString(request.Salt)
		if err != nil || len(salt) != 16 {
			fail(w, errors.New("草稿密钥请求无效"))
			return true
		}
		mac := hmac.New(sha256.New, m.key)
		mac.Write([]byte("hamster-notes-draft/v2/" + m.cache.ID + "/"))
		mac.Write(salt)
		key := mac.Sum(nil)
		defer clear(key)
		m.touch()
		send(w, map[string]any{"Key": key})
		return true
	}
	if strings.HasPrefix(action, "draft-") {
		if action == "draft-put" {
			var envelope encryptedNotesDraft
			_ = json.Unmarshal(req.Draft, &envelope)
			if len(req.Draft) > 3<<20 || !validEncryptedNotesDraft(req.Draft) || m.cache == nil || envelope.VaultID != m.cache.ID {
				fail(w, errors.New("加密草稿无效或过大"))
				return true
			}
			e = writeNotesFile(dir, s.notesDraftFile(), req.Draft)
		} else if action == "draft-clear" {
			e = dir.Remove(s.notesDraftFile())
			if os.IsNotExist(e) {
				e = nil
			}
		}
		if e != nil {
			fail(w, errors.New("加密草稿未能写入本机，浏览器副本仍将保留"))
			return true
		}
		if action == "draft-get" {
			var draft json.RawMessage
			st, err := dir.Lstat(s.notesDraftFile())
			if err == nil && st.Mode().IsRegular() && st.Size() <= 3<<20 {
				b, err := dir.ReadFile(s.notesDraftFile())
				if err == nil && validEncryptedNotesDraft(b) {
					draft = b
				}
			}
			send(w, map[string]any{"Draft": draft})
			return true
		}
		m.touch()
		send(w, map[string]bool{"OK": true})
		return true
	}
	// Fast preview/keepalive: a vault replaced by another process invalidates the
	// metadata cache. Mutations still read and authenticate the entire file.
	st, e := dir.Stat("vault.json")
	if e != nil || m.cache == nil || m.cacheInfo == nil || !os.SameFile(st, m.cacheInfo) || !st.ModTime().Equal(m.cacheInfo.ModTime()) || st.Size() != m.cacheInfo.Size() {
		return false
	}
	p := m.cache
	m.idle = time.Duration(notesLockMinutes(*p)) * time.Minute
	if m.idle > 0 && time.Since(m.last) >= m.idle {
		m.lock()
		http.Error(w, errNotesLocked.Error(), 401)
		return true
	}
	m.touch()
	if action == "keepalive" {
		send(w, map[string]any{"OK": true, "AutoLockMinutes": notesLockMinutes(*p), "FontSize": notesFontSize(*p)})
		return true
	}
	size := notesFontSize(*p)
	if req.FontSize != nil {
		size = *req.FontSize
	}
	html, err := notesMarkdownSized(req.Note.Markdown, size)
	if err != nil {
		fail(w, err)
	} else {
		send(w, map[string]string{"HTML": html})
	}
	return true
}

type encryptedNotesDraft struct {
	Version                       int
	VaultID, Salt, IV, Ciphertext string
}

func validEncryptedNotesDraft(b []byte) bool {
	var d encryptedNotesDraft
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&d) != nil || decoder.Decode(new(any)) != io.EOF || d.Version != 2 || !validNotesID(d.VaultID) {
		return false
	}
	salt, e := base64.StdEncoding.DecodeString(d.Salt)
	if e != nil || len(salt) != 16 {
		return false
	}
	iv, e := base64.StdEncoding.DecodeString(d.IV)
	if e != nil || len(iv) != 12 {
		return false
	}
	cipher, e := base64.StdEncoding.DecodeString(d.Ciphertext)
	return e == nil && len(cipher) >= 16 && len(cipher) <= 2<<20
}
func validNotesID(id string) bool { b, e := hex.DecodeString(id); return e == nil && len(b) == 16 }
func validNotesEditorSettings(e *notesEditorSettings) bool {
	if e == nil {
		return true
	}
	if (e.View != "" && !notesOneOf(e.View, "edit", "split", "preview")) || !notesOneOf(e.Mode, "markdown", "rich") || !notesOneOf(e.Sort, "updated", "created", "name") || utf8.RuneCountInString(e.Category) > 60 || len(e.Positions) > 2000 {
		return false
	}
	for id, p := range e.Positions {
		if !validNotesID(id) || p.Cursor < 0 || p.Cursor > notesMaxText || p.Scroll < 0 || p.Scroll > 100000000 {
			return false
		}
	}
	return true
}
func validNotesPayloadExtras(p notesPayload) bool {
	refs := notesAttachmentRefs(p)
	var attachmentSize int64
	for _, a := range refs {
		attachmentSize += a.Size
	}
	if len(refs) > 2000 || attachmentSize > notesMaxBackup-(64<<20) {
		return false
	}
	if (len(notesAttachmentRefs(p)) > 0 && len(p.AttachmentKey) != 32) || !validNotesID(p.ID) || (len(p.AttachmentKey) != 0 && len(p.AttachmentKey) != 32) || len(p.History) > 200 || len(p.Trash) > 200 || !validNotesEditorSettings(p.EditorSettings) {
		return false
	}
	if p.BackupSettings != nil {
		b := p.BackupSettings
		if b.IntervalMinutes < 1 || b.IntervalMinutes > 10080 || b.Keep < 2 || b.Keep > 50 || len(b.Directory) > 4096 || strings.ContainsRune(b.Directory, 0) {
			return false
		}
	}
	for _, v := range p.History {
		if !validNotesID(v.ID) || !validNote(v.Note) || v.Note.Revision < 1 {
			return false
		}
	}
	seen := map[string]bool{}
	for _, d := range p.Trash {
		if !validNotesID(d.Note.ID) || !validNote(d.Note) || d.Note.Revision < 1 || seen[d.Note.ID] {
			return false
		}
		seen[d.Note.ID] = true
		for _, n := range p.Notes {
			if n.ID == d.Note.ID {
				return false
			}
		}
	}
	return true
}

func notesFeatureAction(p *notesPayload, action string, req notesRequest) (handled, changed bool, result any, err error) {
	handled = true
	at := -1
	for i, n := range p.Notes {
		if n.ID == req.Note.ID {
			at = i
			break
		}
	}
	switch action {
	case "history":
		result = map[string]any{"Versions": []map[string]any{}}
		list := []map[string]any{}
		for i := len(p.History) - 1; i >= 0; i-- {
			v := p.History[i]
			if v.Note.ID == req.Note.ID {
				list = append(list, map[string]any{"ID": v.ID, "At": v.At, "Title": v.Note.Title})
			}
		}
		result = map[string]any{"Versions": list}
	case "history-read":
		for _, v := range p.History {
			if v.ID == req.VersionID && v.Note.ID == req.Note.ID {
				return true, false, map[string]any{"Version": v}, nil
			}
		}
		err = errors.New("历史版本不存在，可能已按保留数量清理")
	case "snapshot", "history-restore":
		if at < 0 {
			err = errors.New("笔记不存在")
			break
		}
		n := p.Notes[at]
		if n.Locked || n.Revision != req.Note.Revision {
			err = errors.New("笔记已锁定编辑或已更新，请重新选择后操作")
			break
		}
		if action == "snapshot" {
			notesAddVersion(p, n, true)
			changed = true
			result = map[string]bool{"OK": true}
			break
		}
		if !req.Confirm {
			err = errors.New("请确认恢复历史版本")
			break
		}
		found := false
		for _, v := range p.History {
			if v.ID == req.VersionID && v.Note.ID == n.ID {
				notesAddVersion(p, n, true)
				old := v.Note
				old.Revision = n.Revision + 1
				old.Created = n.Created
				old.Updated = time.Now().UTC()
				old.Locked = false
				p.Notes[at] = old
				found = true
				changed = true
				result = map[string]any{"Note": old}
				break
			}
		}
		if !found {
			err = errors.New("历史版本不存在")
		}
	case "trash":
		list := []map[string]any{}
		for i := len(p.Trash) - 1; i >= 0; i-- {
			d := p.Trash[i]
			list = append(list, map[string]any{"ID": d.Note.ID, "Title": d.Note.Title, "Deleted": d.Deleted})
		}
		result = map[string]any{"Trash": list}
	case "trash-restore", "trash-delete":
		if !req.Confirm {
			err = errors.New("请确认回收站操作")
			break
		}
		idx := -1
		for i, d := range p.Trash {
			if d.Note.ID == req.Note.ID {
				idx = i
				break
			}
		}
		if idx < 0 {
			err = errors.New("回收站笔记不存在")
			break
		}
		if action == "trash-restore" {
			if len(p.Notes) >= 2000 {
				err = errors.New("最多 2000 篇笔记")
				break
			}
			n := p.Trash[idx].Note
			n.Revision++
			n.Updated = time.Now().UTC()
			p.Notes = append(p.Notes, n)
			result = map[string]any{"Note": n}
		} else {
			history := p.History[:0]
			for _, v := range p.History {
				if v.Note.ID != req.Note.ID {
					history = append(history, v)
				}
			}
			p.History = history
			result = map[string]bool{"OK": true}
		}
		p.Trash = append(p.Trash[:idx], p.Trash[idx+1:]...)
		changed = true
	case "backup-settings":
		b := req.BackupSettings
		if b == nil || b.IntervalMinutes < 1 || b.IntervalMinutes > 10080 || b.Keep < 2 || b.Keep > 50 || len(b.Directory) > 4096 || strings.ContainsRune(b.Directory, 0) {
			err = errors.New("备份间隔为 1–10080 分钟，保留 2–50 份")
			break
		}
		if b.Directory != "" {
			abs, e := filepath.Abs(b.Directory)
			if e != nil {
				err = e
				break
			}
			copy := *b
			copy.Directory = abs
			b = &copy
		}
		p.BackupSettings = b
		changed = true
		result = map[string]any{"BackupSettings": *b}
	case "editor-settings":
		e := req.EditorSettings
		if e == nil || !validNotesEditorSettings(e) {
			err = errors.New("编辑设置无效")
			break
		}
		p.EditorSettings = e
		changed = true
		result = map[string]bool{"OK": true}
	case "backup-status", "backup-now":
		// Handled by the caller, because backups need a confined filesystem root.
		return false, false, nil, nil
	default:
		return false, false, nil, nil
	}
	return
}
func notesOneOf(s string, values ...string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}

func (s *server) commitNotes(dir *os.Root, m *notesMemory, h notesHeader, p notesPayload, key []byte) error {
	notesTrimArchives(&p)
	out, e := sealNotes(h, p, key)
	if e != nil {
		return e
	}
	if e = writeNotesFile(dir, "vault.json", out); e != nil {
		return errors.New("笔记保存失败，请检查磁盘空间与数据目录权限；原文件已保留")
	}
	cached := cloneNotesPayload(p)
	m.cache = &cached
	m.cacheHash = sha256.Sum256(out)
	m.cacheInfo, _ = dir.Stat("vault.json")
	m.backupWarning = ""
	config := notesBackupConfig(p)
	if m.backupTimer != nil {
		m.backupTimer.Stop()
		m.backupTimer = nil
	}
	m.backupFlush = nil
	if config.Enabled {
		backupPayload := notesPayload{ID: p.ID}
		for id, ref := range notesAttachmentRefs(p) {
			backupPayload.Notes = append(backupPayload.Notes, privateNote{Attachments: []notesAttachment{{ID: id, Size: ref.Size, Hash: ref.Hash}}})
		}
		m.backupFlush = func() {
			if err := s.notesAutomaticBackup(backupPayload, out, config); err != nil {
				m.backupWarning = "笔记已保存，但自动备份失败：" + err.Error()
			} else {
				m.lastBackup = time.Now()
				m.backupWarning = ""
			}
		}
		remaining := time.Duration(config.IntervalMinutes)*time.Minute - time.Since(m.lastBackup)
		if m.lastBackup.IsZero() || remaining <= 0 {
			job := m.backupFlush
			m.backupFlush = nil
			job()
		} else {
			m.backupTimer = time.AfterFunc(remaining, func() {
				m.mu.Lock()
				defer m.mu.Unlock()
				m.backupTimer = nil
				if m.backupFlush != nil {
					job := m.backupFlush
					m.backupFlush = nil
					job()
				}
			})
		}
	}
	s.notesCleanAttachments(dir, p)
	return nil
}
func (s *server) notesBackupDirectory(config notesBackupSettings) string {
	if config.Directory != "" {
		return config.Directory
	}
	return filepath.Join(s.dataDir, "private-notes", "auto-backups")
}
func (s *server) notesAutomaticBackup(p notesPayload, b []byte, config notesBackupSettings) error {
	path := s.notesBackupDirectory(config)
	if e := os.MkdirAll(path, 0700); e != nil {
		return e
	}
	st, e := os.Lstat(path)
	if e != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return errors.New("备份目录无效或为链接")
	}
	dir, e := os.OpenRoot(path)
	if e != nil {
		return e
	}
	defer dir.Close()
	prefix := "hamster-notes-auto-" + p.ID + "-"
	name := prefix + time.Now().UTC().Format("20060102T150405.000000000") + ".hamster-notes"
	source, e := os.OpenRoot(filepath.Join(s.dataDir, "private-notes"))
	if e != nil {
		return e
	}
	defer source.Close()
	if e = writeNotesBackupFile(dir, name, source, p, b); e != nil {
		return e
	}
	files, e := os.ReadDir(path)
	if e != nil {
		return e
	}
	names := []string{}
	for _, f := range files {
		n := f.Name()
		if f.Type().IsRegular() && strings.HasPrefix(n, prefix) && strings.HasSuffix(n, ".hamster-notes") {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for len(names) > config.Keep {
		if e = dir.Remove(names[0]); e != nil {
			return e
		}
		names = names[1:]
	}
	return nil
}
func (s *server) notesBackupStatus(p notesPayload, m *notesMemory) map[string]any {
	config := notesBackupConfig(p)
	path := s.notesBackupDirectory(config)
	var latest time.Time
	count := 0
	entries, _ := os.ReadDir(path)
	for _, f := range entries {
		if f.Type().IsRegular() && strings.HasPrefix(f.Name(), "hamster-notes-auto-"+p.ID+"-") && strings.HasSuffix(f.Name(), ".hamster-notes") {
			count++
			if st, e := f.Info(); e == nil && st.ModTime().After(latest) {
				latest = st.ModTime()
			}
		}
	}
	return map[string]any{"Settings": config, "Directory": path, "LastBackup": latest, "Count": count, "Warning": m.backupWarning}
}
