package main

// The notes vault is independent of the OS password vault: its portable key is
// derived from a user-held passphrase, never from a bundled or machine-wide key.
import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	qrcode "github.com/skip2/go-qrcode"
	"golang.org/x/crypto/argon2"
)

const notesMaxFile = 64 << 20
const notesMaxText = 1 << 20
const notesIdle = 10 * time.Minute

var errNotesLocked = errors.New("笔记已锁定，请重新解锁")
var errNotesCredentials = errors.New("主密码或验证码不正确")

type privateNote struct {
	ID, Title, Markdown string
	Created, Updated    time.Time
	Revision            int
	Locked              bool              `json:",omitempty"`
	Pinned              bool              `json:",omitempty"`
	Category            string            `json:",omitempty"`
	Tags                []string          `json:",omitempty"`
	Attachments         []notesAttachment `json:",omitempty"`
}
type notesPayload struct {
	ID              string
	Initialized     bool                 `json:",omitempty"`
	AttachmentKey   []byte               `json:",omitempty"`
	History         []notesVersion       `json:",omitempty"`
	Trash           []notesDeleted       `json:",omitempty"`
	BackupSettings  *notesBackupSettings `json:",omitempty"`
	EditorSettings  *notesEditorSettings `json:",omitempty"`
	Notes           []privateNote
	OTP             string
	LastStep        int64
	Recovery        []string // Only salted hashes, inside the authenticated ciphertext.
	AutoLockMinutes *int     `json:",omitempty"` // nil keeps the original 10-minute default.
	FontSize        *int     `json:",omitempty"` // nil keeps the original 14px default.
}
type notesHeader struct {
	Format     string
	Version    int
	KDF        string
	Memory     uint32
	Iterations uint32
	Lanes      uint8
	Salt       []byte
	TwoFA      bool
}
type notesEnvelope struct {
	notesHeader
	Nonce, Ciphertext []byte
}
type notesMemory struct {
	mu            sync.Mutex
	cache         *notesPayload
	cacheHash     [32]byte
	cacheInfo     os.FileInfo
	lastBackup    time.Time
	backupWarning string
	backupTimer   *time.Timer
	backupFlush   func()
	key           []byte
	session       string
	last          time.Time
	idle          time.Duration
	timer         *time.Timer
	pending       string
	pendingAt     time.Time
	failures      int
	nextTry       time.Time
}

func notesRandom(n int) []byte {
	b := make([]byte, n)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return b
}
func notesID() string { return hex.EncodeToString(notesRandom(16)) }
func notesLockMinutes(p notesPayload) int {
	if p.AutoLockMinutes == nil {
		return 10
	}
	return *p.AutoLockMinutes
}
func validNotesLockMinutes(minutes int) bool { return minutes >= 0 && minutes <= 10080 }
func notesFontSize(p notesPayload) int {
	if p.FontSize == nil {
		return 14
	}
	return *p.FontSize
}
func validNotesFontSize(size int) bool { return size >= 10 && size <= 40 }
func newNotesHeader() notesHeader {
	return notesHeader{"hamster-private-notes", 1, "argon2id", 65536, 3, 2, notesRandom(16), false}
}
func validNotesHeader(h notesHeader) bool {
	// Fixed versioned parameters prevent malicious backups requesting unbounded
	// memory/CPU. Change them only through an explicit format migration.
	return h.Format == "hamster-private-notes" && h.Version == 1 && h.KDF == "argon2id" && h.Memory == 65536 && h.Iterations == 3 && h.Lanes == 2 && len(h.Salt) == 16
}
func notesKey(password string, h notesHeader) []byte {
	b := []byte(password)
	defer clear(b)
	return argon2.IDKey(b, h.Salt, h.Iterations, h.Memory, h.Lanes, 32)
}
func parseNotesEnvelope(b []byte) (notesEnvelope, error) {
	var env notesEnvelope
	if len(b) > notesMaxFile || json.Unmarshal(b, &env) != nil || !validNotesHeader(env.notesHeader) || len(env.Nonce) != 12 || len(env.Ciphertext) < 16 {
		return env, errors.New("不是受支持的加密笔记备份，或文件已损坏；原笔记未变")
	}
	return env, nil
}
func notesCipher(key []byte) cipher.AEAD {
	b, e := aes.NewCipher(key)
	if e != nil {
		panic(e)
	}
	g, e := cipher.NewGCM(b)
	if e != nil {
		panic(e)
	}
	return g
}
func openNotes(env notesEnvelope, key []byte) (notesPayload, error) {
	var p notesPayload
	aad, _ := json.Marshal(env.notesHeader)
	plain, e := notesCipher(key).Open(nil, env.Nonce, env.Ciphertext, aad)
	if e != nil {
		return p, errNotesCredentials
	}
	defer clear(plain)
	if len(plain) > 32<<20 || json.Unmarshal(plain, &p) != nil || p.ID == "" || len(p.Notes) > 2000 || (p.OTP != "") != env.TwoFA || !validNotesLockMinutes(notesLockMinutes(p)) || !validNotesFontSize(notesFontSize(p)) {
		return p, errors.New("笔记内容无效，原文件已保留")
	}
	seen := map[string]bool{}
	for _, note := range p.Notes {
		if note.ID == "" || seen[note.ID] || note.Revision < 1 || !validNote(note) {
			return p, errors.New("笔记内容无效，原文件已保留")
		}
		seen[note.ID] = true
	}
	if !validNotesPayloadExtras(p) {
		return p, errors.New("笔记库的历史、回收站或设置无效，原文件已保留")
	}
	if p.Notes == nil {
		p.Notes = []privateNote{}
	}
	return p, nil
}
func sealNotes(h notesHeader, p notesPayload, key []byte) ([]byte, error) {
	plain, e := json.Marshal(p)
	if e != nil {
		return nil, e
	}
	defer clear(plain)
	if len(plain) > 32<<20 || notesLiveSize(p.Notes) > 10<<20 {
		return nil, errors.New("当前笔记内容超过 10 MB 或笔记库总量超过 32 MB，请导出整理")
	}
	h.TwoFA = p.OTP != ""
	aad, _ := json.Marshal(h)
	nonce := notesRandom(12)
	return json.Marshal(notesEnvelope{h, nonce, notesCipher(key).Seal(nil, nonce, plain, aad)})
}
func validNote(n privateNote) bool {
	return utf8.ValidString(n.Title) && utf8.ValidString(n.Markdown) && strings.TrimSpace(n.Title) != "" && utf8.RuneCountInString(n.Title) <= 200 && !strings.ContainsAny(n.Title, "\x00\r\n") && len(n.Markdown) <= notesMaxText && validNotesMetadata(n) && validNotesAttachments(n)
}

// All vault operations use a confined directory and an OS lock shared by
// different application processes. Closing the lock releases it after a crash.
func (s *server) notesDirectory() (*os.Root, func(), error) {
	if e := os.MkdirAll(s.dataDir, 0700); e != nil {
		return nil, nil, e
	}
	parent, e := os.OpenRoot(s.dataDir)
	if e != nil {
		return nil, nil, e
	}
	defer parent.Close()
	if e = parent.Mkdir("private-notes", 0700); e != nil && !os.IsExist(e) {
		return nil, nil, e
	}
	st, e := parent.Lstat("private-notes")
	if e != nil || st.Mode()&os.ModeSymlink != 0 {
		return nil, nil, errors.New("笔记目录不可使用符号链接")
	}
	dir, e := parent.OpenRoot("private-notes")
	if e != nil {
		return nil, nil, e
	}
	if st, e := dir.Lstat("vault.lock"); e == nil && !st.Mode().IsRegular() {
		dir.Close()
		return nil, nil, errors.New("笔记锁文件无效")
	}
	f, e := dir.OpenFile("vault.lock", os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		dir.Close()
		return nil, nil, e
	}
	if e = lockNotesFile(f); e != nil {
		f.Close()
		dir.Close()
		return nil, nil, errors.New("其他工具箱正在保存笔记，请稍后重试")
	}
	return dir, func() { f.Close(); dir.Close() }, nil
}
func readNotesFile(dir *os.Root) ([]byte, error) {
	st, e := dir.Lstat("vault.json")
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() || st.Size() > notesMaxFile {
		return nil, errors.New("笔记文件无效，原文件已保留")
	}
	f, e := dir.Open("vault.json")
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, notesMaxFile+1))
	if len(b) > notesMaxFile {
		return nil, errors.New("笔记文件过大")
	}
	return b, e
}
func writeNotesFile(dir *os.Root, name string, b []byte) error {
	if st, e := dir.Lstat(name); e == nil && !st.Mode().IsRegular() {
		return errors.New("目标笔记文件不能是链接或特殊文件")
	}
	tmp := ".vault-" + notesID()
	f, e := dir.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer dir.Remove(tmp)
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return dir.Rename(tmp, name)
}
func (m *notesMemory) lock() {
	clear(m.key)
	m.key = nil
	if m.cache != nil {
		clear(m.cache.AttachmentKey)
	}
	m.cache = nil
	m.cacheInfo = nil
	m.cacheHash = [32]byte{}
	if m.backupTimer != nil {
		m.backupTimer.Stop()
		m.backupTimer = nil
	}
	if m.backupFlush != nil {
		flush := m.backupFlush
		m.backupFlush = nil
		flush()
	}
	m.session = ""
	m.pending = ""
	m.last = time.Time{}
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
}
func (m *notesMemory) touch() {
	m.last = time.Now()
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	if m.idle == 0 {
		return
	}
	m.timer = time.AfterFunc(m.idle, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.idle > 0 && time.Since(m.last) >= m.idle {
			m.lock()
		}
	})
}
func (m *notesMemory) activate(key []byte, minutes int) string {
	m.lock()
	m.idle = time.Duration(minutes) * time.Minute
	m.key = bytes.Clone(key)
	m.session = notesID()
	m.touch()
	return m.session
}
func (m *notesMemory) authenticated(r *http.Request) bool {
	if m.session != "" && m.idle > 0 && time.Since(m.last) >= m.idle {
		m.lock()
	}
	if m.session == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Notes-Session")), []byte(m.session)) != 1 {
		return false
	}
	return true
}
func (m *notesMemory) failed() {
	m.failures++
	if m.failures >= 5 {
		delay := time.Duration(1<<min(m.failures-5, 8)) * time.Second
		m.nextTry = time.Now().Add(delay)
	}
}

func notesTOTP(secret string, step int64) string {
	key, e := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if e != nil {
		return ""
	}
	defer clear(key)
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(step))
	h := hmac.New(sha1.New, key)
	h.Write(b)
	digest := h.Sum(nil)
	offset := digest[19] & 15
	return fmt.Sprintf("%06d", (binary.BigEndian.Uint32(digest[offset:offset+4])&0x7fffffff)%1000000)
}
func recoveryHash(id, code string) string {
	code = strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(code), "-", ""), " ", ""))
	h := sha256.Sum256([]byte(id + ":" + code))
	return hex.EncodeToString(h[:])
}

// Recovery codes and successful TOTP steps are consumed in the encrypted file
// before granting access. Restarting the program cannot reuse them.
func verifyNotesOTP(p *notesPayload, code string, now time.Time) bool {
	if p.OTP == "" {
		return true
	}
	code = strings.TrimSpace(code)
	for _, delta := range []int64{0, -1, 1} {
		step := now.Unix()/30 + delta
		if step > p.LastStep && len(code) == 6 && hmac.Equal([]byte(code), []byte(notesTOTP(p.OTP, step))) {
			p.LastStep = step
			return true
		}
	}
	hash := recoveryHash(p.ID, code)
	for i, stored := range p.Recovery {
		if hmac.Equal([]byte(hash), []byte(stored)) {
			p.Recovery = append(p.Recovery[:i], p.Recovery[i+1:]...)
			return true
		}
	}
	return false
}
func notesRecovery(p *notesPayload) []string {
	codes := make([]string, 8)
	p.Recovery = nil
	for i := range codes {
		v := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(notesRandom(16))
		codes[i] = v[:6] + "-" + v[6:12] + "-" + v[12:18] + "-" + v[18:]
		p.Recovery = append(p.Recovery, recoveryHash(p.ID, codes[i]))
	}
	return codes
}

type notesRequest struct {
	Password, Code, NewPassword, Action string
	Note                                privateNote
	Notes                               []privateNote
	Backup                              []byte
	Confirm                             bool
	AutoLockMinutes                     *int
	FontSize                            *int
	Locked                              *bool
	VersionID                           string
	Name, AttachmentID                  string
	Content                             []byte
	NoExample                           bool
	BackupSettings                      *notesBackupSettings
	EditorSettings                      *notesEditorSettings
	Draft                               json.RawMessage
}

func (s *server) notesAPI(w http.ResponseWriter, r *http.Request) {
	// All responses, including failures, must be excluded from caches.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	s.notesInitMu.Lock()
	if s.notes == nil {
		s.notes = &notesMemory{idle: notesIdle}
	}
	m := s.notes
	s.notesInitMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	var req notesRequest
	if err := decodeNotesRequest(w, r, &req); err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			http.Error(w, "笔记请求超过大小限制，请减少单次导入或附件数量", http.StatusRequestEntityTooLarge)
		} else {
			fail(w, errors.New("笔记请求格式不正确，请刷新页面后重试；未保存内容请先保留"))
		}
		return
	}
	if len(req.Password) > 1024 || len(req.NewPassword) > 1024 || len(req.Code) > 128 {
		fail(w, errors.New("密码或验证码过长"))
		return
	}
	action := strings.TrimPrefix(r.URL.Path, "/api/notes/")
	if action == "lock" {
		if m.authenticated(r) {
			m.lock()
		}
		send(w, map[string]any{"OK": true, "BackupWarning": m.backupWarning})
		return
	}
	if s.notesFastAction(w, r, m, action, req) {
		return
	}
	defer clear(req.Content)
	dir, closeDir, e := s.notesDirectory()
	if e != nil {
		fail(w, errors.New("无法访问笔记目录，请检查数据目录权限或其他工具箱实例"))
		return
	}
	defer closeDir()
	b, readErr := readNotesFile(dir)
	exists := !os.IsNotExist(readErr)
	var env notesEnvelope
	var damaged error
	if readErr != nil && exists {
		damaged = readErr
	} else if exists {
		env, damaged = parseNotesEnvelope(b)
	}
	if action == "status" {
		send(w, map[string]any{"Exists": exists, "TwoFA": env.TwoFA, "Damaged": damaged != nil})
		return
	}
	if damaged != nil && action != "restore" {
		fail(w, errors.New("笔记文件损坏或无法读取，请从加密备份恢复；原文件已保留"))
		return
	}
	if action == "create" || action == "unlock" || action == "restore" {
		if time.Now().Before(m.nextTry) {
			http.Error(w, "验证过于频繁，请稍后重试", 429)
			return
		}
		if action == "create" {
			if exists {
				fail(w, errors.New("笔记库已存在，请解锁或导入备份"))
				return
			}
			if req.Password == "" {
				fail(w, errors.New("请输入主密码"))
				return
			}
			h := newNotesHeader()
			key := notesKey(req.Password, h)
			defer clear(key)
			p := notesPayload{ID: notesID(), Initialized: true, AttachmentKey: notesRandom(32), Notes: []privateNote{}}
			if !req.NoExample {
				p.Notes = append(p.Notes, notesExample())
			}
			out, err := sealNotes(h, p, key)
			if err == nil {
				err = writeNotesFile(dir, "vault.json", out)
			}
			if err != nil {
				fail(w, errors.New("无法创建笔记库"))
				return
			}
			m.failures = 0
			send(w, s.notesSessionResult(m, key, p, false))
			return
		}
		originalEnv := env
		bundle := notesBackupBundle{}
		if action == "restore" {
			if !req.Confirm {
				fail(w, errors.New("请确认用备份替换当前笔记库"))
				return
			}
			// An existing vault must be unlocked before it can be replaced.
			if exists && damaged == nil && !m.authenticated(r) {
				http.Error(w, errNotesLocked.Error(), 401)
				return
			}
			bundle, e = unpackNotesBackup(req.Backup)
			if e != nil {
				fail(w, e)
				return
			}
			env, e = parseNotesEnvelope(bundle.Vault)
			if e != nil {
				fail(w, e)
				return
			}
		} else if !exists {
			fail(w, errors.New("请先创建笔记库或导入加密备份"))
			return
		}
		key := notesKey(req.Password, env.notesHeader)
		defer clear(key)
		p, err := openNotes(env, key)
		if err != nil || !verifyNotesOTP(&p, req.Code, time.Now()) {
			m.failed()
			fail(w, errNotesCredentials)
			return
		}
		initialized := false
		if !p.Initialized {
			p.Initialized = true
			initialized = true
			if len(p.Notes) < 2000 && notesLiveSize(p.Notes) < (10<<20)-8192 {
				p.Notes = append(p.Notes, notesExample())
			}
		}
		if action == "restore" {
			if err = installNotesBackupAttachments(dir, p, bundle); err != nil {
				fail(w, err)
				return
			}
		}
		if action == "restore" && exists {
			backupName := "before-import.hamster-notes"
			if damaged != nil {
				backupName = "damaged-" + notesID() + ".hamster-notes"
			}
			if readErr != nil {
				fail(w, errors.New("无法保留损坏文件，请检查文件权限后重试"))
				return
			}
			if damaged == nil {
				var original notesPayload
				original, err = openNotes(originalEnv, m.key)
				if err == nil {
					err = writeNotesBackupFile(dir, backupName, dir, original, b)
				}
			} else {
				err = writeNotesFile(dir, backupName, b)
			}
			if err != nil {
				fail(w, errors.New("无法备份当前笔记，导入已取消"))
				return
			}
		}
		if env.TwoFA || action == "restore" || initialized {
			out, err := sealNotes(env.notesHeader, p, key)
			if err == nil {
				err = writeNotesFile(dir, "vault.json", out)
			}
			if err != nil {
				fail(w, errors.New("无法保存笔记库，解锁或导入已取消"))
				return
			}
		}
		m.failures = 0
		m.nextTry = time.Time{}
		send(w, s.notesSessionResult(m, key, p, env.TwoFA))
		return
	}
	if !exists || !m.authenticated(r) {
		http.Error(w, errNotesLocked.Error(), 401)
		return
	}
	p, e := notesCachedPayload(m, env, b)
	if e != nil {
		m.lock()
		http.Error(w, "笔记库已被其他窗口替换或密码已修改，请重新解锁", 401)
		return
	}
	m.cacheInfo, _ = dir.Stat("vault.json")
	// Refresh encrypted preferences before renewing activity, including changes
	// made by another application instance sharing the same data directory.
	m.idle = time.Duration(notesLockMinutes(p)) * time.Minute
	if m.idle > 0 && time.Since(m.last) >= m.idle {
		m.lock()
		http.Error(w, errNotesLocked.Error(), 401)
		return
	}
	m.touch()
	if action == "resume" {
		send(w, notesUnlockedResult(m, p, env.TwoFA))
		return
	}
	if action == "keepalive" {
		send(w, map[string]any{"OK": true, "AutoLockMinutes": notesLockMinutes(p), "FontSize": notesFontSize(p)})
		return
	}
	if action == "list" {
		send(w, map[string]any{"Notes": p.Notes, "TwoFA": env.TwoFA, "AutoLockMinutes": notesLockMinutes(p), "FontSize": notesFontSize(p)})
		return
	}
	if action == "export-file" {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="hamster-notes-backup.hamster-notes"`)
		format := "vault"
		if len(notesAttachmentRefs(p)) > 0 {
			format = "zip"
		}
		w.Header().Set("X-Notes-Backup-Format", format)
		_ = streamNotesBackupFromRoot(w, dir, p, b)
		return
	}
	if action == "export" {
		var backup bytes.Buffer
		if err := streamNotesBackupFromRoot(&backup, dir, p, b); err != nil {
			fail(w, err)
			return
		}
		send(w, map[string]any{"Backup": backup.Bytes()})
		return
	}
	if action == "preview" {
		size := notesFontSize(p)
		if req.FontSize != nil {
			size = *req.FontSize
		}
		html, err := notesMarkdownSized(req.Note.Markdown, size)
		if err != nil {
			fail(w, err)
			return
		}
		send(w, map[string]string{"HTML": html})
		return
	}
	if handled, result, err := s.notesAttachmentAction(dir, &p, action, req); handled {
		if err != nil {
			fail(w, err)
			return
		}
		if action != "attachment-read" {
			if err = s.commitNotes(dir, m, env.notesHeader, p, m.key); err != nil {
				fail(w, err)
				return
			}
		}
		if value, ok := result.(map[string]any); ok {
			value["BackupWarning"] = m.backupWarning
			if content, ok := value["Content"].([]byte); ok {
				defer clear(content)
			}
		}
		send(w, result)
		return
	}
	if action == "backup-status" || action == "backup-now" {
		if action == "backup-now" {
			if err := s.notesAutomaticBackup(p, b, notesBackupConfig(p)); err != nil {
				fail(w, err)
				return
			}
			m.lastBackup = time.Now()
			m.backupWarning = ""
		}
		send(w, s.notesBackupStatus(p, m))
		return
	}
	if handled, changed, result, err := notesFeatureAction(&p, action, req); handled {
		if err != nil {
			fail(w, err)
			return
		}
		if changed {
			if err = s.commitNotes(dir, m, env.notesHeader, p, m.key); err != nil {
				fail(w, err)
				return
			}
		}
		if resultMap, ok := result.(map[string]any); ok {
			resultMap["BackupWarning"] = m.backupWarning
		}
		send(w, result)
		return
	}
	var result any = map[string]any{"OK": true}
	key := m.key
	switch action {
	case "font-size":
		if req.FontSize == nil || !validNotesFontSize(*req.FontSize) {
			fail(w, errors.New("字号请选择 10–40px"))
			return
		}
		p.FontSize = req.FontSize
		result = map[string]any{"OK": true, "FontSize": notesFontSize(p)}
	case "note-lock":
		if req.Locked == nil {
			fail(w, errors.New("请选择锁定编辑或解锁编辑"))
			return
		}
		at := -1
		for i, n := range p.Notes {
			if n.ID == req.Note.ID {
				at = i
				break
			}
		}
		if at < 0 {
			fail(w, errors.New("笔记不存在"))
			return
		}
		if p.Notes[at].Revision != req.Note.Revision {
			fail(w, errors.New("笔记已更新，请刷新后再修改编辑锁定状态"))
			return
		}
		if p.Notes[at].Locked != *req.Locked {
			p.Notes[at].Locked = *req.Locked
			p.Notes[at].Revision++
		}
		result = map[string]any{"Note": p.Notes[at]}
	case "auto-lock":
		if req.AutoLockMinutes == nil || !validNotesLockMinutes(*req.AutoLockMinutes) {
			fail(w, errors.New("自动锁定时长请选择 1–10080 分钟，或选择不自动锁定"))
			return
		}
		p.AutoLockMinutes = req.AutoLockMinutes
		result = map[string]any{"OK": true, "AutoLockMinutes": notesLockMinutes(p)}
	case "save":
		n := req.Note
		n.Title = strings.TrimSpace(n.Title)
		if !validNote(n) {
			fail(w, errors.New("请填写 1–200 字标题，正文最多 1 MB"))
			return
		}
		at := -1
		for i, item := range p.Notes {
			if item.ID == n.ID {
				at = i
				break
			}
		}
		if n.ID != "" && at < 0 {
			fail(w, errors.New("此笔记已删除，请刷新列表"))
			return
		}
		if at >= 0 && p.Notes[at].Locked {
			fail(w, errors.New("笔记已锁定编辑，请先解锁编辑"))
			return
		}
		if at >= 0 && n.Revision != p.Notes[at].Revision {
			fail(w, errors.New("此笔记已被其他页面修改，请先刷新列表，避免覆盖"))
			return
		}
		n.Updated = time.Now().UTC()
		n.Locked = false // Ordinary saves cannot bypass or change the edit lock.
		n.Revision++
		if at >= 0 {
			n.Created = p.Notes[at].Created
			if n.Title != p.Notes[at].Title || n.Markdown != p.Notes[at].Markdown {
				notesAddVersion(&p, p.Notes[at], false)
			}
			p.Notes[at] = n
		} else {
			if len(p.Notes) >= 2000 {
				fail(w, errors.New("最多 2000 篇笔记"))
				return
			}
			n.ID = notesID()
			n.Created = n.Updated
			p.Notes = append(p.Notes, n)
		}
		result = map[string]any{"Note": n}
	case "delete":
		if !req.Confirm {
			fail(w, errors.New("请确认删除笔记"))
			return
		}
		found := false
		for i, n := range p.Notes {
			if n.ID == req.Note.ID {
				if n.Locked {
					fail(w, errors.New("笔记已锁定编辑，请先解锁编辑再删除"))
					return
				}
				if n.Revision != req.Note.Revision {
					fail(w, errors.New("笔记已更新，请刷新后再删除"))
					return
				}
				p.Trash = append(p.Trash, notesDeleted{Note: n, Deleted: time.Now().UTC()})
				notesTrimArchives(&p)
				p.Notes = append(p.Notes[:i], p.Notes[i+1:]...)
				found = true
				break
			}
		}
		if !found {
			fail(w, errors.New("笔记不存在"))
			return
		}
	case "import-markdown":
		if len(req.Notes) == 0 || len(req.Notes) > 200 || len(p.Notes)+len(req.Notes) > 2000 {
			fail(w, errors.New("每次导入 1–200 篇笔记，总数最多 2000 篇"))
			return
		}
		imported := []privateNote{}
		for _, n := range req.Notes {
			n.Title = strings.TrimSpace(n.Title)
			if !validNote(n) {
				fail(w, errors.New("导入的标题或正文不符合大小限制"))
				return
			}
			n.ID = notesID()
			n.Locked = false
			n.Revision = 1
			n.Created = time.Now().UTC()
			n.Updated = n.Created
			p.Notes = append(p.Notes, n)
			imported = append(imported, n)
		}
		result = map[string]any{"Notes": imported}
	case "2fa-start":
		if p.OTP != "" {
			fail(w, errors.New("验证器已开启"))
			return
		}
		m.pending = base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(notesRandom(20))
		m.pendingAt = time.Now()
		u := "otpauth://totp/" + url.PathEscape("Hamster Toolbox:Private Notes") + "?secret=" + m.pending + "&issuer=Hamster%20Toolbox&algorithm=SHA1&digits=6&period=30"
		png, err := qrcode.Encode(u, qrcode.Medium, 256)
		if err != nil {
			fail(w, errors.New("无法生成验证器二维码"))
			return
		}
		send(w, map[string]any{"Secret": m.pending, "QR": png})
		return
	case "2fa-enable":
		if time.Now().Before(m.nextTry) {
			http.Error(w, "验证过于频繁，请稍后重试", 429)
			return
		}
		if p.OTP != "" || m.pending == "" || time.Since(m.pendingAt) > 5*time.Minute {
			fail(w, errors.New("设置已过期，请重新生成二维码"))
			return
		}
		p.OTP = m.pending
		p.LastStep = -1
		if !verifyNotesOTP(&p, req.Code, time.Now()) {
			m.failed()
			fail(w, errors.New("验证码不正确，请检查验证器时间"))
			return
		}
		result = map[string]any{"RecoveryCodes": notesRecovery(&p)}
	case "2fa-disable", "recovery-renew", "password":
		if time.Now().Before(m.nextTry) {
			http.Error(w, "验证过于频繁，请稍后重试", 429)
			return
		}
		check := notesKey(req.Password, env.notesHeader)
		ok := hmac.Equal(check, m.key)
		clear(check)
		if !ok || !verifyNotesOTP(&p, req.Code, time.Now()) {
			m.failed()
			fail(w, errNotesCredentials)
			return
		}
		if action == "password" {
			if req.NewPassword == "" {
				fail(w, errors.New("请输入新主密码"))
				return
			}
			env.notesHeader = newNotesHeader()
			key = notesKey(req.NewPassword, env.notesHeader)
			defer clear(key)
		} else if action == "2fa-disable" {
			p.OTP = ""
			p.Recovery = nil
			p.LastStep = 0
		} else {
			if p.OTP == "" {
				fail(w, errors.New("请先开启验证器"))
				return
			}
			result = map[string]any{"RecoveryCodes": notesRecovery(&p)}
		}
	default:
		http.NotFound(w, r)
		return
	}
	if action == "password" {
		m.lastBackup = time.Time{}
	}
	e = s.commitNotes(dir, m, env.notesHeader, p, key)
	if e != nil {
		fail(w, e)
		return
	}
	if resultMap, ok := result.(map[string]any); ok {
		resultMap["BackupWarning"] = m.backupWarning
	}
	if action == "password" {
		clear(m.key)
		m.key = bytes.Clone(key)
	}
	if action == "2fa-enable" {
		m.pending = ""
	}
	if action == "auto-lock" {
		m.idle = time.Duration(notesLockMinutes(p)) * time.Minute
		m.touch()
	}
	send(w, result)
}

// Only these embedded assets exist under /notes/. The page is owner-only and
// the guest sharing listener never dispatches this handler.
func (s *server) notesPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	styleHash := sha256.Sum256([]byte(notesPreviewStyle))
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self' 'sha256-"+notesPreviewScriptHash()+"'; style-src 'self' 'sha256-"+base64.StdEncoding.EncodeToString(styleHash[:])+"'; img-src 'self' data: blob:; connect-src 'self'; frame-src 'self'; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, "GET required", 405)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/notes/")
	if name == "" {
		name = "notes.html"
	}
	if name != "notes.html" && name != "notes.js" && name != "notes-general.js" && name != "notes.css" && name != "notes-rich.js" && name != "notes-prose.css" && name != "notes-syntax.js" && name != "notes-layout.js" {
		http.NotFound(w, r)
		return
	}
	if name == "notes.html" && r.URL.Query().Get("session") != s.token {
		http.Error(w, "请从工具箱打开私密笔记", 403)
		return
	}
	b, e := assets.ReadFile("web/" + name)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	kind := "text/css"
	if strings.HasSuffix(name, ".js") {
		kind = "text/javascript"
	}
	if strings.HasSuffix(name, ".html") {
		b = sharedSettingsHTML(b)
		kind = "text/html"
	}
	w.Header().Set("Content-Type", kind+"; charset=utf-8")
	if r.Method != "HEAD" {
		w.Write(b)
	}
}
