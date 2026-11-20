package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

type autostartStatus struct {
	Supported, Enabled, CanEnable, OtherLocation bool
	Message                                      string
}
type autostartStore interface {
	Read() (string, error)
	Write(string) error
	Delete() error
}
type autostartManager struct {
	mu               sync.Mutex
	program, command string
	store            autostartStore
	problem          error
}

func (s *server) startupManager() *autostartManager {
	s.autostartMu.Lock()
	defer s.autostartMu.Unlock()
	if s.autostart == nil {
		exe, err := os.Executable()
		program := filepath.Join(filepath.Dir(exe), "hamster-toolbox-desktop.exe")
		defaultData := ""
		if dir, e := os.UserConfigDir(); e == nil {
			defaultData = filepath.Join(dir, "media-toolbox")
		} else {
			defaultData = filepath.Join(filepath.Dir(exe), "data")
		}
		command, commandErr := autostartCommand(program, s.dataDir, defaultData)
		if err == nil {
			err = commandErr
		}
		s.autostart = &autostartManager{program: program, command: command, store: platformAutostartStore(), problem: err}
	}
	return s.autostart
}
func (m *autostartManager) statusLocked() (autostartStatus, error) {
	status := autostartStatus{Supported: m.store != nil}
	if m.store == nil {
		status.Message = "开机自启目前仅支持 Windows。"
		return status, nil
	}
	current, err := m.store.Read()
	if err != nil {
		return status, err
	}
	status.Enabled = m.command != "" && autostartCommandsEqual(current, m.command)
	status.OtherLocation = current != "" && !status.Enabled
	info, err := os.Lstat(m.program)
	status.CanEnable = m.problem == nil && err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0
	status.Message = "仅当前 Windows 用户登录后在托盘运行，不主动弹出浏览器。"
	if status.OtherLocation {
		status.Message = "已由其他程序位置或数据目录配置自启；开启将切换到当前版本。"
	}
	if !status.CanEnable {
		status.Message = "缺少托盘启动文件，或程序/数据目录路径过长。请使用完整安装目录。"
	}
	return status, nil
}
func (m *autostartManager) read() (autostartStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.statusLocked()
}
func (m *autostartManager) set(enabled bool) (autostartStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	status, err := m.statusLocked()
	if err != nil {
		return status, err
	}
	if !status.Supported {
		return status, errors.New("此平台暂不支持开机自启")
	}
	if enabled {
		if !status.CanEnable {
			return status, errors.New(status.Message)
		}
		err = m.store.Write(m.command)
	} else if status.Enabled {
		err = m.store.Delete()
	}
	if err != nil {
		return status, err
	}
	return m.statusLocked()
}
func (s *server) autostartAPI(w http.ResponseWriter, r *http.Request) {
	var request struct{ Enabled *bool }
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		fail(w, errors.New("自启设置请求无效"))
		return
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		fail(w, errors.New("自启设置请求无效"))
		return
	}
	manager := s.startupManager()
	var status autostartStatus
	var err error
	if request.Enabled == nil {
		status, err = manager.read()
	} else {
		status, err = manager.set(*request.Enabled)
	}
	if err != nil {
		fail(w, err)
		return
	}
	send(w, status)
}
