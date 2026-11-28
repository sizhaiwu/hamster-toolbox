package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

type nativeWindowMemory struct{ Width, Height int }
type nativePlayerConfig struct {
	Program, WindowSize string
	RememberWindows     *bool
	Windows             map[string]nativeWindowMemory
}
type nativePlaybackView struct {
	ID, Name, Kind, Decoder, Codec, Path   string
	Connected, Paused, Muted, OnTop        bool
	Time, Duration                         float64
	Volume                                 float64
	Width, Height                          int
	Speed                                  float64
	DroppedFrames, VideoWidth, VideoHeight int
	RenderDroppedFrames, Rotation          int
	DropMetricsKnown, RenderMetricsKnown   bool
	Focused                                bool
	Tracks                                 []nativeMediaTrack
}
type nativePlayback struct {
	mu                    sync.Mutex
	write                 sync.Mutex
	view                  nativePlaybackView
	cmd                   *exec.Cmd
	conn                  net.Conn
	endpoint, tempDir     string
	pending               map[int]chan error
	nextRequest           int
	fullscreen, maximized bool
	memoryTimer           *time.Timer
	memoryEpoch           uint64
	windowDirty           bool
	memorySuppressedUntil time.Time
	focusMuted            bool
	controlsOptions       string
	controlsWrite         sync.Mutex
}
type nativePlayerManager struct {
	mu                                      sync.Mutex
	configuration                           sync.Mutex
	program, dataDir, lastError, windowSize string
	sessions                                map[string]*nativePlayback
	rememberWindows                         bool
	windowMemory                            map[string]nativeWindowMemory
	memoryEpoch                             uint64
	mediaPreferences                        nativeMediaPreferences
	mediaFocus                              sync.Mutex
}

func (b *fileBrowser) nativePlayer() *nativePlayerManager {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.native == nil {
		b.native = &nativePlayerManager{dataDir: b.dataDir, windowSize: "compact", rememberWindows: true, windowMemory: map[string]nativeWindowMemory{}, sessions: map[string]*nativePlayback{}}
		b.native.loadMediaPreferences()
		data, err := os.ReadFile(filepath.Join(b.dataDir, "native-player.json"))
		if err == nil {
			var c nativePlayerConfig
			if json.Unmarshal(data, &c) == nil {
				b.native.program = c.Program
				if c.RememberWindows != nil {
					b.native.rememberWindows = *c.RememberWindows
				}
				for kind, size := range c.Windows {
					if (kind == "image" || kind == "video") && validNativeWindowMemory(size) {
						b.native.windowMemory[kind] = size
					}
				}
				if c.WindowSize == "medium" || c.WindowSize == "large" {
					b.native.windowSize = c.WindowSize
				}
			}
		}
	}
	return b.native
}
func resolveNativePlayer(configured string) string {
	exe, _ := os.Executable()
	base := filepath.Dir(exe)
	if configured != "" {
		if !filepath.IsAbs(configured) {
			configured = filepath.Join(base, configured)
		}
		if st, e := os.Stat(configured); e == nil && st.Mode().IsRegular() {
			return configured
		}
	}
	name := "mpv"
	if runtime.GOOS == "windows" {
		name = "mpv.exe"
	}
	for _, candidate := range []string{filepath.Join(base, "tools", "mpv", name), filepath.Join(base, "tools", name)} {
		if st, e := os.Stat(candidate); e == nil && st.Mode().IsRegular() {
			return candidate
		}
	}
	if found, e := exec.LookPath(name); e == nil {
		full, _ := filepath.Abs(found)
		return full
	}
	return ""
}
func (m *nativePlayerManager) status() map[string]any {
	m.mu.Lock()
	program, lastError, windowSize, remember := m.program, m.lastError, m.windowSize, m.rememberWindows
	sessions := make([]*nativePlayback, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.mu.Unlock()
	views := []nativePlaybackView{}
	for _, session := range sessions {
		session.mu.Lock()
		views = append(views, session.view)
		session.mu.Unlock()
	}
	sort.Slice(views, func(i, j int) bool {
		if views[i].Name == views[j].Name {
			return views[i].ID < views[j].ID
		}
		return views[i].Name < views[j].Name
	})
	resolved := resolveNativePlayer(program)
	m.mu.Lock()
	mediaPreferences := m.mediaPreferences
	m.mu.Unlock()
	return map[string]any{"Available": resolved != "", "Program": resolved, "WindowSize": windowSize, "RememberWindows": remember, "RememberMedia": mediaPreferences.Remember, "FocusAudio": mediaPreferences.FocusAudio, "ControlsMode": nativeControlsMode(mediaPreferences.Controls), "EndAction": nativeEndAction(mediaPreferences.EndAction), "Error": lastError, "Sessions": views, "DownloadURL": "https://mpv.io/installation/"}
}
func (m *nativePlayerManager) configure(program string) error {
	m.configuration.Lock()
	defer m.configuration.Unlock()
	program = strings.Trim(strings.TrimSpace(program), "\"")
	if program != "" {
		if err := validateExternalProgram(program); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, program, "--version")
		prepareNativePlayerCommand(cmd)
		out := &nativePlayerBuffer{}
		cmd.Stdout = out
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err != nil || !strings.HasPrefix(strings.TrimSpace(out.String()), "mpv ") {
			return errors.New("请选择 mpv 程序文件，当前程序未通过版本检查")
		}
	}
	stored := program
	exe, _ := os.Executable()
	base := filepath.Dir(exe)
	if relative, e := filepath.Rel(base, program); program != "" && e == nil && strings.HasPrefix(filepath.ToSlash(relative), "tools/") {
		stored = filepath.ToSlash(relative)
	}
	m.mu.Lock()
	windowSize := m.windowSize
	m.mu.Unlock()
	return m.saveConfiguration(stored, windowSize)
}

func (m *nativePlayerManager) setWindowSize(size string) error {
	if size != "compact" && size != "medium" && size != "large" {
		return errors.New("请选择紧凑、中等或较大的初始窗口")
	}
	m.configuration.Lock()
	defer m.configuration.Unlock()
	m.mu.Lock()
	program := m.program
	m.mu.Unlock()
	return m.saveConfiguration(program, size)
}

// Called under configuration, so changing size and program cannot lose each other's updates.
func (m *nativePlayerManager) saveConfiguration(program, size string) error {
	m.mu.Lock()
	remember := m.rememberWindows
	windows := cloneNativeWindowMemory(m.windowMemory)
	reset := size != m.windowSize
	if reset {
		windows = map[string]nativeWindowMemory{}
	}
	m.mu.Unlock()
	return m.saveWindowConfiguration(program, size, remember, windows, reset)
}
func (m *nativePlayerManager) saveWindowConfiguration(program, size string, remember bool, windows map[string]nativeWindowMemory, reset bool) error {
	if size == "" {
		size = "compact"
	}
	if err := os.MkdirAll(m.dataDir, 0700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(nativePlayerConfig{Program: program, WindowSize: size, RememberWindows: &remember, Windows: windows}, "", "  ")
	tmp, e := os.CreateTemp(m.dataDir, ".native-player-*.tmp")
	if e != nil {
		return e
	}
	defer os.Remove(tmp.Name())
	if _, e = tmp.Write(data); e != nil {
		tmp.Close()
		return e
	}
	if e = tmp.Close(); e != nil {
		return e
	}
	if e = os.Rename(tmp.Name(), filepath.Join(m.dataDir, "native-player.json")); e != nil {
		return e
	}
	m.mu.Lock()
	m.program = program
	m.windowSize = size
	m.rememberWindows = remember
	m.windowMemory = windows
	if reset {
		m.memoryEpoch++
	}
	m.lastError = ""
	m.mu.Unlock()
	return nil
}

// Bounded diagnostics only; never let a player flood the service's memory.
type nativePlayerBuffer struct {
	mu   sync.Mutex
	data []byte
}

func (b *nativePlayerBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, p...)
	if len(b.data) > 8192 {
		b.data = append([]byte{}, b.data[len(b.data)-8192:]...)
	}
	return len(p), nil
}
func (b *nativePlayerBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return string(b.data) }
func nativePlayerArgs(file, endpoint string, index int, size string) []string {
	fit := "800x600"
	if size == "medium" {
		fit = "1100x825"
	} else if size == "large" {
		fit = "75%x80%"
	}
	args := []string{"--no-config", "--load-scripts=no", "--ytdl=no", "--input-terminal=no", "--terminal=no", "--hwdec=auto", "--vo=gpu-next", "--ontop=yes", "--border=no", "--keep-open=yes", "--keepaspect-window=no", "--auto-window-resize=no", "--autofit=" + fit, "--autofit-larger=90%x90%", "--window-dragging=yes", "--osc=no", "--osd-font-size=28", "--osd-scale-by-window=no", "--script-opts=hamster-mode=full", "--mute=no", "--input-media-keys=no", "--sub-auto=no", "--audio-file-auto=no", "--title=仓鼠播放器 · ${filename}", "--input-ipc-server=" + endpoint}
	if browserKind(file) == "image" {
		args = append(args, "--image-display-duration=inf")
	}
	if runtime.GOOS == "windows" {
		args = append(args, "--gpu-context=d3d11")
	}
	args = append(args, fmt.Sprintf("--geometry=+%d+%d", 40+(index%5)*24, 40+(index%5)*24), "--", file)
	return args
}
func (m *nativePlayerManager) open(file, kind string) error {
	m.mu.Lock()
	program := resolveNativePlayer(m.program)
	windowSize := m.windowSize
	memory, remember, epoch := m.windowMemory[kind], m.rememberWindows, m.memoryEpoch
	index := len(m.sessions)
	m.mu.Unlock()
	if program == "" {
		return errors.New("未找到 mpv，请在原生播放器设置中选择程序或按指引下载")
	}
	id := randomID()
	endpoint, dir, err := nativePlayerEndpoint(id)
	if err != nil {
		return err
	}
	controller, controllerDir, err := prepareNativeController(dir)
	if err != nil {
		return fmt.Errorf("无法准备原生播放控件：%w", err)
	}
	dir = controllerDir
	args := nativePlayerArgs(file, endpoint, index, windowSize)
	args = append(args[:len(args)-2], "--script="+controller, "--", file)
	args = m.mediaPlaybackArgs(args, kind)
	if remember && validNativeWindowMemory(memory) {
		args = replaceNativeFit(args, memory)
	}
	cmd := exec.Command(program, args...)
	prepareNativePlayerCommand(cmd)
	diagnostics := &nativePlayerBuffer{}
	cmd.Stderr = diagnostics
	cmd.Stdout = io.Discard
	s := &nativePlayback{view: nativePlaybackView{ID: id, Name: filepath.Base(file), Path: file, Kind: kind, Muted: false, OnTop: true, Volume: 100, Speed: 1}, cmd: cmd, endpoint: endpoint, tempDir: dir, memoryEpoch: epoch}
	if err = cmd.Start(); err != nil {
		if dir != "" {
			os.Remove(endpoint)
			removeNativeController(dir)
		}
		return fmt.Errorf("无法启动 mpv：%w", err)
	}
	m.mu.Lock()
	m.sessions[id] = s
	m.lastError = ""
	m.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		err := cmd.Wait()
		cancel()
		s.mu.Lock()
		conn := s.conn
		s.conn = nil
		s.view.Connected = false
		s.mu.Unlock()
		if conn != nil {
			conn.Close()
		}
		if s.tempDir != "" {
			os.Remove(s.endpoint)
			removeNativeController(s.tempDir)
		}
		m.mu.Lock()
		delete(m.sessions, id)
		if err != nil {
			m.lastError = "原生播放器已退出：" + strings.TrimSpace(diagnostics.String())
			if len(m.lastError) > 1500 {
				m.lastError = m.lastError[:1500]
			}
		}
		m.mu.Unlock()
	}()
	go s.monitor(ctx, m)
	return nil
}
func (s *nativePlayback) send(command []any) error {
	s.write.Lock()
	defer s.write.Unlock()
	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if conn == nil {
		return errors.New("播放器正在启动或已关闭")
	}
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	return json.NewEncoder(conn).Encode(map[string]any{"command": command})
}
func (s *nativePlayback) monitor(parent context.Context, manager *nativePlayerManager) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	var conn net.Conn
	for ctx.Err() == nil {
		candidate, err := dialNativePlayer(ctx, s.endpoint)
		if err == nil {
			conn = candidate
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(80 * time.Millisecond):
		}
	}
	if conn == nil {
		return
	}
	s.mu.Lock()
	s.conn = conn
	s.view.Connected = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if s.memoryTimer != nil {
			s.memoryTimer.Stop()
		}
		s.mu.Unlock()
		manager.rememberWindow(s)
		conn.Close()
		s.mu.Lock()
		if s.conn == conn {
			s.conn = nil
			s.view.Connected = false
		}
		s.mu.Unlock()
	}()
	for index, name := range []string{"hwdec-current", "video-codec", "time-pos", "duration", "pause", "mute", "ontop", "volume", "fullscreen", "window-maximized", "osd-dimensions", "speed", "track-list", "decoder-frame-drop-count", "frame-drop-count", "video-params/w", "video-params/h", "focused", "video-rotate"} {
		if s.send([]any{"observe_property", index + 1, name}) != nil {
			return
		}
	}
	if s.view.Kind == "image" {
		for _, binding := range [][2]string{{"WHEEL_UP", "add video-zoom 0.15"}, {"WHEEL_DOWN", "add video-zoom -0.15"}, {"0", "set video-zoom 0; set video-pan-x 0; set video-pan-y 0"}, {"r", "cycle-values video-rotate 0 90 180 270"}, {"R", "cycle-values video-rotate 0 270 180 90"}} {
			_ = s.send([]any{"keybind", binding[0], binding[1]})
		}
	} else {
		for _, binding := range [][2]string{{"UP", "add volume 5"}, {"DOWN", "add volume -5"}} {
			_ = s.send([]any{"keybind", binding[0], binding[1]})
		}
	}
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 4096), 128<<10)
	for scanner.Scan() {
		var message struct {
			Event, Name string
			Data        json.RawMessage
			RequestID   int `json:"request_id"`
			Error       string
		}
		if json.Unmarshal(scanner.Bytes(), &message) != nil {
			continue
		}
		if message.RequestID != 0 {
			s.mu.Lock()
			ch := s.pending[message.RequestID]
			s.mu.Unlock()
			if ch != nil {
				var err error
				if message.Error != "success" {
					err = fmt.Errorf("mpv：%s", message.Error)
				}
				select {
				case ch <- err:
				default:
				}
			}
			continue
		}
		if message.Event != "property-change" {
			continue
		}
		s.mu.Lock()
		focused := s.updateMediaProperty(message.Name, message.Data)
		switch message.Name {
		case "hwdec-current":
			json.Unmarshal(message.Data, &s.view.Decoder)
		case "video-codec":
			json.Unmarshal(message.Data, &s.view.Codec)
		case "time-pos":
			json.Unmarshal(message.Data, &s.view.Time)
		case "duration":
			json.Unmarshal(message.Data, &s.view.Duration)
		case "pause":
			json.Unmarshal(message.Data, &s.view.Paused)
		case "mute":
			json.Unmarshal(message.Data, &s.view.Muted)
		case "ontop":
			json.Unmarshal(message.Data, &s.view.OnTop)
		case "volume":
			json.Unmarshal(message.Data, &s.view.Volume)
		case "fullscreen":
			json.Unmarshal(message.Data, &s.fullscreen)
		case "window-maximized":
			json.Unmarshal(message.Data, &s.maximized)
		case "osd-dimensions":
			var d struct{ W, H int }
			if json.Unmarshal(message.Data, &d) == nil && d.W > 0 && d.H > 0 {
				changed := s.view.Width > 0 && (s.view.Width != d.W || s.view.Height != d.H)
				s.view.Width, s.view.Height = d.W, d.H
				if changed && !s.fullscreen && !s.maximized && time.Now().After(s.memorySuppressedUntil) {
					s.windowDirty = true
					if s.memoryTimer != nil {
						s.memoryTimer.Stop()
					}
					s.memoryTimer = time.AfterFunc(700*time.Millisecond, func() { manager.rememberWindow(s) })
				}
			}
		}
		s.mu.Unlock()
		if focused {
			go manager.focusMedia(s.view.ID)
		}
	}
}
func (b *fileBrowser) nativePlayerAPI(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action, ID, Path, Version, Program, WindowSize string
		ControlsMode                                   string
		EndAction                                      string
		Track                                          int
		Value                                          float64
		Enabled                                        bool
	}
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(&req) != nil {
		fail(w, errors.New("原生播放器请求无效"))
		return
	}
	m := b.nativePlayer()
	var err error
	switch req.Action {
	case "end-action":
		err = m.setEndAction(req.EndAction)
	case "controls-mode":
		err = m.setControlsMode(req.ControlsMode)
	case "speed", "audio-track", "subtitle-track", "rotate-left", "rotate-right", "resume-all", "focus-audio", "solo-audio", "remember-media", "clear-history":
		err = m.mediaAction(req.Action, req.ID, req.Value, req.Track, req.Enabled)
	case "status":
	case "configure":
		err = m.configure(req.Program)
	case "window-size":
		err = m.setWindowSize(req.WindowSize)
	case "remember-windows":
		err = m.setRememberWindows(req.Enabled)
	case "tile", "cascade", "pause-all", "mute-all":
		err = m.controlWindows(req.Action)
	case "open":
		kind := browserKind(req.Path)
		if !filepath.IsAbs(req.Path) || strings.ContainsFunc(req.Path, unicode.IsControl) || (kind != "video" && kind != "image" && !browserAmbiguousSource(req.Path)) {
			err = errors.New("请选择资源管理器中的本机图片或视频")
			break
		}
		b.mu.Lock()
		ref, ok := b.references[req.Version]
		b.mu.Unlock()
		if !ok || time.Since(ref.At) > 15*time.Minute {
			err = errors.New("选择已过期，请刷新目录后重试")
			break
		}
		if err = ref.Ref.Check(req.Path); err != nil {
			break
		}
		var f *os.File
		f, err = toolbox.BrowseOpen(req.Path)
		if err != nil {
			break
		}
		st, e := f.Stat()
		if e != nil || !st.Mode().IsRegular() {
			f.Close()
			err = errors.New("只支持普通图片或视频文件")
			break
		}
		kind = browserFileKind(f, st)
		if kind != "video" && kind != "image" {
			f.Close()
			err = errors.New("请选择资源管理器中的本机图片或视频")
			break
		}
		if err = ref.Ref.Check(f.Name()); err == nil {
			err = m.open(f.Name(), kind)
		}
		f.Close()
	case "pause", "resume", "mute", "unmute", "pin", "unpin", "seek", "volume", "zoom-in", "zoom-out", "zoom-reset", "close":
		m.mu.Lock()
		session := m.sessions[req.ID]
		m.mu.Unlock()
		if session == nil {
			err = errors.New("原生播放窗口已关闭")
			break
		}
		var command []any
		switch req.Action {
		case "zoom-in", "zoom-out", "zoom-reset":
			session.mu.Lock()
			image := session.view.Kind == "image"
			session.mu.Unlock()
			if !image {
				err = errors.New("此操作仅用于图片")
				break
			}
			if req.Action == "zoom-reset" {
				for _, property := range []string{"video-zoom", "video-pan-x", "video-pan-y"} {
					if err = session.send([]any{"set_property", property, 0}); err != nil {
						break
					}
				}
			} else {
				step := 0.15
				if req.Action == "zoom-out" {
					step = -step
				}
				command = []any{"add", "video-zoom", step}
			}
		case "volume":
			if math.IsNaN(req.Value) || math.IsInf(req.Value, 0) || req.Value < 0 || req.Value > 100 {
				err = errors.New("音量应在 0～100 之间")
			} else {
				command = []any{"set_property", "volume", req.Value}
			}
		case "pause":
			command = []any{"set_property", "pause", true}
		case "resume":
			session.mu.Lock()
			ended := session.view.Duration > 0 && session.view.Time >= session.view.Duration-0.1
			session.mu.Unlock()
			if ended {
				err = session.send([]any{"seek", 0, "absolute"})
			}
			command = []any{"set_property", "pause", false}
		case "mute":
			session.mu.Lock()
			session.focusMuted = false
			session.mu.Unlock()
			command = []any{"set_property", "mute", true}
		case "unmute":
			session.mu.Lock()
			session.focusMuted = false
			session.mu.Unlock()
			command = []any{"set_property", "mute", false}
		case "pin":
			command = []any{"set_property", "ontop", true}
		case "unpin":
			command = []any{"set_property", "ontop", false}
		case "seek":
			if math.IsNaN(req.Value) || math.IsInf(req.Value, 0) || req.Value < 0 || req.Value > 7*24*3600 {
				err = errors.New("播放位置无效")
			} else {
				command = []any{"seek", req.Value, "absolute"}
			}
		case "close":
			command = []any{"quit"}
		}
		if err == nil && command != nil {
			err = session.send(command)
			if err != nil && req.Action == "close" {
				err = session.cmd.Process.Kill()
			}
		}
	default:
		err = errors.New("不支持的播放器操作")
	}
	if err != nil {
		fail(w, err)
		return
	}
	send(w, m.status())
}
