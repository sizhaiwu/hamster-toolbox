package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const scheduleLogLimit = 64 * 1024

func (m *scheduleManager) parallelism() int {
	if m.store.Concurrency >= 1 && m.store.Concurrency <= 8 {
		return m.store.Concurrency
	}
	return 4
}
func (m *scheduleManager) retention() int {
	if m.store.Retention >= 5 && m.store.Retention <= 100 {
		return m.store.Retention
	}
	return 20
}
func (m *scheduleManager) isQueued(id string) bool {
	for _, v := range m.queue {
		if v == id {
			return true
		}
	}
	return false
}
func (m *scheduleManager) unqueue(id string) {
	for i, v := range m.queue {
		if v == id {
			m.queue = append(m.queue[:i], m.queue[i+1:]...)
			for k := range m.store.Jobs {
				if m.store.Jobs[k].ID == id {
					m.store.Jobs[k].Next, _ = nextScheduledTime(m.store.Jobs[k], time.Now())
				}
			}
			return
		}
	}
}
func (m *scheduleManager) submit(id string, now time.Time) error {
	if m.closed {
		return errors.New("服务正在退出")
	}
	if m.active[id] != nil {
		return errors.New("此任务正在运行")
	}
	if m.isQueued(id) {
		return errors.New("此任务已在等待队列")
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
	if len(m.active) >= m.parallelism() || len(m.queue) > 0 {
		m.queue = append(m.queue, id)
		return nil
	}
	return m.launch(id, now)
}
func (m *scheduleManager) pauseInvalid(i int, e error) {
	m.store.Jobs[i].Enabled = false
	m.store.Jobs[i].Error = e.Error()
	m.store.Jobs[i].Next = time.Time{}
	if err := m.save(); err != nil {
		m.err = "保存异常任务状态失败：" + err.Error()
	}
}
func (m *scheduleManager) loadSchedules() {
	m.err = ""
	m.lastConfig = nil
	m.store = scheduleStore{Version: 1, Jobs: []scheduledJob{}, Runs: []scheduleRun{}}
	b, e := os.ReadFile(filepath.Join(m.dir, "schedules.json"))
	if errors.Is(e, os.ErrNotExist) {
		return
	}
	if e == nil {
		e = json.Unmarshal(b, &m.store)
	}
	if e != nil || m.store.Version != 1 || len(m.store.Jobs) > 100 {
		m.err = "读取计划任务失败，原文件未覆盖"
		return
	}
	if b, e = os.ReadFile(filepath.Join(m.dir, "schedules-history.json")); e == nil {
		var history struct{ Runs []scheduleRun }
		if e = json.Unmarshal(b, &history); e != nil {
			m.err = "执行记录文件损坏，原文件未覆盖"
			return
		}
		m.store.Runs = history.Runs
	} else if !errors.Is(e, os.ErrNotExist) {
		m.err = "读取执行记录失败：" + e.Error()
		return
	}
	if len(m.store.Runs) > 10200 {
		m.err = "执行记录数量无效"
		return
	}
	seen := map[string]bool{}
	for i := range m.store.Jobs {
		j := &m.store.Jobs[i]
		if j.ID == "" || seen[j.ID] {
			m.err = "计划任务 ID 无效，原文件未覆盖"
			return
		}
		seen[j.ID] = true
		j.Queued = false
		j.Error = ""
		e = validateScheduledJob(j)
		if e == nil {
			j.Next, e = nextScheduledTime(*j, time.Now())
		}
		if e != nil {
			j.Enabled = false
			j.Error = e.Error()
			j.Next = time.Time{}
		}
	}
	for i := range m.store.Runs {
		r := &m.store.Runs[i]
		if r.Status == "running" {
			r.Status = "interrupted"
			r.Finished = time.Now()
			r.Output = m.readRunLog(*r) + "\n上次运行中服务退出，未自动重跑"
		}
	}
}
func scheduleAtomic(path string, data []byte) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".schedule-save-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	e = f.Chmod(0600)
	if e == nil {
		_, e = f.Write(data)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(f.Name(), path)
}
func safeScheduleLogID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
func (m *scheduleManager) logPath(id string) string {
	return filepath.Join(m.dir, "schedule-logs", id+".log")
}
func (m *scheduleManager) writeRunLog(id, output string) error {
	if !safeScheduleLogID(id) {
		return errors.New("日志 ID 无效")
	}
	if len(output) > scheduleLogLimit+128 {
		output = output[len(output)-scheduleLogLimit:]
	}
	return scheduleAtomic(m.logPath(id), []byte(output))
}
func (m *scheduleManager) readRunLog(r scheduleRun) string {
	if b := m.live[r.ID]; b != nil {
		return b.snapshot()
	}
	if r.Output != "" {
		return r.Output
	}
	if !safeScheduleLogID(r.ID) {
		return "日志 ID 无效"
	}
	b, e := os.ReadFile(m.logPath(r.ID))
	if e != nil {
		return ""
	}
	return strings.ToValidUTF8(string(b), "�")
}
func (m *scheduleManager) flushLiveLogs() {
	for id, b := range m.live {
		snapshot := b.snapshot()
		b.mu.Lock()
		same := snapshot == b.flushed
		if !same {
			b.flushed = snapshot
		}
		b.mu.Unlock()
		if !same {
			if e := m.writeRunLog(id, snapshot); e != nil {
				m.err = "保存实时日志失败：" + e.Error()
				return
			}
		}
	}
}
func (m *scheduleManager) save() error {
	history := append([]scheduleRun{}, m.store.Runs...)
	for i := range history {
		if history[i].Output != "" {
			if e := m.writeRunLog(history[i].ID, history[i].Output); e != nil {
				return e
			}
			m.store.Runs[i].Output = ""
		}
		history[i].Output = ""
	}
	b, e := json.Marshal(struct{ Runs []scheduleRun }{history})
	if e != nil {
		return e
	}
	if e = scheduleAtomic(filepath.Join(m.dir, "schedules-history.json"), b); e != nil {
		return e
	}
	config := m.store
	config.Runs = nil
	config.Jobs = append([]scheduledJob{}, m.store.Jobs...)
	for i := range config.Jobs {
		config.Jobs[i].Next = time.Time{}
		config.Jobs[i].Queued = false
	}
	data, e := json.MarshalIndent(config, "", "  ")
	if e != nil {
		return e
	}
	if !bytes.Equal(data, m.lastConfig) {
		if e = scheduleAtomic(filepath.Join(m.dir, "schedules.json"), data); e != nil {
			return e
		}
		m.lastConfig = append([]byte(nil), data...)
	}
	m.cleanLogs(m.garbage)
	m.garbage = nil
	return nil
}
func (m *scheduleManager) cleanLogs(ids []string) {
	kept := map[string]bool{}
	for _, r := range m.store.Runs {
		kept[r.ID] = true
	}
	for _, id := range ids {
		if safeScheduleLogID(id) && !kept[id] && m.live[id] == nil {
			_ = os.Remove(m.logPath(id))
		}
	}
}
func (m *scheduleManager) sweepLogs() {
	entries, e := os.ReadDir(filepath.Join(m.dir, "schedule-logs"))
	if e != nil {
		return
	}
	ids := []string{}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".log") {
			ids = append(ids, strings.TrimSuffix(entry.Name(), ".log"))
		}
	}
	m.cleanLogs(ids)
}
func (m *scheduleManager) trimRuns() {
	counts := map[string]int{}
	existing := map[string]bool{}
	for _, j := range m.store.Jobs {
		existing[j.ID] = true
	}
	kept := make([]scheduleRun, 0, len(m.store.Runs))
	orphanCount := 0
	for _, r := range m.store.Runs {
		keep := r.Status == "running"
		if existing[r.JobID] {
			keep = keep || counts[r.JobID] < m.retention()
			counts[r.JobID]++
		} else {
			keep = keep || orphanCount < 100
			orphanCount++
		}
		if keep {
			kept = append(kept, r)
		} else {
			m.garbage = append(m.garbage, r.ID)
		}
	}
	m.store.Runs = kept
}

type scheduleLogKey struct{}
type scheduleOutput struct {
	mu        sync.Mutex
	b         bytes.Buffer
	truncated bool
	flushed   string
}

func (b *scheduleOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	if len(p) >= scheduleLogLimit {
		b.b.Reset()
		b.b.Write(p[len(p)-scheduleLogLimit:])
		b.truncated = true
	} else {
		if extra := b.b.Len() + len(p) - scheduleLogLimit; extra > 0 {
			b.b.Next(extra)
			b.truncated = true
		}
		b.b.Write(p)
	}
	return n, nil
}
func (b *scheduleOutput) snapshot() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	prefix := ""
	if b.truncated {
		prefix = "[输出较长，仅保留最后 64 KB]\n"
	}
	return prefix + strings.ToValidUTF8(b.b.String(), "�")
}
func scheduleLog(ctx context.Context) *scheduleOutput {
	if b, ok := ctx.Value(scheduleLogKey{}).(*scheduleOutput); ok {
		return b
	}
	return &scheduleOutput{}
}
func executeScheduleHTTP(ctx context.Context, j scheduledJob) (string, error) {
	output := scheduleLog(ctx)
	var last error
	for attempt := 0; attempt <= j.RetryCount; attempt++ {
		if attempt > 0 {
			fmt.Fprintf(output, "\n第 %d 次尝试，等待 %d 秒…\n", attempt+1, j.RetryDelay)
			timer := time.NewTimer(time.Duration(j.RetryDelay) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return output.snapshot(), ctx.Err()
			case <-timer.C:
			}
		}
		body := ""
		if j.Method == "POST" {
			body = j.Body
		}
		req, e := http.NewRequestWithContext(ctx, j.Method, j.URL, strings.NewReader(body))
		if e != nil {
			return output.snapshot(), e
		}
		for k, v := range j.Headers {
			req.Header.Set(k, v)
		}
		resp, e := (&http.Client{Timeout: time.Duration(j.Timeout) * time.Second}).Do(req)
		if e != nil {
			last = e
			fmt.Fprintln(output, e)
		} else {
			b, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024*1024+1))
			resp.Body.Close()
			fmt.Fprintln(output, resp.Status)
			output.Write(b)
			last = readErr
			if last == nil && (j.ExpectedStatus > 0 && resp.StatusCode != j.ExpectedStatus || j.ExpectedStatus == 0 && (resp.StatusCode < 200 || resp.StatusCode >= 300)) {
				last = errors.New("HTTP 状态不符合要求")
			}
			if last == nil && j.ResponseContains != "" && !strings.Contains(string(b[:min(len(b), 1024*1024)]), j.ResponseContains) {
				last = errors.New("响应前 1 MB 未包含指定内容")
			}
			if last != nil {
				fmt.Fprintln(output, "\n检查失败：", last)
			}
		}
		if last == nil {
			return output.snapshot(), nil
		}
		if ctx.Err() != nil {
			return output.snapshot(), ctx.Err()
		}
	}
	return output.snapshot(), last
}
