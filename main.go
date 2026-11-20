package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

//go:embed web/*
var assets embed.FS

type state struct {
	Status                string
	HistoryID, RetryOf    string
	Error                 string
	Total, Done           int
	Logs                  []string
	Items                 []toolbox.Item
	Result                toolbox.Result
	PlanID                string
	Tool                  string
	Actionable, Conflicts int
	Operation             *toolbox.OperationProgress `json:",omitempty"`
}
type server struct {
	browser        *fileBrowser
	mu             sync.Mutex
	state          state
	plan           *toolbox.Plan
	cancel         context.CancelFunc
	token          string
	dataDir        string
	settingsMu     sync.Mutex
	passwordsMu    sync.Mutex
	historyMu      sync.Mutex
	desktopClosing bool
	taskWait       sync.WaitGroup
	autostartMu    sync.Mutex
	autostart      *autostartManager
	updates        *releaseChecker
}

func (s *server) line(m string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Logs = append(s.state.Logs, time.Now().Format("15:04:05")+" "+m)
	if len(s.state.Logs) > 3000 {
		s.state.Logs = s.state.Logs[len(s.state.Logs)-3000:]
	}
}
func (s *server) progress(n, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Done = n
	s.state.Total = total
	s.state.Operation = nil
}
func (s *server) operationProgress(p toolbox.OperationProgress) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Status != "running" || s.state.Tool != p.Tool {
		return
	}
	s.state.Operation = &p
}
func (s *server) busy() bool { return s.state.Status == "scanning" || s.state.Status == "running" }
func send(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, e error) { http.Error(w, e.Error(), 400) }
func (s *server) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Header.Get("X-Toolbox-Token") != s.token {
			http.Error(w, "无效会话，请从启动时的页面打开", 403)
			return
		}
		next(w, r)
	}
}
func (s *server) api(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST required", 405)
		return
	}
	switch r.URL.Path {
	case "/api/autostart":
		s.autostartAPI(w, r)
	case "/api/task-history", "/api/task-history-detail", "/api/task-history-clear":
		s.taskHistoryAPI(w, r)
	case "/api/cli-commands":
		s.cliCommandsAPI(w, r)
	case "/api/dedupe-info", "/api/dedupe-open":
		s.dedupeAPI(w, r)
	case "/api/passwords", "/api/password-get", "/api/password-save", "/api/password-delete":
		s.passwordAPI(w, r)
	case "/api/video-commands", "/api/native-commands":
		s.videoCommands(w, r)
	case "/api/info":
		send(w, appInfo())
	case "/api/version-update":
		s.versionUpdateAPI(w, r)
	case "/api/defaults":
		send(w, struct {
			toolbox.Config
			Platform               string
			PasswordVaultSupported bool
		}{toolbox.DefaultConfig(), runtime.GOOS, runtime.GOOS == "windows"})
	case "/api/number-info":
		var c toolbox.Config
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		if e := json.NewDecoder(r.Body).Decode(&c); e != nil {
			fail(w, e)
			return
		}
		info, e := toolbox.InspectNumbering(r.Context(), c)
		if e != nil {
			fail(w, e)
			return
		}
		send(w, info)
	case "/api/cache-clear":
		if s.browser == nil {
			fail(w, fmt.Errorf("资源管理器尚未就绪"))
			return
		}
		if err := s.browser.clearDiskThumbnails(); err != nil {
			fail(w, err)
			return
		}
		send(w, map[string]bool{"OK": true})
	case "/api/settings":
		s.settingsMu.Lock()
		p, e := s.loadPreferences()
		s.settingsMu.Unlock()
		if e != nil {
			fail(w, e)
			return
		}
		send(w, p)
	case "/api/save-settings":
		var c toolbox.Config
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		if e := json.NewDecoder(r.Body).Decode(&c); e != nil {
			fail(w, e)
			return
		}
		p, e := s.saveWorkspace(c)
		if e != nil {
			fail(w, e)
			return
		}
		send(w, p)
	case "/api/ui-settings":
		var change struct {
			BrowserAutoPlay     *bool
			BrowserMuted        *bool
			RememberPlayback    *bool
			RestoreTabs         *bool
			DefaultPlayer       *string
			FontScale           *int
			ThumbnailCacheMB    *int
			SidebarCollapsed    *bool
			ArchiveAdvancedOpen *bool
			TaskPopup           *bool
			TaskSound           *bool
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if e := decoder.Decode(&change); e != nil {
			fail(w, e)
			return
		}
		if change.DefaultPlayer != nil && *change.DefaultPlayer != "browser" && *change.DefaultPlayer != "mpv" {
			fail(w, fmt.Errorf("默认播放器无效"))
			return
		}
		if change.FontScale != nil && *change.FontScale != 100 && *change.FontScale != 110 && *change.FontScale != 125 {
			fail(w, fmt.Errorf("字号无效"))
			return
		}
		if change.ThumbnailCacheMB != nil && (*change.ThumbnailCacheMB < 0 || *change.ThumbnailCacheMB > 4096) {
			fail(w, fmt.Errorf("缩略图缓存须为 0–4096 MB"))
			return
		}
		s.settingsMu.Lock()
		p, e := s.loadPreferences()
		if e == nil {
			if change.SidebarCollapsed != nil {
				p.SidebarCollapsed = *change.SidebarCollapsed
			}
			if change.ArchiveAdvancedOpen != nil {
				p.ArchiveAdvancedOpen = *change.ArchiveAdvancedOpen
			}
			if change.TaskPopup != nil {
				p.TaskPopup = change.TaskPopup
			}
			if change.TaskSound != nil {
				p.TaskSound = change.TaskSound
			}
			if change.BrowserAutoPlay != nil {
				p.BrowserAutoPlay = change.BrowserAutoPlay
			}
			if change.BrowserMuted != nil {
				p.BrowserMuted = change.BrowserMuted
			}
			if change.RememberPlayback != nil {
				p.RememberPlayback = change.RememberPlayback
			}
			if change.RestoreTabs != nil {
				p.RestoreTabs = change.RestoreTabs
			}
			if change.DefaultPlayer != nil {
				p.DefaultPlayer = *change.DefaultPlayer
			}
			if change.FontScale != nil {
				p.FontScale = *change.FontScale
			}
			if change.ThumbnailCacheMB != nil {
				p.ThumbnailCacheMB = change.ThumbnailCacheMB
			}
			e = s.writePreferences(p)
		}
		s.settingsMu.Unlock()
		if e != nil {
			fail(w, e)
			return
		}
		send(w, p)
	case "/api/dependencies":
		var req struct {
			Config toolbox.Config
			Verify bool
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		if e := json.NewDecoder(r.Body).Decode(&req); e != nil {
			fail(w, e)
			return
		}
		if !toolbox.ToolExists(req.Config.Tool) {
			fail(w, fmt.Errorf("未知工具"))
			return
		}
		send(w, toolbox.CheckDependencies(r.Context(), req.Config, req.Verify))
	case "/api/state":
		var req struct{ Summary bool }
		if e := json.NewDecoder(r.Body).Decode(&req); e != nil {
			fail(w, e)
			return
		}
		s.mu.Lock()
		snapshot := s.state
		snapshot.Logs = append([]string{}, s.state.Logs...)
		if req.Summary {
			snapshot.Items = nil
		}
		s.mu.Unlock()
		send(w, snapshot)
	case "/api/preview":
		s.preview(w, r)
	case "/api/browse":
		var req struct{ Path, Kind string }
		if e := json.NewDecoder(r.Body).Decode(&req); e != nil {
			fail(w, e)
			return
		}
		if req.Path == "" {
			drives := browserLocations()
			send(w, map[string]interface{}{"Path": "", "Parent": "", "Dirs": drives, "Files": []string{}})
			return
		}
		p, e := filepath.Abs(req.Path)
		if e != nil {
			fail(w, e)
			return
		}
		entries, e := os.ReadDir(p)
		if e != nil {
			fail(w, e)
			return
		}
		dirs := []string{}
		files := []string{}
		for _, d := range entries {
			if d.IsDir() && d.Type()&os.ModeSymlink == 0 {
				dirs = append(dirs, filepath.Join(p, d.Name()))
			} else if d.Type()&os.ModeSymlink == 0 {
				ext := strings.ToLower(filepath.Ext(d.Name()))
				info, _ := d.Info()
				if req.Kind == "file" || req.Kind == "program" && info != nil && selectableProgram(d.Name(), info) || req.Kind == "font" && (ext == ".ttf" || ext == ".ttc" || ext == ".otf") {
					files = append(files, filepath.Join(p, d.Name()))
				}
			}
		}
		parent := filepath.Dir(p)
		if parent == p {
			parent = ""
		}
		send(w, map[string]interface{}{"Path": p, "Parent": parent, "Dirs": dirs, "Files": files})
	case "/api/plan":
		var req struct {
			toolbox.Config
			RetryTaskID string
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		if e := json.NewDecoder(r.Body).Decode(&req); e != nil {
			fail(w, e)
			return
		}
		c := req.Config
		s.mu.Lock()
		if s.busy() || s.desktopClosing {
			s.mu.Unlock()
			fail(w, fmt.Errorf("已有任务正在运行"))
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		s.cancel = cancel
		s.plan = nil
		s.state = state{Status: "scanning", Tool: c.Tool, RetryOf: req.RetryTaskID, Logs: []string{}, Items: []toolbox.Item{}}
		s.mu.Unlock()
		go func() {
			defer cancel()
			var p *toolbox.Plan
			var e error
			if req.RetryTaskID != "" {
				var sources []string
				sources, e = s.remainingHistorySources(ctx, req.RetryTaskID, c)
				if e == nil {
					p, e = toolbox.BuildRemainingPlan(ctx, c, sources, s.line)
				}
			} else {
				p, e = toolbox.BuildPlan(ctx, c, s.line)
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			if e != nil {
				s.state.Status = "error"
				if ctx.Err() != nil {
					s.state.Status = "cancelled"
				}
				s.state.Error = e.Error()
				return
			}
			s.plan = p
			s.state.Status = "ready"
			s.state.PlanID = randomID()
			s.state.Total = len(p.Items)
			for _, it := range p.Items {
				if it.Conflict != "" {
					s.state.Conflicts++
				} else {
					s.state.Actionable++
				}
			}
			s.state.Items = p.Items
			if len(s.state.Items) > 1000 {
				s.state.Items = s.state.Items[:1000]
			}
		}()
		send(w, map[string]bool{"ok": true})
	case "/api/execute":
		var req struct{ PlanID string }
		if e := json.NewDecoder(r.Body).Decode(&req); e != nil {
			fail(w, e)
			return
		}
		s.mu.Lock()
		if s.busy() || s.desktopClosing || s.state.Status != "ready" || req.PlanID == "" || req.PlanID != s.state.PlanID {
			s.mu.Unlock()
			fail(w, fmt.Errorf("请先重新预览"))
			return
		}
		p := s.plan
		if e := toolbox.CheckReady(p); e != nil {
			s.mu.Unlock()
			fail(w, e)
			return
		}
		journal, historyErr := s.newTaskJournal(p, s.state.PlanID, s.state.RetryOf)
		if historyErr != nil {
			s.mu.Unlock()
			fail(w, fmt.Errorf("无法保存任务记录，请检查数据目录：%w", historyErr))
			return
		}
		if journal != nil {
			s.state.HistoryID = journal.meta.ID
		}
		ctx, cancel := context.WithCancel(context.Background())
		s.cancel = cancel
		s.state.Status = "running"
		s.state.Done = 0
		s.state.Operation = nil
		s.taskWait.Add(1)
		s.mu.Unlock()
		go func() {
			defer s.taskWait.Done()
			defer cancel()
			runCtx := toolbox.WithOperationProgress(ctx, s.operationProgress)
			res := toolbox.ExecuteObserved(runCtx, p, s.line, s.progress, journal.observe)
			s.mu.Lock()
			s.state.Result = res
			s.state.Operation = nil
			s.state.Status = "finished"
			if ctx.Err() != nil {
				s.state.Status = "cancelled"
			}
			snapshot := s.state
			snapshot.Logs = append([]string{}, s.state.Logs...)
			s.mu.Unlock()
			if e := journal.finish(snapshot); e != nil {
				s.line("任务历史保存失败：" + e.Error())
			}
			if e := s.pruneTaskHistory(false); e != nil {
				s.line("历史记录清理失败：" + e.Error())
			}
		}()
		send(w, map[string]bool{"ok": true})
	case "/api/cancel":
		s.mu.Lock()
		if s.cancel != nil {
			s.cancel()
		}
		s.mu.Unlock()
		send(w, map[string]bool{"ok": true})
	case "/api/log":
		s.mu.Lock()
		defer s.mu.Unlock()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, strings.Join(s.state.Logs, "\n"))
	default:
		http.NotFound(w, r)
	}
}
func randomID() string {
	b := make([]byte, 24)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func main() {
	if len(os.Args) > 1 && os.Args[1] == "cli" {
		release, _, err := prepareDesktop("", false, true)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		code := cliMain(os.Args[2:])
		release()
		os.Exit(code)
	}
	port := flag.Int("port", 0, "本地监听端口，0 为自动分配")
	noOpen := flag.Bool("no-open", false, "不自动打开浏览器")
	tray := flag.Bool("tray", desktopMode == "true", "在系统托盘运行（Windows）")
	dataDir := flag.String("data", "", "任务日志目录")
	showVersion := flag.Bool("version", false, "显示版本并退出")
	checkTools := flag.Bool("check-tools", false, "检查随包依赖并输出 JSON")
	selfTest := flag.Bool("self-test", false, "在临时目录测试随包组件，不启动界面")
	flag.Parse()
	if *showVersion {
		fmt.Printf("Hamster Toolbox %s (%s)\n", appVersion, appInfo()["Platform"])
		return
	}
	if *checkTools || *selfTest {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		deps := toolbox.CheckPortableBundle(ctx)
		ready := true
		warning := toolbox.PortableBundleWarning(toolsManifestSHA256)
		if warning != "" {
			ready = false
		}
		for _, d := range deps {
			if !d.Available {
				ready = false
			}
		}
		if *checkTools {
			json.NewEncoder(os.Stdout).Encode(map[string]any{"Version": appVersion, "Ready": ready, "Warning": warning, "Dependencies": deps})
		}
		if !ready {
			if warning != "" {
				fmt.Fprintln(os.Stderr, warning)
			}
			if *selfTest {
				for _, d := range deps {
					if !d.Available {
						fmt.Fprintln(os.Stderr, d.Name+": "+d.Message)
					}
				}
			}
			os.Exit(1)
		}
		if *selfTest {
			if e := toolbox.SelfTestPortable(ctx, func(s string) { fmt.Println(s) }); e != nil {
				fmt.Fprintln(os.Stderr, e)
				os.Exit(1)
			}
			fmt.Println("PASS: portable bundle self-test")
		}
		return
	}
	exe, _ := os.Executable()
	if *dataDir == "" {
		if userDir, e := os.UserConfigDir(); e == nil {
			*dataDir = filepath.Join(userDir, "media-toolbox")
		} else {
			*dataDir = filepath.Join(filepath.Dir(exe), "data")
		}
	}
	releaseDesktop, existingDesktop, desktopErr := prepareDesktop(*dataDir, *tray, *noOpen)
	if desktopErr != nil {
		desktopError(desktopErr.Error())
		return
	}
	if existingDesktop {
		return
	}
	defer releaseDesktop()
	toolbox.ProtectDataDirectory(*dataDir)
	color, restoreConsole := enableConsoleColor()
	defer restoreConsole()
	console := startupConsole{out: os.Stdout, color: color}
	s := &server{token: randomID(), dataDir: *dataDir, state: state{Status: "idle", Logs: []string{}, Items: []toolbox.Item{}}}
	browser, e := newFileBrowser(*dataDir, s.token)
	s.browser = browser
	sharingErr := e
	if sharingErr == nil {
		sharingErr = browser.startSavedSharing()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/files/", browser.local)
	mux.HandleFunc("/api/", s.guard(s.api))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'")
		p := r.URL.Path
		if p == "/" {
			if r.URL.Query().Get("session") != s.token {
				http.Error(w, "请使用程序启动时提供的链接", 403)
				return
			}
			p = "/index.html"
		}
		brandType := brandContentType(strings.TrimPrefix(p, "/"))
		if p != "/index.html" && p != "/app.js" && p != "/version-info.js" && p != "/task-feedback.js" && p != "/task-history.js" && p != "/general-settings.css" && p != "/style.css" && p != "/theme.css" && p != "/toolbox-layout.css" && brandType == "" {
			http.NotFound(w, r)
			return
		}
		if p == "/index.html" && r.URL.Query().Get("session") != s.token {
			http.Error(w, "无效会话", 403)
			return
		}
		b, e := assets.ReadFile("web" + p)
		if e != nil {
			http.NotFound(w, r)
			return
		}
		if p == "/index.html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		} else if strings.HasSuffix(p, ".js") {
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		} else if brandType != "" {
			w.Header().Set("Content-Type", brandType)
		} else {
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		}
		w.Write(b)
	})
	listener, e := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if e != nil {
		if *tray {
			desktopError(fmt.Sprintf("无法开启本机端口 %d：%v", *port, e))
			return
		}
		failure := startupConsole{out: os.Stderr}
		failure.listenFailure(*port, e)
		os.Exit(1)
	}
	url := "http://" + listener.Addr().String() + "/?session=" + s.token
	browserMessage := "已跳过自动打开 · 请使用下方链接"
	var browserErr error
	if !*noOpen {
		cmd := openBrowserCommand(url)
		browserErr = cmd.Start()
		if browserErr == nil {
			browserMessage = "已请求打开默认浏览器 · 未弹出时可使用下方链接"
			go cmd.Wait()
		}
	}
	console.ready(url, s.dataDir, browserMessage, browserErr, sharingErr, browser)
	httpServer := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	if *tray {
		if e := runDesktopTray(url, s, httpServer, listener); e != nil {
			desktopError(e.Error())
		}
		listener.Close()
		browser.mu.Lock()
		sharedServer := browser.server
		browser.mu.Unlock()
		if sharedServer != nil {
			sharedServer.Close()
		}
		// Let completed-task journaling and cancelled file transfers finish
		// their cleanup before a normal tray exit terminates the process.
		s.taskWait.Wait()
		browser.operations.Lock()
		browser.operations.Unlock()
		return
	}
	if e := httpServer.Serve(listener); e != nil && e != http.ErrServerClosed {
		console.status("31", "服务停止", e.Error())
		os.Exit(1)
	}
}
