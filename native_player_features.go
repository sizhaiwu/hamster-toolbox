package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type nativeMediaTrack struct {
	ID       int    `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Lang     string `json:"lang"`
	Codec    string `json:"codec"`
	Selected bool   `json:"selected"`
}
type nativeMediaPreferences struct {
	Remember, FocusAudio bool
	Controls             string
	EndAction            string
}

func (m *nativePlayerManager) loadMediaPreferences() {
	data, e := os.ReadFile(filepath.Join(m.dataDir, "native-media.json"))
	if e == nil && len(data) < 4096 {
		_ = json.Unmarshal(data, &m.mediaPreferences)
	}
	m.mediaPreferences.Controls = nativeControlsMode(m.mediaPreferences.Controls)
	m.mediaPreferences.EndAction = nativeEndAction(m.mediaPreferences.EndAction)
}
func (m *nativePlayerManager) setMediaPreference(action string, enabled bool) error {
	m.configuration.Lock()
	defer m.configuration.Unlock()
	m.mu.Lock()
	p := m.mediaPreferences
	m.mu.Unlock()
	if action == "remember-media" {
		p.Remember = enabled
	} else {
		p.FocusAudio = enabled
	}
	return m.saveMediaPreferences(p)
}

// Caller holds configuration, keeping independent settings from overwriting each other.
func (m *nativePlayerManager) saveMediaPreferences(p nativeMediaPreferences) error {
	m.mu.Lock()
	restoreFocus := m.mediaPreferences.FocusAudio && !p.FocusAudio
	m.mu.Unlock()
	if e := os.MkdirAll(m.dataDir, 0700); e != nil {
		return e
	}
	data, _ := json.Marshal(p)
	f, e := os.CreateTemp(m.dataDir, ".native-media-*.tmp")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(data); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = os.Rename(f.Name(), filepath.Join(m.dataDir, "native-media.json")); e != nil {
		return e
	}
	m.mu.Lock()
	m.mediaPreferences = p
	m.mu.Unlock()
	if restoreFocus {
		go m.restoreFocusAudio()
	}
	return nil
}
func validNativeControls(mode string) bool {
	return mode == "full"
}
func nativeControlsMode(_ string) string {
	// All saved modes, including the retired simple/hidden modes, use the unified controller.
	return "full"
}
func nativeOSCOptions(mode string) string {
	return "hamster-mode=" + nativeControlsMode(mode)
}
func nativeEndAction(action string) string {
	if action == "loop" || action == "close" {
		return action
	}
	return "keep"
}
func (m *nativePlayerManager) setEndAction(action string) error {
	if action != "keep" && action != "loop" && action != "close" {
		return errors.New("请选择停留最后一帧、循环当前视频或关闭当前播放器")
	}
	m.configuration.Lock()
	defer m.configuration.Unlock()
	m.mu.Lock()
	p := m.mediaPreferences
	m.mu.Unlock()
	p.EndAction = action
	return m.saveMediaPreferences(p)
}
func (m *nativePlayerManager) applyControls(s *nativePlayback) error {
	s.controlsWrite.Lock()
	defer s.controlsWrite.Unlock()
	m.mu.Lock()
	mode := nativeControlsMode(m.mediaPreferences.Controls)
	m.mu.Unlock()
	s.mu.Lock()
	options := nativeOSCOptions(mode)
	same := options == s.controlsOptions
	s.mu.Unlock()
	if same {
		return nil
	}
	if e := s.request([]any{"set_property", "options/script-opts", options}); e != nil {
		return e
	}
	s.mu.Lock()
	s.controlsOptions = options
	s.mu.Unlock()
	return nil
}
func (m *nativePlayerManager) setControlsMode(mode string) error {
	if !validNativeControls(mode) {
		return errors.New("播放器使用统一控制栏，请更新页面后重试")
	}
	m.configuration.Lock()
	defer m.configuration.Unlock()
	m.mu.Lock()
	p := m.mediaPreferences
	m.mu.Unlock()
	p.Controls = mode
	if e := m.saveMediaPreferences(p); e != nil {
		return e
	}
	failures := 0
	for _, s := range m.mediaSessions() {
		if e := m.applyControls(s); e != nil {
			failures++
		}
	}
	if failures > 0 {
		m.mu.Lock()
		m.lastError = fmt.Sprintf("已保存控制栏模式，%d 个窗口尚未连接，请重新打开这些窗口后生效", failures)
		m.mu.Unlock()
	}
	return nil
}
func watchLaterFile(name string) bool {
	b, e := hex.DecodeString(name)
	return e == nil && (len(b) == 16 || len(b) == 32)
}
func (m *nativePlayerManager) cleanMediaHistory(clear bool) error {
	dir := filepath.Join(m.dataDir, "native-watch-later")
	if st, e := os.Lstat(dir); e == nil && st.Mode()&os.ModeSymlink != 0 {
		return errors.New("观看记录目录不能是链接")
	}
	entries, e := os.ReadDir(dir)
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	if e != nil {
		return e
	}
	type record struct {
		name     string
		modified int64
	}
	var records []record
	for _, entry := range entries {
		if !watchLaterFile(entry.Name()) || !entry.Type().IsRegular() {
			continue
		}
		st, e := entry.Info()
		if e == nil {
			records = append(records, record{entry.Name(), st.ModTime().UnixNano()})
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].modified > records[j].modified })
	keep := 500
	if clear {
		keep = 0
	}
	var failures int
	for i := keep; i < len(records); i++ {
		if e = os.Remove(filepath.Join(dir, records[i].name)); e != nil {
			failures++
		}
	}
	if failures > 0 {
		return fmt.Errorf("有 %d 条观看记录暂时无法清除", failures)
	}
	return nil
}
func (m *nativePlayerManager) mediaPlaybackArgs(args []string, kind string) []string {
	m.mu.Lock()
	remember := m.mediaPreferences.Remember
	controls := nativeControlsMode(m.mediaPreferences.Controls)
	endAction := nativeEndAction(m.mediaPreferences.EndAction)
	m.mu.Unlock()
	keepOpen, loopFile := "yes", "no"
	if kind == "video" {
		if endAction == "close" {
			keepOpen = "no"
		} else if endAction == "loop" {
			loopFile = "inf"
		}
	}
	args = append([]string{}, args...)
	for i, arg := range args {
		if strings.HasPrefix(arg, "--script-opts=") {
			args[i] = "--script-opts=" + nativeOSCOptions(controls)
		} else if strings.HasPrefix(arg, "--keep-open=") {
			args[i] = "--keep-open=" + keepOpen
		}
	}
	extra := []string{"--resume-playback=no", "--save-position-on-quit=no"}
	if remember {
		dir := filepath.Join(m.dataDir, "native-watch-later")
		if os.MkdirAll(dir, 0700) == nil {
			_ = m.cleanMediaHistory(false)
			extra = []string{"--resume-playback=yes", "--save-position-on-quit=yes", "--watch-later-directory=" + dir, "--write-filename-in-watch-later-config=no", "--resume-playback-check-mtime=yes", "--watch-later-options=start,volume,speed,aid,sid,video-rotate,video-zoom,video-pan-x,video-pan-y"}
		}
	}
	extra = append(extra, "--loop-file="+loopFile)
	if len(args) < 2 {
		return args
	}
	out := append([]string{}, args[:len(args)-2]...)
	out = append(out, extra...)
	return append(out, args[len(args)-2:]...)
}

// Called while the session mutex is held. No commands are sent from the reader lock.
func (s *nativePlayback) updateMediaProperty(name string, data json.RawMessage) bool {
	switch name {
	case "speed":
		_ = json.Unmarshal(data, &s.view.Speed)
	case "track-list":
		var tracks []nativeMediaTrack
		if json.Unmarshal(data, &tracks) == nil && len(tracks) <= 256 {
			s.view.Tracks = tracks
		}
	case "decoder-frame-drop-count":
		if string(data) != "null" && json.Unmarshal(data, &s.view.DroppedFrames) == nil {
			s.view.DropMetricsKnown = true
		}
	case "frame-drop-count":
		if string(data) != "null" && json.Unmarshal(data, &s.view.RenderDroppedFrames) == nil {
			s.view.RenderMetricsKnown = true
		}
	case "video-rotate":
		_ = json.Unmarshal(data, &s.view.Rotation)
	case "video-params/w":
		_ = json.Unmarshal(data, &s.view.VideoWidth)
	case "video-params/h":
		_ = json.Unmarshal(data, &s.view.VideoHeight)
	case "focused":
		var focused bool
		if json.Unmarshal(data, &focused) == nil {
			changed := focused && !s.view.Focused
			s.view.Focused = focused
			return changed
		}
	}
	return false
}
func (m *nativePlayerManager) mediaSessions() []*nativePlayback {
	m.mu.Lock()
	defer m.mu.Unlock()
	var sessions []*nativePlayback
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	return sessions
}
func (m *nativePlayerManager) focusMedia(id string) {
	m.mediaFocus.Lock()
	defer m.mediaFocus.Unlock()
	m.mu.Lock()
	enabled := m.mediaPreferences.FocusAudio
	m.mu.Unlock()
	if !enabled {
		return
	}
	for _, s := range m.mediaSessions() {
		s.mu.Lock()
		if s.view.Kind != "video" || !s.view.Connected {
			s.mu.Unlock()
			continue
		}
		mute := s.view.ID != id
		change := mute && !s.view.Muted || !mute && s.focusMuted
		if change {
			s.focusMuted = mute
		}
		s.mu.Unlock()
		if change {
			_ = s.send([]any{"set_property", "mute", mute})
		}
	}
}
func (m *nativePlayerManager) restoreFocusAudio() {
	m.mediaFocus.Lock()
	defer m.mediaFocus.Unlock()
	for _, s := range m.mediaSessions() {
		s.mu.Lock()
		restore := s.focusMuted
		s.focusMuted = false
		s.mu.Unlock()
		if restore {
			_ = s.send([]any{"set_property", "mute", false})
		}
	}
}
func (m *nativePlayerManager) mediaAction(action, id string, value float64, track int, enabled bool) error {
	switch action {
	case "solo-audio":
		if e := m.setMediaPreference("focus-audio", true); e != nil {
			return e
		}
		m.focusMedia(id)
		return nil
	case "remember-media", "focus-audio":
		return m.setMediaPreference(action, enabled)
	case "clear-history":
		return m.cleanMediaHistory(true)
	case "resume-all":
		var failed int
		for _, s := range m.mediaSessions() {
			s.mu.Lock()
			video := s.view.Kind == "video"
			s.mu.Unlock()
			if video && s.send([]any{"set_property", "pause", false}) != nil {
				failed++
			}
		}
		if failed > 0 {
			return fmt.Errorf("%d 个窗口未能恢复播放", failed)
		}
		return nil
	}
	m.mu.Lock()
	s := m.sessions[id]
	m.mu.Unlock()
	if s == nil {
		return errors.New("原生播放窗口已关闭")
	}
	s.mu.Lock()
	kind := s.view.Kind
	tracks := append([]nativeMediaTrack{}, s.view.Tracks...)
	s.mu.Unlock()
	switch action {
	case "speed":
		if kind != "video" || math.IsNaN(value) || math.IsInf(value, 0) || value < 0.25 || value > 4 {
			return errors.New("视频倍速应在 0.25～4 之间")
		}
		return s.send([]any{"set_property", "speed", value})
	case "audio-track", "subtitle-track":
		if kind != "video" {
			return errors.New("此操作仅用于视频")
		}
		property, typ := "aid", "audio"
		if action == "subtitle-track" {
			property, typ = "sid", "sub"
		}
		if track == 0 {
			return s.send([]any{"set_property", property, "no"})
		}
		valid := false
		for _, t := range tracks {
			if t.ID == track && t.Type == typ {
				valid = true
				break
			}
		}
		if !valid {
			return errors.New("音轨或字幕已变化，请刷新播放器状态")
		}
		return s.send([]any{"set_property", property, track})
	case "rotate-left", "rotate-right":
		if kind != "image" {
			return errors.New("此操作仅用于图片")
		}
		step := 90
		if strings.HasSuffix(action, "left") {
			step = -90
		}
		s.mu.Lock()
		rotation := (s.view.Rotation + step + 360) % 360
		s.mu.Unlock()
		return s.send([]any{"set_property", "video-rotate", rotation})
	}
	return errors.New("不支持的播放器操作")
}
