package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type savedPassword struct{ ID, Name, Password string }
type passwordChoice struct{ ID, Name string }

func (s *server) loadPasswords() ([]savedPassword, error) {
	b, e := os.ReadFile(filepath.Join(s.dataDir, "passwords.dat"))
	if os.IsNotExist(e) {
		return []savedPassword{}, nil
	}
	if e != nil {
		return nil, errors.New("无法读取常用密码文件")
	}
	if !bytes.HasPrefix(b, []byte("HTPW1")) {
		return nil, errors.New("常用密码文件格式无效，原文件已保留")
	}
	plain, e := passwordCrypt(b[5:], false)
	if e != nil {
		return nil, e
	}
	defer clear(plain)
	var result []savedPassword
	if json.Unmarshal(plain, &result) != nil {
		return nil, errors.New("常用密码文件损坏，原文件已保留")
	}
	return result, nil
}

func (s *server) storePasswords(values []savedPassword) error {
	plain, e := json.Marshal(values)
	if e != nil {
		return e
	}
	defer clear(plain)
	encrypted, e := passwordCrypt(plain, true)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(s.dataDir, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(s.dataDir, ".passwords-*")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	_, e = f.Write(append([]byte("HTPW1"), encrypted...))
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return os.Rename(name, filepath.Join(s.dataDir, "passwords.dat"))
}

func (s *server) passwordAPI(w http.ResponseWriter, r *http.Request) {
	s.passwordsMu.Lock()
	defer s.passwordsMu.Unlock()
	values, e := s.loadPasswords()
	if e != nil {
		fail(w, e)
		return
	}
	if r.URL.Path == "/api/passwords" {
		choices := []passwordChoice{}
		for _, v := range values {
			choices = append(choices, passwordChoice{v.ID, v.Name})
		}
		send(w, choices)
		return
	}
	var req savedPassword
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		fail(w, errors.New("密码请求格式无效"))
		return
	}
	at := -1
	for i, v := range values {
		if v.ID == req.ID {
			at = i
			break
		}
	}
	switch r.URL.Path {
	case "/api/password-get":
		if at < 0 {
			fail(w, errors.New("常用密码已不存在，请刷新列表"))
			return
		}
		send(w, map[string]string{"Password": values[at].Password})
		return
	case "/api/password-save":
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" || utf8.RuneCountInString(req.Name) > 60 || strings.ContainsAny(req.Name, "\x00\r\n") {
			fail(w, errors.New("请填写 1–60 字的密码名称"))
			return
		}
		if req.Password == "" || len(req.Password) > 4096 || strings.ContainsAny(req.Password, "\x00\r\n") {
			fail(w, errors.New("请输入密码，不能包含换行或空字符"))
			return
		}
		for _, v := range values {
			if v.ID != req.ID && strings.EqualFold(v.Name, req.Name) {
				fail(w, errors.New("名称已存在，请换一个容易区分的名称"))
				return
			}
		}
		if req.ID != "" && at < 0 {
			fail(w, errors.New("此密码已被删除，请重新添加"))
			return
		}
		if at >= 0 {
			values[at] = req
		} else {
			if len(values) >= 100 {
				fail(w, errors.New("最多保存 100 个常用密码"))
				return
			}
			req.ID = randomID()
			values = append(values, req)
		}
	case "/api/password-delete":
		if at < 0 {
			fail(w, errors.New("此密码已被删除"))
			return
		}
		values = append(values[:at], values[at+1:]...)
	default:
		fail(w, fmt.Errorf("未知密码操作"))
		return
	}
	if e = s.storePasswords(values); e != nil {
		fail(w, errors.New("常用密码保存失败，原文件已保留"))
		return
	}
	send(w, map[string]bool{"ok": true})
}
