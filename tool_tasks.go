package main

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

type taskRouteKey struct{}
type taskExecutor func(context.Context, *toolbox.Plan, func(string), func(int, int), func(toolbox.FileOutcome)) toolbox.Result

func (s *server) runTask(ctx context.Context, p *toolbox.Plan, log func(string), progress func(int, int), observe func(toolbox.FileOutcome)) toolbox.Result {
	if run := s.taskRoot().taskExecutor; run != nil {
		return run(ctx, p, log, progress, observe)
	}
	return toolbox.ExecuteObserved(ctx, p, log, progress, observe)
}

// Each tool has a task-only handler with its own mutex, plan, cancellation and
// live state. Shared settings, authentication, notes and history remain owned
// by the root server. No server or mutex is copied.
func (s *server) taskRoot() *server {
	if s.taskOwner != nil {
		return s.taskOwner
	}
	return s
}
func (s *server) taskSlots() []*server {
	r := s.taskRoot()
	r.tasksMu.Lock()
	defer r.tasksMu.Unlock()
	slots := []*server{r}
	for _, slot := range r.tasks {
		slots = append(slots, slot)
	}
	return slots
}
func (s *server) toolTask(tool string) *server {
	r := s.taskRoot()
	r.tasksMu.Lock()
	defer r.tasksMu.Unlock()
	if r.tasks == nil {
		r.tasks = map[string]*server{}
	}
	if r.tasks[tool] == nil {
		r.tasks[tool] = &server{taskOwner: r, dataDir: r.dataDir, state: state{Tool: tool, Status: "idle"}}
	}
	return r.tasks[tool]
}
func (s *server) routeToolTask(w http.ResponseWriter, r *http.Request) bool {
	if s.taskOwner != nil || r.Context().Value(taskRouteKey{}) != nil {
		return false
	}
	switch r.URL.Path {
	case "/api/state", "/api/plan", "/api/execute", "/api/cancel", "/api/log", "/api/preview", "/api/video-commands", "/api/native-commands":
	default:
		return false
	}
	tool := r.Header.Get("X-Toolbox-Tool")
	target := s
	if tool != "" {
		if !toolbox.ToolExists(tool) && tool != "dedupe" {
			fail(w, fmt.Errorf("未知工具"))
			return true
		}
		target = s.toolTask(tool)
	}
	if r.URL.Path == "/api/plan" || r.URL.Path == "/api/execute" || r.URL.Path == "/api/cancel" {
		s.taskDispatchMu.Lock()
		defer s.taskDispatchMu.Unlock()
	}
	target.api(w, r.WithContext(context.WithValue(r.Context(), taskRouteKey{}, true)))
	return true
}

type toolTaskSummary struct {
	Tool, Status, PlanID, TaskID, HistoryID, Error string
	Total, Done, Actionable, Conflicts             int
	Result                                         toolbox.Result
	Operation                                      *toolbox.OperationProgress `json:",omitempty"`
}

func (s *server) taskSummaries() []toolTaskSummary {
	var result []toolTaskSummary
	for _, slot := range s.taskSlots() {
		slot.mu.Lock()
		st := slot.state
		if st.Tool != "" {
			result = append(result, toolTaskSummary{Tool: st.Tool, Status: st.Status, PlanID: st.PlanID, TaskID: st.TaskID, HistoryID: st.HistoryID, Error: st.Error, Total: st.Total, Done: st.Done, Actionable: st.Actionable, Conflicts: st.Conflicts, Result: st.Result, Operation: st.Operation})
		}
		slot.mu.Unlock()
	}
	return result
}
func (s *server) historyMutex() *sync.Mutex { return &s.taskRoot().historyMu }
func publicTaskConfig(c toolbox.Config) *toolbox.Config {
	c = toolbox.ScopeConfig(c)
	c.ArchivePassword = ""
	c.ExtractPassword = ""
	return &c
}

type taskPaths struct{ read, write []string }

func taskPath(path string) string {
	if path == "" {
		return ""
	}
	p, e := filepath.Abs(path)
	if e != nil {
		return ""
	}
	if actual, err := toolbox.CanonicalResourcePath(path); err == nil {
		p = actual
	}
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	return filepath.Clean(p)
}
func taskOverlap(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	rel, e := filepath.Rel(a, b)
	if e == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return true
	}
	rel, e = filepath.Rel(b, a)
	return e == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
func taskResources(c toolbox.Config) taskPaths {
	p := taskPaths{read: []string{taskPath(c.Root)}}
	switch c.Tool {
	case "format", "rename", "number", "describe":
		p.write = []string{taskPath(c.Root)}
	default:
		output := c.Output
		if output == "" {
			output = filepath.Join(c.Root, "toolbox-output", c.Tool)
			switch c.Tool {
			case "thumbnail":
				output = filepath.Join(c.Root, "视频预览图")
			case "archive":
				output = filepath.Join(c.Root, "0utput_7z")
			case "extract":
				output = filepath.Join(c.Root, "解压资源")
			case "pdf":
				output = filepath.Join(c.Root, "toolbox-output", "目录索引.pdf")
			}
		}
		p.write = []string{taskPath(output)}
		if c.Tool == "extract" && c.ExtractDeleteSource {
			p.write = append(p.write, taskPath(c.Root))
		}
		if c.Tool == "archive" {
			for _, path := range strings.Split(c.ArchiveExtras, "\n") {
				if strings.TrimSpace(path) != "" {
					p.read = append(p.read, taskPath(strings.TrimSpace(path)))
				}
			}
		}
	}
	return p
}

func taskMediaKinds(c toolbox.Config, writing bool) int {
	switch c.Tool {
	case "video":
		return 2
	case "image":
		return 1
	case "thumbnail":
		if writing {
			return 1
		}
		return 2
	case "exif":
		switch c.MetadataTypes {
		case "images":
			return 1
		case "videos":
			return 2
		}
		return 3
	case "describe":
		if !writing {
			return 3
		}
	case "pdf", "archive":
		if writing {
			return 4
		}
	}
	return 7
}
func taskReadConflict(reader, writer toolbox.Config, root, write string) bool {
	if !taskOverlap(root, write) {
		return false
	}
	if taskMediaKinds(reader, false)&taskMediaKinds(writer, true) == 0 {
		return false
	}
	switch reader.Tool {
	case "image", "video", "exif", "describe", "thumbnail":
		rel, err := filepath.Rel(root, write)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			first := strings.Split(rel, string(filepath.Separator))[0]
			if first == "toolbox-output" || first == "0utput_7z" || first == "compress_output" || first == "exif_clear_output" {
				return false
			}
		}
	}
	return true
}

// Called with the dispatch mutex and this slot's mutex held. Running readers
// may coexist; overlapping writes, destructive renames and outputs cannot.
func (s *server) taskConflict(c toolbox.Config) error {
	incoming := taskResources(c)
	for _, other := range s.taskSlots() {
		if other == s {
			continue
		}
		other.mu.Lock()
		active := other.busy()
		reserved := other.reserved
		name := other.state.Tool
		other.mu.Unlock()
		if !active {
			continue
		}
		if name == c.Tool {
			return fmt.Errorf("此工具已有任务正在运行")
		}
		if reserved == nil {
			continue
		}
		paths := taskResources(*reserved)
		for _, a := range incoming.write {
			for _, b := range paths.write {
				if taskOverlap(a, b) {
					return fmt.Errorf("与正在运行的 %s 任务路径冲突，请更换输入/输出目录或等待该任务完成", name)
				}
			}
			for _, b := range paths.read {
				if taskReadConflict(*reserved, c, b, a) {
					return fmt.Errorf("输出目录与正在运行的 %s 任务输入冲突，请更换目录或等待完成", name)
				}
			}
		}
		for _, a := range incoming.read {
			for _, b := range paths.write {
				if taskReadConflict(c, *reserved, a, b) {
					return fmt.Errorf("正在运行的 %s 任务会修改此资源目录，请更换目录或等待完成", name)
				}
			}
		}
	}
	return nil
}
func (s *server) allTasksIdle() bool {
	for _, slot := range s.taskSlots() {
		slot.mu.Lock()
		busy := slot.busy()
		slot.mu.Unlock()
		if busy {
			return false
		}
	}
	return true
}

func (s *server) recordTaskCompletion(st state) {
	r := s.taskRoot()
	event := toolTaskSummary{Tool: st.Tool, Status: st.Status, PlanID: st.PlanID, TaskID: st.TaskID, HistoryID: st.HistoryID, Error: st.Error, Total: st.Total, Done: st.Done, Actionable: st.Actionable, Conflicts: st.Conflicts, Result: st.Result}
	if len(event.Error) > 512 {
		event.Error = event.Error[:512]
	}
	r.tasksMu.Lock()
	defer r.tasksMu.Unlock()
	r.completedTasks = append(r.completedTasks, event)
	if len(r.completedTasks) > 128 {
		r.completedTasks = append([]toolTaskSummary(nil), r.completedTasks[len(r.completedTasks)-128:]...)
	}
}
func (s *server) taskCompletions() []toolTaskSummary {
	r := s.taskRoot()
	r.tasksMu.Lock()
	defer r.tasksMu.Unlock()
	return append([]toolTaskSummary(nil), r.completedTasks...)
}
