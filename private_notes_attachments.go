package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

const notesMaxAttachment = 25 << 20
const notesMaxBackup = 512 << 20

type notesAttachment struct {
	ID, Name, Mime, Hash string
	Size                 int64
}
type notesBackupBundle struct {
	Vault []byte
	Files map[string]*zip.File
}

func notesExample() privateNote {
	now := time.Now().UTC()
	return privateNote{ID: notesID(), Title: "使用示例 · 仓鼠笔记", Revision: 1, Created: now, Updated: now, Pinned: true, Category: "使用说明", Tags: []string{"示例"}, Markdown: `# 欢迎使用仓鼠笔记

这是一篇可以编辑或删除的示例，不包含真实密码。

## 不会 Markdown 也可以写

选择上方的 **富文本**，直接输入文字；选中文字后点击加粗、标题、列表等按钮即可排版。也可以切换到 Markdown，使用分栏一边编辑一边预览。

- 修改后会自动加密保存，关闭前确认“已加密保存”。
- Ctrl / ⌘ + S 立即保存，Ctrl / ⌘ + F 查找正文。
- 图片、截图和文件可以粘贴、拖入，或点击“附件”选择。

| 功能 | 用法 |
| --- | --- |
| 整理 | 分类、标签和置顶 |
| 历史 | 查看旧版本，或保存当前快照 |
| 回收站 | 找回误删的笔记 |
| 锁定编辑 | 防止误改，仍能查看 |

## 记得备份

在“安全与备份”中设置自动备份，建议选择另一块磁盘。加密备份包含图片和文件，可以迁移到其他电脑。

主密码无法找回。验证器恢复码不能代替主密码；单篇 Markdown 导出是明文，附件下载也是解密后的文件，请妥善保管。
`}
}
func validNotesAttachments(n privateNote) bool {
	if len(n.Attachments) > 100 {
		return false
	}
	seen := map[string]bool{}
	for _, a := range n.Attachments {
		if !validNotesID(a.ID) || seen[a.ID] || !utf8.ValidString(a.Name) || utf8.RuneCountInString(a.Name) > 200 || strings.TrimSpace(a.Name) == "" || strings.ContainsAny(a.Name, "\x00\r\n/\\") || len(a.Mime) > 100 || a.Size < 0 || a.Size > notesMaxAttachment {
			return false
		}
		seen[a.ID] = true
	}
	return true
}
func notesAttachmentRefs(p notesPayload) map[string]notesAttachment {
	out := map[string]notesAttachment{}
	for _, n := range p.Notes {
		for _, a := range n.Attachments {
			out[a.ID] = a
		}
	}
	for _, v := range p.History {
		for _, a := range v.Note.Attachments {
			out[a.ID] = a
		}
	}
	for _, d := range p.Trash {
		for _, a := range d.Note.Attachments {
			out[a.ID] = a
		}
	}
	return out
}
func notesAttachmentRoot(dir *os.Root) (*os.Root, error) {
	if e := dir.Mkdir("attachments", 0700); e != nil && !os.IsExist(e) {
		return nil, e
	}
	st, e := dir.Lstat("attachments")
	if e != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("附件目录不可使用链接")
	}
	return dir.OpenRoot("attachments")
}
func attachmentFilename(vault, id string) string { return vault + "-" + id + ".bin" }
func sealNotesAttachment(p notesPayload, id string, plain []byte) []byte {
	nonce := notesRandom(12)
	return append(nonce, notesCipher(p.AttachmentKey).Seal(nil, nonce, plain, []byte(p.ID+"/"+id))...)
}
func openNotesAttachment(p notesPayload, id string, b []byte) ([]byte, error) {
	if len(p.AttachmentKey) != 32 || len(b) < 28 || len(b) > notesMaxAttachment+28 {
		return nil, errors.New("附件损坏或不完整")
	}
	return notesCipher(p.AttachmentKey).Open(nil, b[:12], b[12:], []byte(p.ID+"/"+id))
}
func readEncryptedAttachment(root *os.Root, vault, id string) ([]byte, error) {
	name := attachmentFilename(vault, id)
	st, e := root.Lstat(name)
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() || st.Size() > notesMaxAttachment+28 {
		return nil, errors.New("附件文件无效")
	}
	return root.ReadFile(name)
}
func (s *server) notesAttachmentAction(dir *os.Root, p *notesPayload, action string, req notesRequest) (bool, any, error) {
	if action != "attachment-add" && action != "attachment-read" && action != "attachment-remove" {
		return false, nil, nil
	}
	at := -1
	for i, n := range p.Notes {
		if n.ID == req.Note.ID {
			at = i
			break
		}
	}
	if at < 0 {
		return true, nil, errors.New("笔记不存在")
	}
	n := p.Notes[at]
	root, e := notesAttachmentRoot(dir)
	if e != nil {
		return true, nil, e
	}
	defer root.Close()
	if action == "attachment-read" {
		for _, a := range n.Attachments {
			if a.ID == req.AttachmentID {
				b, e := readEncryptedAttachment(root, p.ID, a.ID)
				if e != nil {
					return true, nil, errors.New("附件文件缺失，请从完整备份恢复")
				}
				plain, e := openNotesAttachment(*p, a.ID, b)
				if e != nil {
					return true, nil, errors.New("附件无法解密或已损坏")
				}
				return true, map[string]any{"Attachment": a, "Content": plain}, nil
			}
		}
		return true, nil, errors.New("附件不属于当前笔记")
	}
	if n.Locked || n.Revision != req.Note.Revision {
		return true, nil, errors.New("笔记已锁定编辑或已更新，请重新选择后重试")
	}
	notesAddVersion(p, n, true)
	if action == "attachment-remove" {
		found := false
		remaining := []notesAttachment{}
		for _, a := range n.Attachments {
			if a.ID == req.AttachmentID {
				found = true
			} else {
				remaining = append(remaining, a)
			}
		}
		if !found {
			return true, nil, errors.New("附件不存在")
		}
		n.Attachments = remaining
	} else {
		if len(req.Content) > notesMaxAttachment || len(n.Attachments) >= 100 {
			return true, nil, errors.New("每个附件最多 25 MB，每篇最多 100 个附件")
		}
		total := int64(len(req.Content))
		refs := notesAttachmentRefs(*p)
		for _, a := range refs {
			total += a.Size
		}
		if total > notesMaxBackup-(64<<20) || len(refs) >= 2000 {
			return true, nil, errors.New("附件总量最多 448 MB、2000 个，请先导出整理")
		}
		a := notesAttachment{ID: notesID(), Name: req.Name, Mime: http.DetectContentType(req.Content), Size: int64(len(req.Content))}
		if !validNotesAttachments(privateNote{Attachments: []notesAttachment{a}}) {
			return true, nil, errors.New("附件名称无效")
		}
		if len(p.AttachmentKey) == 0 {
			p.AttachmentKey = notesRandom(32)
		}
		b := sealNotesAttachment(*p, a.ID, req.Content)
		sum := sha256.Sum256(b)
		a.Hash = hex.EncodeToString(sum[:])
		if e = writeNotesFile(root, attachmentFilename(p.ID, a.ID), b); e != nil {
			return true, nil, errors.New("无法保存加密附件，请检查磁盘空间")
		}
		n.Attachments = append(append([]notesAttachment(nil), n.Attachments...), a)
	}
	n.Revision++
	n.Updated = time.Now().UTC()
	p.Notes[at] = n
	return true, map[string]any{"Note": n}, nil
}
func unpackNotesBackup(b []byte) (notesBackupBundle, error) {
	bundle := notesBackupBundle{Vault: b, Files: map[string]*zip.File{}}
	if len(b) > notesMaxBackup {
		return bundle, errors.New("备份最多 512 MB")
	}
	if len(b) < 4 || !bytes.Equal(b[:4], []byte{'P', 'K', 3, 4}) {
		return bundle, nil
	}
	reader, e := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if e != nil {
		return bundle, errors.New("备份容器损坏")
	}
	bundle.Vault = nil
	total := uint64(0)
	if len(reader.File) > 2001 {
		return bundle, errors.New("备份包含过多附件")
	}
	for _, f := range reader.File {
		total += f.UncompressedSize64
		if total > notesMaxBackup || !f.Mode().IsRegular() || f.Flags&1 != 0 {
			return bundle, errors.New("备份容量或文件类型无效")
		}
		if f.Name == "vault.json" {
			if bundle.Vault != nil || f.UncompressedSize64 > notesMaxFile {
				return bundle, errors.New("备份中的笔记库无效")
			}
			r, e := f.Open()
			if e != nil {
				return bundle, e
			}
			bundle.Vault, e = io.ReadAll(io.LimitReader(r, notesMaxFile+1))
			r.Close()
			if e != nil || len(bundle.Vault) > notesMaxFile {
				return bundle, errors.New("笔记库读取失败")
			}
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(f.Name, "attachments/"), ".bin")
		if f.Name != "attachments/"+id+".bin" || !validNotesID(id) || bundle.Files[id] != nil || f.UncompressedSize64 > notesMaxAttachment+28 {
			return bundle, errors.New("备份附件名称或大小无效")
		}
		bundle.Files[id] = f
	}
	if bundle.Vault == nil {
		return bundle, errors.New("备份中缺少笔记库")
	}
	return bundle, nil
}
func installNotesBackupAttachments(dir *os.Root, p notesPayload, bundle notesBackupBundle) error {
	refs := notesAttachmentRefs(p)
	if len(refs) != len(bundle.Files) {
		return errors.New("备份中的附件不完整，请使用包含附件的完整备份")
	}
	if len(refs) == 0 {
		return nil
	}
	root, e := notesAttachmentRoot(dir)
	if e != nil {
		return e
	}
	defer root.Close()
	// Verify every attachment before publishing any of them.
	for id, a := range refs {
		f := bundle.Files[id]
		if f == nil {
			return errors.New("备份缺少附件")
		}
		r, e := f.Open()
		if e != nil {
			return e
		}
		cipher, e := io.ReadAll(io.LimitReader(r, notesMaxAttachment+29))
		r.Close()
		if e != nil {
			return e
		}
		plain, e := openNotesAttachment(p, id, cipher)
		size := len(plain)
		clear(plain)
		if e != nil || int64(size) != a.Size {
			return errors.New("备份附件无法解密或校验失败")
		}
		name := attachmentFilename(p.ID, id)
		if st, e := root.Lstat(name); e == nil {
			if !st.Mode().IsRegular() {
				return errors.New("附件目标不可覆盖")
			}
			old, e := root.ReadFile(name)
			if e != nil || !bytes.Equal(old, cipher) {
				return errors.New("附件标识冲突，原笔记未变")
			}
		} else if !os.IsNotExist(e) {
			return e
		}
	}
	for id, f := range bundle.Files {
		name := attachmentFilename(p.ID, id)
		if _, e := root.Lstat(name); e == nil {
			continue
		}
		r, e := f.Open()
		if e != nil {
			return e
		}
		b, e := io.ReadAll(io.LimitReader(r, notesMaxAttachment+29))
		r.Close()
		if e != nil {
			return e
		}
		if e = writeNotesFile(root, name, b); e != nil {
			return e
		}
	}
	return nil
}
func streamNotesBackupFromRoot(w io.Writer, dir *os.Root, p notesPayload, vault []byte) error {
	refs := notesAttachmentRefs(p)
	if len(refs) == 0 {
		_, e := w.Write(vault)
		return e
	}
	root, e := notesAttachmentRoot(dir)
	if e != nil {
		return e
	}
	defer root.Close()
	writer := zip.NewWriter(w)
	entry, e := writer.CreateHeader(&zip.FileHeader{Name: "vault.json", Method: zip.Store})
	if e != nil {
		return e
	}
	if _, e = entry.Write(vault); e != nil {
		return e
	}
	for id, ref := range refs {
		name := attachmentFilename(p.ID, id)
		st, e := root.Lstat(name)
		if e != nil || !st.Mode().IsRegular() || st.Size() > notesMaxAttachment+28 {
			return errors.New("附件缺失或无效，备份未完成")
		}
		input, e := root.Open(name)
		if e != nil {
			return e
		}
		entry, e = writer.CreateHeader(&zip.FileHeader{Name: "attachments/" + id + ".bin", Method: zip.Store})
		if e == nil {
			hasher := sha256.New()
			var copied int64
			copied, e = io.Copy(io.MultiWriter(entry, hasher), io.LimitReader(input, notesMaxAttachment+29))
			if e == nil && (copied > notesMaxAttachment+28 || (ref.Size > 0 && copied != ref.Size+28) || (ref.Hash != "" && hex.EncodeToString(hasher.Sum(nil)) != ref.Hash)) {
				e = errors.New("附件完整性校验失败，备份未完成")
			}
		}
		input.Close()
		if e != nil {
			return e
		}
	}
	return writer.Close()
}
func writeNotesBackupFile(target *os.Root, name string, source *os.Root, p notesPayload, vault []byte) error {
	tmp := ".vault-" + notesID()
	f, e := target.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer target.Remove(tmp)
	e = streamNotesBackupFromRoot(f, source, p, vault)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if st, e := target.Lstat(name); e == nil && !st.Mode().IsRegular() {
		return errors.New("备份目标不能是链接或特殊文件")
	}
	return target.Rename(tmp, name)
}
func (s *server) notesCleanAttachments(dir *os.Root, p notesPayload) {
	refs := notesAttachmentRefs(p)
	root, e := notesAttachmentRoot(dir)
	if e != nil {
		return
	}
	defer root.Close()
	file, e := root.Open(".")
	if e != nil {
		return
	}
	defer file.Close()
	entries, e := file.ReadDir(-1)
	if e != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		id := strings.TrimSuffix(strings.TrimPrefix(name, p.ID+"-"), ".bin")
		if name == attachmentFilename(p.ID, id) && validNotesID(id) && refs[id].ID == "" && entry.Type().IsRegular() {
			_ = root.Remove(name)
		}
	}
}
func decodeNotesRequest(w http.ResponseWriter, r *http.Request, req *notesRequest) error {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		r.Body = http.MaxBytesReader(w, r.Body, notesMaxBackup+1<<20)
		reader, e := r.MultipartReader()
		if e != nil {
			return e
		}
		fields := map[string]bool{}
		for {
			part, e := reader.NextPart()
			if e == io.EOF {
				break
			}
			if e != nil {
				return e
			}
			name := part.FormName()
			if fields[name] {
				return errors.New("重复字段")
			}
			fields[name] = true
			limit := int64(1024)
			if name == "Backup" {
				limit = notesMaxBackup
			}
			b, e := io.ReadAll(io.LimitReader(part, limit+1))
			part.Close()
			if e != nil || int64(len(b)) > limit {
				return errors.New("备份或验证信息过大")
			}
			switch name {
			case "Password":
				req.Password = string(b)
			case "Code":
				req.Code = string(b)
			case "Confirm":
				req.Confirm = string(b) == "true"
			case "Backup":
				req.Backup = b
			default:
				return fmt.Errorf("不支持的备份字段")
			}
		}
		return nil
	}
	r.Body = http.MaxBytesReader(w, r.Body, 90<<20)
	return json.NewDecoder(r.Body).Decode(req)
}
