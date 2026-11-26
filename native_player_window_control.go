package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

func validNativeWindowMemory(size nativeWindowMemory) bool {
	return size.Width >= 160 && size.Width <= 16384 && size.Height >= 120 && size.Height <= 16384
}
func cloneNativeWindowMemory(source map[string]nativeWindowMemory) map[string]nativeWindowMemory {
	out := map[string]nativeWindowMemory{}
	for kind, size := range source {
		out[kind] = size
	}
	return out
}
func replaceNativeFit(args []string, size nativeWindowMemory) []string {
	for i, arg := range args {
		if strings.HasPrefix(arg, "--autofit=") {
			args[i] = fmt.Sprintf("--autofit=%dx%d", size.Width, size.Height)
		}
	}
	return args
}
func (m *nativePlayerManager) setRememberWindows(enabled bool) error {
	m.configuration.Lock()
	defer m.configuration.Unlock()
	m.mu.Lock()
	program, size, memory := m.program, m.windowSize, cloneNativeWindowMemory(m.windowMemory)
	m.mu.Unlock()
	return m.saveWindowConfiguration(program, size, enabled, memory, false)
}
func (m *nativePlayerManager) rememberWindow(s *nativePlayback) {
	s.mu.Lock()
	size, kind, epoch := nativeWindowMemory{s.view.Width, s.view.Height}, s.view.Kind, s.memoryEpoch
	skip := s.fullscreen || s.maximized || !s.windowDirty
	s.mu.Unlock()
	if skip || !validNativeWindowMemory(size) || (kind != "image" && kind != "video") {
		return
	}
	m.configuration.Lock()
	defer m.configuration.Unlock()
	m.mu.Lock()
	if !m.rememberWindows || epoch != m.memoryEpoch || m.windowMemory[kind] == size {
		m.mu.Unlock()
		return
	}
	program, preset, memory := m.program, m.windowSize, cloneNativeWindowMemory(m.windowMemory)
	m.mu.Unlock()
	memory[kind] = size
	if err := m.saveWindowConfiguration(program, preset, true, memory, false); err != nil {
		m.mu.Lock()
		m.lastError = "窗口大小记忆失败：" + err.Error()
		m.mu.Unlock()
	} else {
		s.mu.Lock()
		if s.view.Width == size.Width && s.view.Height == size.Height {
			s.windowDirty = false
		}
		s.mu.Unlock()
	}
}

func (s *nativePlayback) request(command []any) error {
	s.mu.Lock()
	s.nextRequest++
	id := s.nextRequest
	if s.pending == nil {
		s.pending = map[int]chan error{}
	}
	ch := make(chan error, 1)
	s.pending[id] = ch
	conn := s.conn
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.pending, id); s.mu.Unlock() }()
	if conn == nil {
		return errors.New("播放器尚未连接")
	}
	s.write.Lock()
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	err := json.NewEncoder(conn).Encode(map[string]any{"command": command, "request_id": id})
	s.write.Unlock()
	if err != nil {
		return err
	}
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case err := <-ch:
		return err
	case <-timer.C:
		return errors.New("播放器未确认窗口操作")
	}
}

// A cascade changes position only. Tiling explicitly changes window sizes.
// mpv handles display scaling; Wayland window managers may deny positioning.
func nativeLayoutGeometry(count, index int, tile bool) string {
	if !tile {
		return fmt.Sprintf("+%d+%d", 40+(index%12)*28, 40+(index%12)*28)
	}
	cols := int(math.Ceil(math.Sqrt(float64(count))))
	if cols < 1 {
		cols = 1
	}
	rows := (count + cols - 1) / cols
	if rows < 1 {
		rows = 1
	}
	w, h := max(1, 90/cols-2), max(1, 84/rows-2)
	left, top := 5+(index%cols)*(90/cols), 5+(index/cols)*(84/rows)
	x, y := min(100, left*100/(100-w)), min(100, top*100/(100-h))
	return fmt.Sprintf("%d%%x%d%%+%d%%+%d%%", w, h, x, y)
}
func (m *nativePlayerManager) controlWindows(action string) error {
	m.mu.Lock()
	sessions := make([]*nativePlayback, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.mu.Unlock()
	if len(sessions) == 0 {
		return errors.New("当前没有原生查看窗口")
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].view.ID < sessions[j].view.ID })
	var wg sync.WaitGroup
	var mu sync.Mutex
	failed := 0
	var first error
	slots := make(chan struct{}, 4)
	for index, s := range sessions {
		slots <- struct{}{}
		wg.Add(1)
		go func(index int, s *nativePlayback) {
			defer wg.Done()
			defer func() { <-slots }()
			var command []any
			switch action {
			case "tile", "cascade":
				m.rememberWindow(s)
				s.mu.Lock()
				s.memorySuppressedUntil = time.Now().Add(3 * time.Second)
				s.mu.Unlock()
				// Fullscreen/maximized windows must return to normal before placement.
				if err := s.request([]any{"set_property", "fullscreen", false}); err != nil {
					mu.Lock()
					failed++
					if first == nil {
						first = err
					}
					mu.Unlock()
					return
				}
				_ = s.request([]any{"set_property", "window-maximized", false})
				command = []any{"set_property", "geometry", nativeLayoutGeometry(len(sessions), index, action == "tile")}
			case "pause-all":
				command = []any{"set_property", "pause", true}
			case "mute-all":
				command = []any{"set_property", "mute", true}
			default:
				command = nil
			}
			var err error
			if command == nil {
				err = errors.New("不支持的窗口操作")
			} else {
				err = s.request(command)
			}
			if err != nil {
				mu.Lock()
				failed++
				if first == nil {
					first = err
				}
				mu.Unlock()
			}
		}(index, s)
	}
	wg.Wait()
	if failed > 0 {
		return fmt.Errorf("已完成 %d 个窗口，%d 个未完成：%w", len(sessions)-failed, failed, first)
	}
	return nil
}
