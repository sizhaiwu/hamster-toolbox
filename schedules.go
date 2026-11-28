package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

type scheduledJob struct {
	ReadPaths, WritePaths []string

	Period                                string
	Every, AtHour, AtMinute, Day, Weekday int

	ID, Name, Kind, Shell, Script, Directory, URL, Method, Body, Cron string
	Error                                                             string
	Queued                                                            bool `json:",omitempty"`
	ExpectedStatus, RetryCount, RetryDelay                            int
	ResponseContains                                                  string
	RetryPOST                                                         bool
	Headers                                                           map[string]string
	Enabled                                                           bool
	Interval, Timeout                                                 int // seconds
	Next                                                              time.Time
}
type scheduleRun struct {
	ID, JobID, Name, Status, Output string
	Started                         time.Time
	Finished                        time.Time
}
type scheduleStore struct {
	ExamplesInitialized bool

	Concurrency, Retention int
	Version                int
	Jobs                   []scheduledJob
	Runs                   []scheduleRun
}
type scheduleManager struct {
	lockFile *os.File

	mu         sync.Mutex
	dir        string
	store      scheduleStore
	active     map[string]context.CancelFunc
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	err        string
	closed     bool
	queue      []string
	live       map[string]*scheduleOutput
	lastConfig []byte
	garbage    []string
}

func newScheduleManager(dir string) *scheduleManager {
	if absolute, e := filepath.Abs(dir); e == nil {
		dir = absolute
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &scheduleManager{dir: dir, ctx: ctx, cancel: cancel, active: map[string]context.CancelFunc{}, store: scheduleStore{Version: 1, Jobs: []scheduledJob{}, Runs: []scheduleRun{}}}
	m.live = map[string]*scheduleOutput{}
	m.loadSchedules()
	return m
}

func validateScheduledJob(j *scheduledJob) error {
	for _, paths := range [][]string{j.ReadPaths, j.WritePaths} {
		if len(paths) > 16 {
			return errors.New("互斥路径最多 16 个")
		}
		for _, p := range paths {
			if len(p) > 4096 || !filepath.IsAbs(p) {
				return errors.New("互斥路径需为绝对路径")
			}
		}
	}

	if j.RetryCount < 0 || j.RetryCount > 3 || j.ExpectedStatus < 0 || j.ExpectedStatus > 599 || j.ExpectedStatus > 0 && j.ExpectedStatus < 100 || len(j.ResponseContains) > 4096 {
		return errors.New("HTTP 检查或重试参数无效")
	}
	if j.RetryCount > 0 {
		if j.Kind != "http" {
			return errors.New("脚本不自动重试")
		}
		if j.Method == "POST" && !j.RetryPOST {
			return errors.New("POST 重试需要明确允许，可能重复操作")
		}
		if j.RetryDelay == 0 {
			j.RetryDelay = 5
		}
		if j.RetryDelay < 1 || j.RetryDelay > 300 {
			return errors.New("重试间隔需为 1～300 秒")
		}
	}

	if e := validateScheduleTiming(j); e != nil {
		return e
	}

	j.Name = strings.TrimSpace(j.Name)
	if j.Name == "" || len(j.Name) > 200 {
		return errors.New("请填写任务名称（最多 200 字节）")
	}
	if j.Timeout == 0 {
		j.Timeout = 600
	}
	if j.Timeout < 1 || j.Timeout > 86400 {
		return errors.New("超时需为 1～86400 秒")
	}

	switch j.Kind {
	case "script":
		if strings.TrimSpace(j.Script) == "" || len(j.Script) > 65536 {
			return errors.New("请填写脚本（最多 64 KB）")
		}
		switch j.Shell {
		case "powershell", "cmd":
			if runtime.GOOS != "windows" {
				return errors.New("当前系统请使用 sh 或 bash")
			}
		case "sh", "bash":
		default:
			return errors.New("请选择脚本解释器")
		}
		if j.Directory != "" {
			if !filepath.IsAbs(j.Directory) {
				return errors.New("工作目录需为绝对路径")
			}
			st, e := os.Stat(j.Directory)
			if e != nil || !st.IsDir() {
				return errors.New("工作目录不存在")
			}
		}
	case "http":
		u, e := url.Parse(j.URL)
		if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || len(j.URL) > 8192 {
			return errors.New("请填写 http/https 地址，认证请放入请求头")
		}
		if j.Method != "GET" && j.Method != "POST" {
			return errors.New("请求方式仅支持 GET / POST")
		}
		if len(j.Body) > 65536 || len(j.Headers) > 32 {
			return errors.New("请求体或请求头过大")
		}
		for k, v := range j.Headers {
			if k == "" || strings.ContainsAny(k+v, "\r\n") || len(k)+len(v) > 8192 {
				return errors.New("请求头无效")
			}
		}
	default:
		return errors.New("任务类型无效")
	}
	return nil
}

// Five-field cron: lists, ranges and steps. Day-of-month/day-of-week use standard OR semantics.
type scheduleCron [5]map[int]bool

func parseScheduleCron(value string) (scheduleCron, error) {
	var result scheduleCron
	fields := strings.Fields(value)
	if len(fields) != 5 {
		return result, errors.New("Cron 需要 5 项：分 时 日 月 周")
	}
	mins := []int{0, 0, 1, 1, 0}
	maxs := []int{59, 23, 31, 12, 7}
	for i, f := range fields {
		result[i] = map[int]bool{}
		for _, part := range strings.Split(f, ",") {
			split := strings.Split(part, "/")
			if len(split) > 2 {
				return result, errors.New("Cron 步长无效")
			}
			step := 1
			var e error
			if len(split) == 2 {
				step, e = strconv.Atoi(split[1])
				if e != nil || step < 1 || step > maxs[i]+1 {
					return result, errors.New("Cron 步长无效")
				}
			}
			a, b := mins[i], maxs[i]
			if split[0] != "*" {
				bounds := strings.Split(split[0], "-")
				if len(bounds) > 2 {
					return result, errors.New("Cron 范围无效")
				}
				a, e = strconv.Atoi(bounds[0])
				if e != nil {
					return result, errors.New("Cron 数字无效")
				}
				b = a
				if len(bounds) == 2 {
					b, e = strconv.Atoi(bounds[1])
				} else if len(split) == 2 {
					b = maxs[i]
				}
				if e != nil || a < mins[i] || b > maxs[i] || a > b {
					return result, errors.New("Cron 超出有效范围")
				}
			}
			for n := a; n <= b; n += step {
				v := n
				if i == 4 && v == 7 {
					v = 0
				}
				result[i][v] = true
			}
		}
	}
	return result, nil
}
func nextScheduledTime(j scheduledJob, after time.Time) (time.Time, error) {
	if j.Interval > 0 {
		return after.Add(time.Duration(j.Interval) * time.Second), nil
	}
	c, e := parseScheduleCron(j.Cron)
	if e != nil {
		return time.Time{}, e
	}
	fields := strings.Fields(j.Cron)
	t := after.Truncate(time.Minute).Add(time.Minute)
	// Includes leap-day schedules; bounded work and no new scheduler dependencies.
	for n := 0; n < 5*366*24*60; n++ {
		day := c[2][t.Day()]
		week := c[4][int(t.Weekday())]
		match := day && week
		if fields[2] != "*" && fields[4] != "*" {
			match = day || week
		}
		if c[0][t.Minute()] && c[1][t.Hour()] && c[3][int(t.Month())] && match {
			return t, nil
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}, errors.New("未来五年内没有符合 Cron 的时间，请检查日期")
}
func (m *scheduleManager) start() {
	m.mu.Lock()
	if e := os.MkdirAll(m.dir, 0700); e != nil {
		m.err = e.Error()
		m.mu.Unlock()
		return
	}
	f, e := os.OpenFile(filepath.Join(m.dir, "schedules.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e == nil {
		e = lockTaskRecord(f)
	}
	if e != nil {
		if f != nil {
			f.Close()
		}
		m.err = "另一个工具箱正在管理此数据目录的计划任务，或无法取得锁；本实例不执行计划任务"
		m.mu.Unlock()
		return
	}
	m.lockFile = f
	m.loadSchedules()
	if m.err != "" {
		m.mu.Unlock()
		return
	}
	if !m.store.ExamplesInitialized {
		before := append([]scheduledJob(nil), m.store.Jobs...)
		for _, j := range initialScheduledJobs() {
			exists := false
			for _, old := range m.store.Jobs {
				if old.ID == j.ID {
					exists = true
					break
				}
			}
			if !exists && len(m.store.Jobs) < 100 {
				j.Next, _ = nextScheduledTime(j, time.Now())
				m.store.Jobs = append(m.store.Jobs, j)
			}
		}
		m.store.ExamplesInitialized = true
		if e := m.save(); e != nil {
			m.store.Jobs = before
			m.store.ExamplesInitialized = false
			m.err = "初始化示例任务失败：" + e.Error()
			m.mu.Unlock()
			return
		}
	}
	if e := m.save(); e != nil {
		m.err = "保存恢复状态失败：" + e.Error()
		m.mu.Unlock()
		return
	}
	m.sweepLogs()
	m.mu.Unlock()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-m.ctx.Done():
				return
			case now := <-ticker.C:
				m.tick(now)
			}
		}
	}()
}
func (m *scheduleManager) stop() {
	m.mu.Lock()
	m.closed = true
	m.queue = nil
	m.cancel()
	for _, cancel := range m.active {
		cancel()
	}
	m.mu.Unlock()
	m.wg.Wait()
	m.mu.Lock()
	if m.lockFile != nil {
		unlockTaskRecord(m.lockFile)
		m.lockFile.Close()
		m.lockFile = nil
	}
	m.mu.Unlock()
}
func (m *scheduleManager) tick(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != "" || m.closed {
		return
	}
	m.flushLiveLogs()
	if m.err != "" {
		return
	}
	for len(m.queue) > 0 && len(m.active) < m.parallelism() {
		id := m.queue[0]
		m.queue = m.queue[1:]
		if e := m.launch(id, now); e != nil && m.err != "" {
			return
		}
	}
	jobs := append([]scheduledJob(nil), m.store.Jobs...)
	sort.SliceStable(jobs, func(i, j int) bool { return jobs[i].Next.Before(jobs[j].Next) })
	for _, j := range jobs {
		if j.Enabled && !j.Next.IsZero() && !j.Next.After(now) && m.active[j.ID] == nil && !m.isQueued(j.ID) {
			_ = m.submit(j.ID, now)
			if m.err != "" {
				return
			}
		}
	}

}

// Called with the mutex held; durable claim before side effects, never overlap a job.
func (m *scheduleManager) launch(id string, now time.Time) error {
	if m.closed {
		return errors.New("服务正在退出")
	}
	if m.active[id] != nil {
		return errors.New("此任务正在运行，不重复执行")
	}
	if len(m.active) >= m.parallelism() {
		return errors.New("执行位已满")
	}
	idx := -1
	for i, j := range m.store.Jobs {
		if j.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return errors.New("任务不存在")
	}
	j := m.store.Jobs[idx]
	if e := validateScheduledJob(&j); e != nil {
		m.pauseInvalid(idx, e)
		return e
	}
	next, e := nextScheduledTime(j, now)
	if e != nil {
		return e
	}
	old := m.store.Jobs[idx].Next
	oldRuns := append([]scheduleRun(nil), m.store.Runs...)
	m.store.Jobs[idx].Error = ""
	m.store.Jobs[idx].Next = next
	run := scheduleRun{ID: randomID(), JobID: id, Name: j.Name, Status: "running", Started: now}
	m.store.Runs = append([]scheduleRun{run}, m.store.Runs...)
	m.trimRuns()
	if e = m.save(); e != nil {
		m.store.Jobs[idx].Next = old
		m.store.Runs = oldRuns
		m.err = "保存执行记录失败：" + e.Error()
		return fmt.Errorf("保存执行记录失败，未执行：%w", e)
	}
	ctx, cancel := context.WithTimeout(m.ctx, time.Duration(j.Timeout)*time.Second)
	m.active[id] = cancel
	outputBuffer := &scheduleOutput{}
	m.live[run.ID] = outputBuffer
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		output, err := executeScheduledJob(context.WithValue(ctx, scheduleLogKey{}, outputBuffer), j, m.dir)
		status := "success"
		if err != nil {
			status = "failed"
			if ctx.Err() != nil {
				status = "cancelled"
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					status = "timeout"
				}
			}
			output += "\n" + err.Error()
		}
		cancel()
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.active, id)
		delete(m.live, run.ID)
		for i := range m.store.Jobs {
			if m.store.Jobs[i].ID == id {
				m.store.Jobs[i].Next, _ = nextScheduledTime(m.store.Jobs[i], time.Now())
			}
		}

		for i := range m.store.Runs {
			if m.store.Runs[i].ID == run.ID {
				m.store.Runs[i].Status = status
				m.store.Runs[i].Finished = time.Now()
				m.store.Runs[i].Output = output
				break
			}
		}
		if e := m.writeRunLog(run.ID, output); e != nil {
			m.err = "保存执行日志失败：" + e.Error()
		}
		if e := m.save(); e != nil {
			m.err = "保存计划任务结果失败：" + e.Error()
		}
	}()
	return nil
}

func executeScheduledJob(ctx context.Context, j scheduledJob, dir string) (string, error) {
	if j.Kind == "http" {
		return executeScheduleHTTP(ctx, j)
	}

	parentToken := ""
	if len(j.ReadPaths)+len(j.WritePaths) > 0 {
		release, token, e := toolbox.AcquireResourcePathsWithToken(ctx, j.ReadPaths, j.WritePaths, func(message string) { fmt.Fprintln(scheduleLog(ctx), message) })
		if e != nil {
			return scheduleLog(ctx).snapshot(), e
		}
		defer release()
		parentToken = token
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	ext := ".sh"
	if j.Shell == "cmd" {
		ext = ".cmd"
	}
	if j.Shell == "powershell" {
		ext = ".ps1"
	}
	f, e := os.CreateTemp(dir, ".scheduled-*"+ext)
	if e != nil {
		return "", e
	}
	name := f.Name()
	defer os.Remove(name)
	text := j.Script
	if j.Shell == "powershell" {
		text = "\ufeff$ErrorActionPreference = 'Stop'\r\n[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)\r\n$OutputEncoding = [Console]::OutputEncoding\r\n" + text
	}
	if j.Shell == "cmd" {
		text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", "\r\n")
	}
	e = f.Chmod(0600)
	if e == nil {
		_, e = f.WriteString(text)
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return "", e
	}
	var cmd *exec.Cmd
	switch j.Shell {
	case "powershell":
		cmd = exec.CommandContext(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", name)
	case "cmd":
		cmd = exec.CommandContext(ctx, "cmd.exe", "/D", "/C", name)
	default:
		cmd = exec.CommandContext(ctx, j.Shell, name)
	}
	if parentToken != "" {
		cmd.Env = append(os.Environ(), "HAMSTER_RESOURCE_PARENT="+parentToken)
	}
	cmd.Dir = j.Directory
	if cmd.Dir == "" {
		cmd.Dir = dir
	}
	output := scheduleLog(ctx)
	cmd.Stdout = output
	cmd.Stderr = output
	cmd.WaitDelay = 2 * time.Second
	configureScheduledProcess(cmd)
	e = cmd.Run()
	return output.snapshot(), e
}
func (s *server) scheduleAPI(w http.ResponseWriter, r *http.Request) {
	m := s.taskRoot().scheduler
	if m == nil {
		fail(w, errors.New("计划任务尚未就绪"))
		return
	}
	var q struct {
		Action, ID                  string
		Job                         scheduledJob
		IDs                         []string
		Operation, Format, Conflict string
		Payload                     json.RawMessage
		TemplateID                  string
		Params                      map[string]string
		Options                     struct{ Concurrency, Retention int }
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024*1024)
	if e := json.NewDecoder(r.Body).Decode(&q); e != nil {
		fail(w, errors.New("计划任务请求无效或过大"))
		return
	}
	if q.Action == "pick-path" {
		result, e := schedulePathList(q.Params["path"], q.Params["mode"], q.Params["search"])
		if e != nil {
			fail(w, e)
			return
		}
		send(w, result)
		return
	}
	if q.Action == "library" || q.Action == "template-build" {
		s.scheduleLibraryAPI(w, q.Action, q.TemplateID, q.Params)
		return
	}
	if q.Action == "save" || q.Action == "preview" {
		b, _ := json.Marshal(q.Job)
		if len(b) > 160*1024 {
			fail(w, errors.New("单个任务超过 160 KB"))
			return
		}
	}
	if q.Action == "preview" {
		result, e := previewScheduleTimes(q.Job, time.Now())
		if e != nil {
			fail(w, e)
			return
		}
		send(w, result)
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if q.Action == "options" {
		if m.err != "" {
			fail(w, errors.New(m.err))
			return
		}
		if q.Options.Concurrency < 1 || q.Options.Concurrency > 8 || q.Options.Retention < 5 || q.Options.Retention > 100 {
			fail(w, errors.New("并发需为 1～8，日志保留需为 5～100"))
			return
		}
		beforeA, beforeB := m.store.Concurrency, m.store.Retention
		beforeRuns := append([]scheduleRun(nil), m.store.Runs...)
		beforeGarbage := append([]string(nil), m.garbage...)
		m.store.Concurrency = q.Options.Concurrency
		m.store.Retention = q.Options.Retention
		m.trimRuns()
		if e := m.save(); e != nil {
			m.store.Concurrency = beforeA
			m.store.Retention = beforeB
			m.store.Runs = beforeRuns
			m.garbage = beforeGarbage
			fail(w, e)
			return
		}
		send(w, map[string]bool{"OK": true})
		return
	}
	if q.Action == "logs" {
		result, e := m.scheduleLogs(q.ID, q.Params, time.Now())
		if e != nil {
			fail(w, e)
			return
		}
		send(w, result)
		return
	}
	if q.Action == "detail" {
		for _, j := range m.store.Jobs {
			if j.ID == q.ID {
				send(w, j)
				return
			}
		}
		for _, run := range m.store.Runs {
			if run.ID == q.ID {
				run.Output = m.readRunLog(run)
				send(w, run)
				return
			}
		}
		fail(w, errors.New("任务或记录不存在"))
		return
	}
	if q.Action == "" || q.Action == "list" {
		jobs := append([]scheduledJob{}, m.store.Jobs...)
		runs := append([]scheduleRun{}, m.store.Runs...)
		for i := range jobs {
			jobs[i].Queued = m.isQueued(jobs[i].ID)
			jobs[i].Script = ""
			jobs[i].Headers = nil
			jobs[i].Body = ""
			jobs[i].URL = ""
			jobs[i].Directory = ""
		}
		for i := range runs {
			runs[i].Output = ""
		}
		send(w, map[string]any{"Jobs": jobs, "Runs": runs, "Error": m.err, "Platform": runtime.GOOS, "Now": time.Now(), "Active": len(m.active), "Queued": len(m.queue), "Concurrency": m.parallelism(), "Retention": m.retention()})
		return
	}

	if q.Action == "cancel" {
		m.unqueue(q.ID)
		if c := m.active[q.ID]; c != nil {
			c()
		}
		send(w, map[string]bool{"OK": true})
		return
	}

	if m.err != "" {
		fail(w, errors.New(m.err))
		return
	}
	if q.Action == "clear-logs" {
		result, e := m.clearScheduleLogs(q.ID)
		if e != nil {
			fail(w, e)
			return
		}
		send(w, result)
		return
	}
	if q.Action == "batch" {
		m.batchAPI(w, q.Operation, q.IDs)
		return
	}
	if q.Action == "export" || q.Action == "import-preview" || q.Action == "import" {
		m.transferAPI(w, q.Action, q.Format, q.Conflict, q.IDs, q.Payload)
		return
	}
	idx := -1
	for i, j := range m.store.Jobs {
		if j.ID == q.ID {
			idx = i
			break
		}
	}
	if q.Action == "run" {
		if e := m.submit(q.ID, time.Now()); e != nil {
			fail(w, e)
			return
		}
		send(w, map[string]bool{"OK": true})
		return
	}
	before := append([]scheduledJob(nil), m.store.Jobs...)
	beforeQueue := append([]string(nil), m.queue...)
	switch q.Action {
	case "save":
		if m.active[q.ID] != nil || m.isQueued(q.ID) {
			fail(w, errors.New("请先停止任务或取消等待再编辑"))
			return
		}
		j := q.Job
		j.Error = ""
		j.Queued = false
		if e := validateScheduledJob(&j); e != nil {
			fail(w, e)
			return
		}
		next, e := nextScheduledTime(j, time.Now())
		if e != nil {
			fail(w, e)
			return
		}
		j.Next = next
		if q.ID == "" {
			if len(m.store.Jobs) >= 100 {
				fail(w, errors.New("最多保存 100 个计划任务"))
				return
			}
			j.ID = randomID()
			m.store.Jobs = append(m.store.Jobs, j)
		} else {
			if idx < 0 {
				fail(w, errors.New("任务不存在"))
				return
			}
			j.ID = q.ID
			m.store.Jobs[idx] = j
		}
	case "delete", "toggle":
		if idx < 0 {
			fail(w, errors.New("任务不存在"))
			return
		}
		if m.active[q.ID] != nil {
			fail(w, errors.New("请先停止正在运行的任务"))
			return
		}
		if q.Action == "delete" {
			m.unqueue(q.ID)
			m.store.Jobs = append(m.store.Jobs[:idx], m.store.Jobs[idx+1:]...)
		} else {
			if !m.store.Jobs[idx].Enabled {
				check := m.store.Jobs[idx]
				if e := validateScheduledJob(&check); e != nil {
					fail(w, e)
					return
				}
			}
			m.store.Jobs[idx].Enabled = !m.store.Jobs[idx].Enabled
			m.store.Jobs[idx].Error = ""
			if !m.store.Jobs[idx].Enabled {
				m.unqueue(q.ID)
			}
			m.store.Jobs[idx].Next, _ = nextScheduledTime(m.store.Jobs[idx], time.Now())
		}
	default:
		fail(w, errors.New("不支持的计划任务操作"))
		return
	}
	if e := m.save(); e != nil {
		m.store.Jobs = before
		m.queue = beforeQueue
		fail(w, e)
		return
	}
	send(w, map[string]bool{"OK": true})
}

// Calendar periods follow the familiar panel input fields. Legacy interval jobs
// retain elapsed-time semantics rather than silently changing their schedule.
func normalizeScheduledPeriod(j *scheduledJob) error {
	if j.Period == "" {
		return nil
	}
	if j.AtHour < 0 || j.AtHour > 23 || j.AtMinute < 0 || j.AtMinute > 59 {
		return errors.New("小时需为 0～23，分钟需为 0～59")
	}
	j.Interval = 0
	switch j.Period {
	case "day":
		j.Cron = fmt.Sprintf("%d %d * * *", j.AtMinute, j.AtHour)
	case "day-n":
		if j.Every < 1 || j.Every > 31 {
			return errors.New("N 天需为 1～31")
		}
		j.Cron = fmt.Sprintf("%d %d */%d * *", j.AtMinute, j.AtHour, j.Every)
	case "hour":
		j.Cron = fmt.Sprintf("%d * * * *", j.AtMinute)
	case "hour-n":
		if j.Every < 1 || j.Every > 24 {
			return errors.New("N 小时需为 1～24")
		}
		j.Cron = fmt.Sprintf("%d */%d * * *", j.AtMinute, j.Every)
	case "minute-n":
		if j.Every < 1 || j.Every > 60 {
			return errors.New("N 分钟需为 1～60")
		}
		j.Cron = fmt.Sprintf("*/%d * * * *", j.Every)
	case "week":
		if j.Weekday < 0 || j.Weekday > 6 {
			return errors.New("请选择星期")
		}
		j.Cron = fmt.Sprintf("%d %d * * %d", j.AtMinute, j.AtHour, j.Weekday)
	case "month":
		if j.Day < 1 || j.Day > 31 {
			return errors.New("每月日期需为 1～31")
		}
		j.Cron = fmt.Sprintf("%d %d %d * *", j.AtMinute, j.AtHour, j.Day)
	case "second-n":
		if j.Every < 1 || j.Every > 59 {
			return errors.New("N 秒需为 1～59")
		}
		j.Interval = j.Every
		j.Cron = ""
	case "cron": // Parsed by the common cron validator below.
	default:
		return errors.New("执行周期无效")
	}
	return nil
}
func initialScheduledJobs() []scheduledJob {
	shell, system, disk := "sh", "# 示例：只输出时间、系统与工作目录，不修改文件\ndate\nuname -s\npwd", "# 示例：只查看磁盘空间，不清理文件\ndf -h"
	if runtime.GOOS == "windows" {
		shell = "powershell"
		system = "# 示例：只输出时间、系统与工作目录，不修改文件\nGet-Date\n[System.Environment]::OSVersion.VersionString\nGet-Location"
		disk = "# 示例：只查看磁盘空间，不清理文件\nGet-PSDrive -PSProvider FileSystem | Select-Object Name, Used, Free"
	}
	jobs := []scheduledJob{
		{ID: "example-system", Name: "示例 · 输出系统信息", Kind: "script", Shell: shell, Script: system, Period: "day", AtHour: 9, Timeout: 30},
		{ID: "example-disk", Name: "示例 · 查看磁盘空间", Kind: "script", Shell: shell, Script: disk, Period: "week", Weekday: 1, AtHour: 9, Timeout: 30},
		{ID: "example-get", Name: "示例 · GET 检查链接", Kind: "http", Method: "GET", URL: "https://example.com/", Period: "hour", AtMinute: 0, Timeout: 30},
		{ID: "example-post", Name: "示例 · POST JSON（先修改地址）", Kind: "http", Method: "POST", URL: "https://example.invalid/api", Headers: map[string]string{"Content-Type": "application/json"}, Body: `{"source":"hamster-toolbox","action":"check"}`, Period: "minute-n", Every: 30, Timeout: 30},
	}
	for i := range jobs {
		_ = validateScheduledJob(&jobs[i])
	}
	return jobs
}

// Preview and execution share the same timing validation and next-time calculation.
func validateScheduleTiming(j *scheduledJob) error {
	if e := normalizeScheduledPeriod(j); e != nil {
		return e
	}
	if len(j.Cron) > 256 {
		return errors.New("Cron 表达式过长")
	}
	if j.Interval != 0 {
		if (j.Interval < 60 && j.Period != "second-n") || j.Interval < 1 || j.Interval > 31536000 {
			return errors.New("间隔需为 60～31536000 秒")
		}
	} else {
		if _, e := parseScheduleCron(j.Cron); e != nil {
			return e
		}
	}
	return nil
}
func previewScheduleTimes(j scheduledJob, now time.Time) (map[string]any, error) {
	if e := validateScheduleTiming(&j); e != nil {
		return nil, e
	}
	times := make([]string, 0, 5)
	cursor := now
	weekdays := []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}
	for i := 0; i < 5; i++ {
		next, e := nextScheduledTime(j, cursor)
		if e != nil {
			return nil, e
		}
		times = append(times, next.Format("2006-01-02")+"（"+weekdays[next.Weekday()]+"）"+next.Format("15:04:05"))
		cursor = next
	}
	return map[string]any{"Times": times, "Timezone": now.Format("MST UTC-07:00"), "Now": now.Format("2006-01-02 15:04:05")}, nil
}
