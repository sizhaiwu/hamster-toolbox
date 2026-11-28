package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

// The GUI launcher is built from the same application, with the Windows GUI
// subsystem. CLI users keep the ordinary console executable.
var desktopMode = "false"

type desktopTrayAction struct {
	ID      int
	Label   string
	Route   string
	Browser bool
}

var desktopTrayActions = []desktopTrayAction{
	{1, "打开工具箱", "/", false},
	{2, "在浏览器中打开", "/", true},
	{3, "资源管理器", "/files/", false},
	{4, "私密笔记", "/notes/", false},
	{5, "计划任务", "/schedules.html", false},
	{6, "通用设置", "/#settings", false},
}

func desktopTrayDestination(address string, id int) (string, bool, error) {
	if !validDesktopAddress(address) {
		return "", false, fmt.Errorf("本机工具箱地址无效")
	}
	base, _ := url.Parse(address)
	token := base.Query().Get("session")
	if token == "" {
		return "", false, fmt.Errorf("本机会话无效，请重启工具箱")
	}
	for _, action := range desktopTrayActions {
		if action.ID == id {
			target, _ := url.Parse(action.Route)
			target = base.ResolveReference(target)
			target.RawQuery = url.Values{"session": {token}}.Encode()
			return target.String(), action.Browser, nil
		}
	}
	return "", false, fmt.Errorf("未知托盘操作")
}

func desktopInstanceName(data string) string {
	p, err := filepath.Abs(data)
	if err != nil {
		p = filepath.Clean(data)
	}
	if physical, err := filepath.EvalSymlinks(p); err == nil {
		p = physical
	}
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	sum := sha256.Sum256([]byte(p))
	return "HamsterToolbox.Desktop." + hex.EncodeToString(sum[:12])
}

func (s *server) desktopCanExit() bool {
	r := s.taskRoot()
	r.taskDispatchMu.Lock()
	defer r.taskDispatchMu.Unlock()
	return r.allTasksIdle() && r.desktopBackgroundIdle()
}
func (s *server) beginDesktopExit() bool {
	r := s.taskRoot()
	r.taskDispatchMu.Lock()
	defer r.taskDispatchMu.Unlock()
	if !r.allTasksIdle() || !r.desktopBackgroundIdle() {
		return false
	}
	r.mu.Lock()
	r.desktopClosing = true
	r.mu.Unlock()
	return true
}

func (s *server) desktopBackgroundIdle() bool {
	if s.scheduler != nil {
		s.scheduler.mu.Lock()
		busy := len(s.scheduler.active) > 0 || len(s.scheduler.queue) > 0
		s.scheduler.mu.Unlock()
		if busy {
			return false
		}
	}
	if s.browser != nil {
		if !s.browser.operations.TryLock() {
			return false
		}
		s.browser.operations.Unlock()
	}
	return true
}
