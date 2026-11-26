package toolbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const BrowserTextLimit = 2 << 20

var ErrTextConflict = errors.New("文件内容已被其他操作修改，请保留当前编辑内容，重新打开文件后对比；未覆盖原文件")
var ErrTextEncoding = errors.New("当前支持 UTF-8、UTF-8 BOM 和 UTF-16 文本；其他编码请先在本机编辑器中转换，原文件未修改")
var ErrTextTooLarge = errors.New("文件超过 2 MiB，请下载后使用本机编辑器")
var ErrTextBinary = errors.New("此文件包含二进制内容，不能作为文本编辑")

type BrowserText struct {
	Text, Revision, Encoding, Newline string
	BOM                               bool
}

func textDigest(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }
func readTextBytes(path string) ([]byte, os.FileInfo, error) {
	f, err := BrowseOpen(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, nil, ErrTextBinary
	}
	if st.Size() > BrowserTextLimit {
		return nil, nil, ErrTextTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(f, BrowserTextLimit+1))
	if len(data) > BrowserTextLimit {
		return nil, nil, ErrTextTooLarge
	}
	return data, st, err
}
func decodeBrowserText(data []byte) (BrowserText, error) {
	out := BrowserText{Revision: textDigest(data), Encoding: "UTF-8", Newline: "LF"}
	payload := data
	if bytes.HasPrefix(payload, []byte{0xef, 0xbb, 0xbf}) {
		out.BOM = true
		payload = payload[3:]
	}
	if bytes.HasPrefix(data, []byte{0xff, 0xfe}) || bytes.HasPrefix(data, []byte{0xfe, 0xff}) {
		out.BOM = true
		out.Encoding = "UTF-16 LE"
		payload = data[2:]
		var order binary.ByteOrder = binary.LittleEndian
		if data[0] == 0xfe {
			out.Encoding = "UTF-16 BE"
			order = binary.BigEndian
		}
		if len(payload)%2 != 0 {
			return out, ErrTextEncoding
		}
		units := make([]uint16, len(payload)/2)
		for i := range units {
			units[i] = order.Uint16(payload[i*2:])
		}
		for i := 0; i < len(units); i++ {
			u := units[i]
			if u >= 0xd800 && u <= 0xdbff {
				if i+1 >= len(units) || units[i+1] < 0xdc00 || units[i+1] > 0xdfff {
					return out, ErrTextEncoding
				}
				i++
			} else if u >= 0xdc00 && u <= 0xdfff {
				return out, ErrTextEncoding
			}
		}
		out.Text = string(utf16.Decode(units))
	} else {
		if !utf8.Valid(payload) {
			return out, ErrTextEncoding
		}
		out.Text = string(payload)
	}
	for _, r := range out.Text {
		if r < 32 && r != '\n' && r != '\r' && r != '\t' && r != '\f' {
			return out, ErrTextBinary
		}
	}
	crlf := strings.Count(out.Text, "\r\n")
	lf := strings.Count(out.Text, "\n")
	cr := strings.Count(out.Text, "\r")
	if crlf > 0 && crlf == lf && cr == crlf {
		out.Newline = "CRLF"
	} else if cr > 0 && lf == 0 {
		out.Newline = "CR"
	} else if cr > 0 {
		return out, errors.New("文件混合使用多种换行格式，请先用本机编辑器统一换行后再编辑")
	}
	out.Text = strings.ReplaceAll(strings.ReplaceAll(out.Text, "\r\n", "\n"), "\r", "\n")
	return out, nil
}
func BrowserReadText(path string) (BrowserText, error) {
	ref, err := CaptureBrowserItem(path)
	if err != nil {
		return BrowserText{}, err
	}
	data, _, err := readTextBytes(path)
	if err != nil {
		return BrowserText{}, err
	}
	if err = ref.Check(path); err != nil {
		return BrowserText{}, ErrTextConflict
	}
	return decodeBrowserText(data)
}
func encodeBrowserText(text string, original BrowserText) ([]byte, error) {
	if !utf8.ValidString(text) {
		return nil, ErrTextEncoding
	}
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	if original.Newline == "CRLF" {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	} else if original.Newline == "CR" {
		text = strings.ReplaceAll(text, "\n", "\r")
	}
	data := []byte(text)
	if strings.HasPrefix(original.Encoding, "UTF-16") {
		units := utf16.Encode([]rune(text))
		data = make([]byte, 2+len(units)*2)
		var order binary.ByteOrder = binary.LittleEndian
		data[0], data[1] = 0xff, 0xfe
		if original.Encoding == "UTF-16 BE" {
			order = binary.BigEndian
			data[0], data[1] = 0xfe, 0xff
		}
		for i, u := range units {
			order.PutUint16(data[2+i*2:], u)
		}
	} else if original.BOM {
		data = append([]byte{0xef, 0xbb, 0xbf}, data...)
	}
	if len(data) > BrowserTextLimit {
		return nil, ErrTextTooLarge
	}
	if _, err := decodeBrowserText(data); err != nil {
		return nil, err
	}
	return data, nil
}

// Write to a sibling first, then retain and verify the old file before publishing.
// No in-place truncation; concurrent changes and publication failure keep the old bytes.
func BrowserSaveText(ctx context.Context, path, revision, text, historyRoot string) (BrowserText, string, error) {
	var empty BrowserText
	p, err := safePath(path)
	if err != nil {
		return empty, "", err
	}
	ref, err := CaptureBrowserItem(p)
	if err != nil {
		return empty, "", err
	}
	data, st, err := readTextBytes(p)
	if err != nil {
		return empty, "", err
	}
	if st.Mode().Perm()&0222 == 0 {
		return empty, "", errors.New("文件为只读，不能保存")
	}
	if revision == "" || textDigest(data) != revision {
		return empty, "", ErrTextConflict
	}
	original, err := decodeBrowserText(data)
	if err != nil {
		return empty, "", err
	}
	encoded, err := encodeBrowserText(text, original)
	if err != nil {
		return empty, "", err
	}
	if err = ref.Check(p); err != nil {
		return empty, "", ErrTextConflict
	}
	if bytes.Equal(encoded, data) {
		return original, "", nil
	}
	id, err := snapshotFileIdentity(p)
	if err != nil {
		return empty, "", err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".toolbox-edit-*")
	if err != nil {
		return empty, "", err
	}
	staged := f.Name()
	defer os.Remove(staged)
	if _, err = f.Write(encoded); err == nil {
		err = f.Chmod(st.Mode().Perm())
	}
	if err == nil {
		err = f.Sync()
	}
	if ce := f.Close(); err == nil {
		err = ce
	}
	if err != nil {
		return empty, "", err
	}
	if err = ctx.Err(); err != nil {
		return empty, "", err
	}
	if err = ref.Check(p); err != nil {
		return empty, "", ErrTextConflict
	}
	if _, err = safePath(p); err != nil {
		return empty, "", err
	}
	// Keep recovery history outside the edited directory. The sibling is only
	// a short-lived rollback location, so publication stays on the same volume.
	history, err := writeBrowserTextHistory(historyRoot, p, data)
	if err != nil {
		return empty, "", err
	}
	published := false
	defer func() {
		if !published {
			os.Remove(history)
		}
	}()
	backupDir, err := os.MkdirTemp(filepath.Dir(p), ".toolbox-edit-rollback-")
	if err != nil {
		return empty, "", err
	}
	defer os.Remove(backupDir) // Never recursively delete a failed recovery.
	backup := filepath.Join(backupDir, filepath.Base(p))
	if err = browserMoveNoReplace(p, backup); err != nil {
		return empty, "", err
	}
	restore := func(cause error) (BrowserText, string, error) {
		if e := browserMoveNoReplace(backup, p); e != nil {
			return empty, backup, fmt.Errorf("保存失败，原文件保留在备份目录 %s：%w", backup, cause)
		}
		return empty, "", cause
	}
	check, _, err := readTextBytes(backup)
	if err != nil {
		return restore(err)
	}
	checkID, err := snapshotFileIdentity(backup)
	if err != nil || !sameFileIdentity(id, checkID) || !bytes.Equal(check, data) {
		return restore(ErrTextConflict)
	}
	if err = ctx.Err(); err != nil {
		return restore(err)
	}
	if _, err = safePath(p); err != nil {
		return restore(err)
	}
	if err = browserMoveNoReplace(staged, p); err != nil {
		return restore(err)
	}
	published = true
	// All handles are closed before removal (required on Windows).
	if err = os.Remove(backup); err != nil {
		// The save succeeded; retain and report the extra recovery copy if the
		// OS prevents cleanup, rather than claiming the new file was not saved.
		out, decodeErr := decodeBrowserText(encoded)
		return out, backup, decodeErr
	}
	pruneBrowserTextHistory(filepath.Dir(history), history)
	out, err := decodeBrowserText(encoded)
	return out, history, err
}
