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
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//go:embed web/*
var assets embed.FS

type state struct {
	Status                string
	Error                 string
	Total, Done           int
	Logs                  []string
	Items                 []toolbox.Item
	Result                toolbox.Result
	PlanID                string
	Tool                  string
	Actionable, Conflicts int
}
type server struct {
	mu          sync.Mutex
	state       state
	plan        *toolbox.Plan
	cancel      context.CancelFunc
	token       string
	dataDir     string
	settingsMu  sync.Mutex
	passwordsMu sync.Mutex
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
	case "/api/defaults":
		send(w, toolbox.DefaultConfig())
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
			drives := []string{}
			for c := 'C'; c <= 'Z'; c++ {
				p := fmt.Sprintf("%c:\\", c)
				if st, e := os.Stat(p); e == nil && st.IsDir() {
					drives = append(drives, p)
				}
			}
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
				if req.Kind == "file" || req.Kind == "program" && ext == ".exe" || req.Kind == "font" && (ext == ".ttf" || ext == ".ttc" || ext == ".otf") {
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
		var c toolbox.Config
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		if e := json.NewDecoder(r.Body).Decode(&c); e != nil {
			fail(w, e)
			return
		}
		s.mu.Lock()
		if s.busy() {
			s.mu.Unlock()
			fail(w, fmt.Errorf("已有任务正在运行"))
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		s.cancel = cancel
		s.plan = nil
		s.state = state{Status: "scanning", Tool: c.Tool, Logs: []string{}, Items: []toolbox.Item{}}
		s.mu.Unlock()
		go func() {
			defer cancel()
			p, e := toolbox.BuildPlan(ctx, c, s.line)
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
		if s.busy() || s.state.Status != "ready" || req.PlanID == "" || req.PlanID != s.state.PlanID {
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
		ctx, cancel := context.WithCancel(context.Background())
		s.cancel = cancel
		s.state.Status = "running"
		s.state.Done = 0
		s.mu.Unlock()
		go func() {
			defer cancel()
			res := toolbox.Execute(ctx, p, s.line, s.progress)
			s.mu.Lock()
			s.state.Result = res
			s.state.Status = "finished"
			if ctx.Err() != nil {
				s.state.Status = "cancelled"
			}
			snapshot := s.state
			s.mu.Unlock()
			os.MkdirAll(s.dataDir, 0755)
			recordPlan := *p
			recordPlan.Config.ArchivePassword = ""
			recordPlan.Config.ExtractPassword = ""
			data, _ := json.MarshalIndent(struct {
				State state
				Plan  *toolbox.Plan
			}{snapshot, &recordPlan}, "", "  ")
			if e := os.WriteFile(filepath.Join(s.dataDir, time.Now().Format("20060102-150405")+"-"+randomID()[:6]+".json"), data, 0644); e != nil {
				s.line("日志保存失败：" + e.Error())
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
		os.Exit(cliMain(os.Args[2:]))
	}
	port := flag.Int("port", 0, "本地监听端口，0 为自动分配")
	noOpen := flag.Bool("no-open", false, "不自动打开浏览器")
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
	toolbox.ProtectDataDirectory(*dataDir)
	s := &server{token: randomID(), dataDir: *dataDir, state: state{Status: "idle", Logs: []string{}, Items: []toolbox.Item{}}}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/", s.guard(s.api))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'")
		p := r.URL.Path
		if p == "/" {
			if r.URL.Query().Get("session") != s.token {
				http.Error(w, "请使用程序启动时提供的链接", 403)
				return
			}
			p = "/index.html"
		}
		if p != "/index.html" && p != "/app.js" && p != "/style.css" {
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
		} else if p == "/app.js" {
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		} else {
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		}
		w.Write(b)
	})
	listener, e := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if e != nil {
		log.Fatal(e)
	}
	url := "http://" + listener.Addr().String() + "/?session=" + s.token
	fmt.Println("仓鼠工具箱 " + appVersion + " 已启动：" + url + "\n关闭此窗口即可退出。任务日志：" + s.dataDir)
	if !*noOpen {
		cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		if e = cmd.Start(); e != nil {
			fmt.Println("请手动打开链接：", e)
		} else {
			go cmd.Wait()
		}
	}
	httpServer := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(httpServer.Serve(listener))
}
