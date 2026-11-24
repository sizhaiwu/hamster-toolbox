package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type rememberedPassword struct {
	Remember          bool
	Password, SavedID string
}

func (s *server) loadPasswordMemory() (map[string]rememberedPassword, error) {
	values := map[string]rememberedPassword{}
	f, err := os.Open(filepath.Join(s.dataDir, "password-memory.dat"))
	if os.IsNotExist(err) {
		return values, nil
	}
	if err != nil {
		return nil, errors.New("无法读取密码记忆文件")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil || len(b) > 65536 || !bytes.HasPrefix(b, []byte("HTPM1")) {
		return nil, errors.New("密码记忆文件无效，原文件已保留")
	}
	plain, err := passwordCrypt(b[5:], false)
	if err != nil {
		return nil, err
	}
	defer clear(plain)
	if json.Unmarshal(plain, &values) != nil || values == nil {
		return nil, errors.New("密码记忆文件损坏，原文件已保留")
	}
	return values, nil
}

func (s *server) storePasswordMemory(values map[string]rememberedPassword) error {
	plain, err := json.Marshal(values)
	if err != nil {
		return err
	}
	defer clear(plain)
	encrypted, err := passwordCrypt(plain, true)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(s.dataDir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(s.dataDir, ".password-memory-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(append([]byte("HTPM1"), encrypted...)); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), filepath.Join(s.dataDir, "password-memory.dat"))
}

func (s *server) passwordMemoryAPI(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Tool string
		rememberedPassword
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Tool != "archive" && req.Tool != "extract" {
		fail(w, errors.New("密码记忆请求无效"))
		return
	}
	if runtime.GOOS != "windows" {
		if r.URL.Path == "/api/password-memory-save" && req.Remember {
			fail(w, errors.New("当前平台暂不支持本机加密记忆，可在本次页面使用临时密码"))
			return
		}
		send(w, map[string]any{"Supported": false, "Remember": false, "Password": "", "SavedID": ""})
		return
	}
	s.passwordsMu.Lock()
	defer s.passwordsMu.Unlock()
	values, err := s.loadPasswordMemory()
	if err != nil {
		fail(w, err)
		return
	}
	if r.URL.Path == "/api/password-memory-save" {
		if !req.Remember {
			req.Password = ""
			req.SavedID = ""
		}
		if len(req.Password) > 4096 || strings.ContainsAny(req.Password, "\x00\r\n") {
			fail(w, errors.New("密码不能超过 4096 字节或包含换行、空字符"))
			return
		}
		if req.SavedID != "" {
			vault, e := s.loadPasswords()
			if e != nil {
				fail(w, e)
				return
			}
			found := false
			for _, saved := range vault {
				if saved.ID == req.SavedID && saved.Password == req.Password {
					found = true
					break
				}
			}
			if !found {
				fail(w, errors.New("常用密码已变更，请重新选择"))
				return
			}
			// Keep only the reference, so editing/deleting a saved password also
			// updates its remembered use without retaining a stale secret copy.
			req.Password = ""
		}
		values[req.Tool] = req.rememberedPassword
		if err = s.storePasswordMemory(values); err != nil {
			fail(w, errors.New("密码记忆保存失败，原文件已保留"))
			return
		}
		send(w, map[string]bool{"ok": true})
		return
	}
	value, exists := values[req.Tool]
	if !exists {
		value.Remember = true
	}
	if value.Remember && value.SavedID != "" {
		vault, e := s.loadPasswords()
		if e != nil {
			fail(w, e)
			return
		}
		id := value.SavedID
		value.SavedID = ""
		value.Password = ""
		for _, saved := range vault {
			if saved.ID == id {
				value.SavedID = id
				value.Password = saved.Password
				break
			}
		}
	}
	if !value.Remember {
		value.Password = ""
		value.SavedID = ""
	}
	send(w, struct {
		Supported bool
		rememberedPassword
	}{true, value})
}
