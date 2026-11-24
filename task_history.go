package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

const historyKeep = 50
const historyItemLimit = 100000
const historyReadLimit = 64 << 20

var taskIDPattern = regexp.MustCompile(`^[a-f0-9]{48}$`)

type taskRecord struct {
	Version                    int
	ID, ParentID, Tool, Status string
	Started, Updated, Ended    time.Time
	Total, Done                int
	Result                     toolbox.Result
	Independent, Limited       bool
	Config                     toolbox.Config `json:",omitempty"`
	Logs                       []string       `json:",omitempty"`
	Error                      string         `json:",omitempty"`
}
type historyItem struct {
	Item            toolbox.Item
	Status, Message string
}
type historyEvent struct {
	Type            string
	Items           []historyItem `json:",omitempty"`
	Index           int           `json:",omitempty"`
	Status, Message string        `json:",omitempty"`
}
type taskJournal struct {
	mu            sync.Mutex
	meta          taskRecord
	root          string
	lease, events *os.File
	indices       map[string]int
	secrets       []string
	stop, stopped chan struct{}
	problem       error
	warned        bool
	eventBytes    int64
}

func cleanHistoryConfig(c toolbox.Config) toolbox.Config {
	c = toolbox.PortableConfig(toolbox.ScopeConfig(c))
	c.ArchivePassword, c.ExtractPassword = "", ""
	return c
}
func sameHistoryPath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
func (s *server) historyDirectory(create bool) (string, error) {
	if s.dataDir == "" {
		return "", nil
	}
	root := filepath.Join(s.dataDir, "task-history")
	if create {
		if err := os.MkdirAll(root, 0700); err != nil {
			return "", err
		}
	}
	info, err := os.Lstat(root)
	if os.IsNotExist(err) && !create {
		return root, nil
	}
	if err != nil {
		return "", err
	}
	physical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(root))
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !sameHistoryPath(physical, filepath.Join(parent, filepath.Base(root))) {
		return "", errors.New("任务历史目录不能是目录链接")
	}
	return root, nil
}
func historyFile(root, id, suffix string) string { return filepath.Join(root, id+suffix) }
func writeTaskRecord(root string, record taskRecord) error {
	f, err := os.CreateTemp(root, ".task-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = json.NewEncoder(f).Encode(record); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), historyFile(root, record.ID, ".json"))
}
func readTaskRecord(root, id string) (taskRecord, error) {
	var record taskRecord
	if root == "" || !taskIDPattern.MatchString(id) {
		return record, errors.New("无效任务记录")
	}
	f, err := os.Open(historyFile(root, id, ".json"))
	if err != nil {
		return record, err
	}
	defer f.Close()
	if err = json.NewDecoder(io.LimitReader(f, 2<<20)).Decode(&record); err != nil {
		return record, err
	}
	if record.ID != id || record.Version != 1 || !toolbox.ToolExists(record.Tool) || record.Config.Tool != record.Tool || !filepath.IsAbs(record.Config.Root) {
		return record, errors.New("任务记录格式无效")
	}
	record.Config = cleanHistoryConfig(record.Config)
	if record.Status == "running" && !taskRecordActive(root, id) {
		record.Status = "interrupted"
	}
	return record, nil
}
func taskRecordActive(root, id string) bool {
	f, err := os.OpenFile(historyFile(root, id, ".lock"), os.O_RDWR, 0600)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		return true
	}
	defer f.Close()
	if lockTaskRecord(f) != nil {
		return true
	}
	unlockTaskRecord(f)
	return false
}
func historyText(text string, secrets []string) string {
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[密码已隐藏]")
		}
	}
	if len(text) > 4096 {
		text = string([]rune(text)[:min(len([]rune(text)), 1024)]) + "…"
	}
	return text
}
func historyLogs(lines []string, limit int, secrets []string) []string {
	start := max(0, len(lines)-limit)
	out := []string{}
	budget := 1 << 20
	for i := len(lines) - 1; i >= start; i-- {
		line := historyText(lines[i], secrets)
		if len(line) > budget {
			break
		}
		budget -= len(line)
		out = append(out, line)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}
func (s *server) newTaskJournal(p *toolbox.Plan, planID, parentID string) (*taskJournal, error) {
	root, err := s.historyDirectory(true)
	if err != nil {
		return nil, err
	}
	if root == "" {
		return nil, nil
	}
	j := &taskJournal{root: root, indices: map[string]int{}, stop: make(chan struct{}), stopped: make(chan struct{}), secrets: []string{p.Config.ArchivePassword, p.Config.ExtractPassword}}
	j.meta = taskRecord{Version: 1, ID: randomID(), ParentID: parentID, Tool: p.Config.Tool, Status: "running", Started: time.Now(), Updated: time.Now(), Total: len(p.Items), Config: cleanHistoryConfig(p.Config), Independent: toolbox.IndependentFileTask(p.Config.Tool)}
	j.lease, err = os.OpenFile(historyFile(root, j.meta.ID, ".lock"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = lockTaskRecord(j.lease); err != nil {
		j.lease.Close()
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			if j.events != nil {
				j.events.Close()
			}
			unlockTaskRecord(j.lease)
			j.lease.Close()
			os.Remove(historyFile(root, j.meta.ID, ".lock"))
			os.Remove(historyFile(root, j.meta.ID, ".events"))
		}
	}()
	j.events, err = os.OpenFile(historyFile(root, j.meta.ID, ".events"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	items := []historyItem{}
	budget := 32 << 20
	if j.meta.Independent {
		for _, it := range p.Items {
			cost := 2*(len(it.Source)+len(it.Target)+len(it.Conflict)+len(it.Note)) + 256
			if len(items) >= historyItemLimit || cost > budget {
				j.meta.Limited = true
				break
			}
			budget -= cost
			status := "pending"
			if it.Conflict != "" {
				status = "skipped"
			}
			j.indices[it.Source] = len(items)
			items = append(items, historyItem{Item: it, Status: status, Message: it.Conflict})
		}
	}
	w := bufio.NewWriterSize(j.events, 64<<10)
	if err = json.NewEncoder(w).Encode(historyEvent{Type: "plan", Items: items}); err == nil {
		err = w.Flush()
	}
	if err == nil {
		err = j.events.Sync()
	}
	if err != nil {
		return nil, err
	}
	if info, statErr := j.events.Stat(); statErr == nil {
		j.eventBytes = info.Size()
	}
	if err = writeTaskRecord(root, j.meta); err != nil {
		return nil, err
	}
	ok = true
	go func() {
		defer close(j.stopped)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-j.stop:
				return
			case <-ticker.C:
				s.mu.Lock()
				snapshot := s.state
				snapshot.Logs = append([]string{}, s.state.Logs...)
				s.mu.Unlock()
				if snapshot.PlanID != planID {
					continue
				}
				j.mu.Lock()
				j.meta.Done = snapshot.Done
				j.meta.Updated = time.Now()
				j.meta.Logs = historyLogs(snapshot.Logs, 200, j.secrets)
				if err := j.events.Sync(); err != nil {
					j.problem = err
					j.meta.Limited = true
				}
				if err := writeTaskRecord(j.root, j.meta); err != nil {
					j.problem = err
				}
				problem := j.problem
				warn := problem != nil && !j.warned
				j.warned = j.warned || warn
				j.mu.Unlock()
				if warn {
					s.line("任务历史保存异常，当前任务继续；未完成项恢复可能不可用：" + problem.Error())
				}
			}
		}
	}()
	return j, nil
}
func (j *taskJournal) observe(outcome toolbox.FileOutcome) {
	if j == nil {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	index, ok := j.indices[outcome.Source]
	if !ok {
		return
	}
	data, err := json.Marshal(historyEvent{Type: "result", Index: index, Status: outcome.Status, Message: historyText(outcome.Message, j.secrets)})
	if err == nil && j.eventBytes+int64(len(data)+1) <= historyReadLimit {
		var n int
		n, err = j.events.Write(append(data, '\n'))
		j.eventBytes += int64(n)
	} else if err == nil {
		j.meta.Limited = true
	}
	if err != nil {
		j.problem = err
		j.meta.Limited = true
	}
	switch outcome.Status {
	case "success":
		j.meta.Result.Success++
	case "failed":
		j.meta.Result.Failed++
	case "skipped":
		j.meta.Result.Skipped++
	}
}
func (j *taskJournal) finish(snapshot state) error {
	if j == nil {
		return nil
	}
	close(j.stop)
	<-j.stopped
	j.mu.Lock()
	defer j.mu.Unlock()
	j.meta.Status = snapshot.Status
	j.meta.Done = snapshot.Done
	j.meta.Result = snapshot.Result
	j.meta.Updated = time.Now()
	j.meta.Ended = j.meta.Updated
	j.meta.Error = historyText(snapshot.Error, j.secrets)
	j.meta.Logs = historyLogs(snapshot.Logs, 3000, j.secrets)
	if err := j.events.Sync(); err != nil {
		j.problem = err
		j.meta.Limited = true
	}
	if err := j.events.Close(); err != nil {
		j.problem = err
		j.meta.Limited = true
	}
	err := writeTaskRecord(j.root, j.meta)
	unlockTaskRecord(j.lease)
	j.lease.Close()
	if err != nil {
		return err
	}
	return j.problem
}
func loadHistoryItems(root string, record taskRecord) ([]historyItem, error) {
	f, err := os.Open(historyFile(root, record.ID, ".events"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	decoder := json.NewDecoder(io.LimitReader(f, historyReadLimit))
	var event historyEvent
	if err = decoder.Decode(&event); err != nil {
		return nil, err
	}
	if event.Type != "plan" || len(event.Items) > historyItemLimit {
		return nil, errors.New("任务逐项记录无效")
	}
	items := event.Items
	for {
		event = historyEvent{}
		err = decoder.Decode(&event)
		if err == io.EOF {
			break
		}
		if err == io.ErrUnexpectedEOF && record.Status == "running" {
			break
		}
		if err != nil {
			return nil, err
		}
		if event.Type != "result" || event.Index < 0 || event.Index >= len(items) {
			return nil, errors.New("任务逐项记录损坏，不能定向恢复")
		}
		if event.Status != "success" && event.Status != "failed" && event.Status != "skipped" && event.Status != "pending" {
			return nil, errors.New("未知逐项结果")
		}
		items[event.Index].Status = event.Status
		items[event.Index].Message = event.Message
	}
	return items, nil
}
func (s *server) listTaskHistory() ([]taskRecord, error) {
	root, err := s.historyDirectory(false)
	if err != nil {
		return nil, err
	}
	records := []taskRecord{}
	if root == "" {
		return records, nil
	}
	files, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return records, nil
	}
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(file.Name(), ".json")
		if !taskIDPattern.MatchString(id) {
			continue
		}
		record, err := readTaskRecord(root, id)
		if err != nil {
			continue
		}
		record.Logs = nil
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Started.After(records[j].Started) })
	return records, nil
}
func (s *server) pruneTaskHistory(clear bool) error {
	s.historyMutex().Lock()
	defer s.historyMutex().Unlock()
	records, err := s.listTaskHistory()
	if err != nil {
		return err
	}
	root, err := s.historyDirectory(false)
	if err != nil {
		return err
	}
	totalBytes := int64(0)
	sizes := make(map[string]int64, len(records))
	for _, record := range records {
		for _, suffix := range []string{".events", ".json"} {
			if st, e := os.Stat(historyFile(root, record.ID, suffix)); e == nil {
				sizes[record.ID] += st.Size()
				totalBytes += st.Size()
			}
		}
	}
	for i := len(records) - 1; i >= 0; i-- {
		record := records[i]
		if !clear && i < historyKeep && (totalBytes <= 256<<20 || i == 0) {
			continue
		}
		if taskRecordActive(root, record.ID) {
			continue
		}
		for _, suffix := range []string{".events", ".json", ".lock"} {
			if err := os.Remove(historyFile(root, record.ID, suffix)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		totalBytes -= sizes[record.ID]
	}
	return nil
}
func (s *server) remainingHistorySources(ctx context.Context, id string, c toolbox.Config) ([]string, error) {
	root, err := s.historyDirectory(false)
	if err != nil {
		return nil, err
	}
	record, err := readTaskRecord(root, id)
	if err != nil {
		return nil, err
	}
	if record.Status == "running" {
		return nil, errors.New("原任务仍在执行，请等待完成或先取消")
	}
	if !record.Independent || record.Limited {
		return nil, errors.New("此记录不能定向恢复，请载入设置后核对完整预览")
	}
	if c.Tool != record.Tool || !sameHistoryPath(c.Root, record.Config.Root) {
		return nil, errors.New("工具或输入目录已改变，请重新选择对应历史记录")
	}
	items, err := loadHistoryItems(root, record)
	if err != nil {
		return nil, err
	}
	sources := []string{}
	for _, item := range items {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if item.Item.Conflict == "" && (item.Status == "failed" || item.Status == "pending") {
			sources = append(sources, item.Item.Source)
		}
	}
	return sources, nil
}
func (s *server) taskHistoryAPI(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID     string
		Offset int
		Filter string
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fail(w, err)
		return
	}
	if r.URL.Path == "/api/task-history-clear" {
		if err := s.pruneTaskHistory(true); err != nil {
			fail(w, err)
			return
		}
		send(w, map[string]bool{"ok": true})
		return
	}
	if r.URL.Path == "/api/task-history" {
		records, err := s.listTaskHistory()
		if err != nil {
			fail(w, err)
			return
		}
		interrupted := 0
		for _, record := range records {
			if record.Status == "interrupted" {
				interrupted++
			}
		}
		if len(records) > historyKeep {
			records = records[:historyKeep]
		}
		send(w, map[string]any{"Records": records, "Interrupted": interrupted, "Keep": historyKeep})
		return
	}
	root, err := s.historyDirectory(false)
	if err != nil {
		fail(w, err)
		return
	}
	record, err := readTaskRecord(root, req.ID)
	if err != nil {
		fail(w, err)
		return
	}
	items, err := loadHistoryItems(root, record)
	warning := ""
	if err != nil {
		record.Limited = true
		items = nil
		warning = "逐项记录不可读，定向重扫已关闭；仍可载入参数并核对完整预览。"
	}
	remaining := 0
	selected := []historyItem{}
	if record.Independent && record.Status == "interrupted" && warning == "" {
		record.Result = toolbox.Result{}
		record.Done = 0
		for _, item := range items {
			switch item.Status {
			case "success":
				record.Result.Success++
				record.Done++
			case "failed":
				record.Result.Failed++
				record.Done++
			case "skipped":
				record.Result.Skipped++
				record.Done++
			}
		}
	}
	for _, item := range items {
		pending := item.Item.Conflict == "" && (item.Status == "failed" || item.Status == "pending")
		if pending {
			remaining++
		}
		if req.Filter == "all" || pending {
			selected = append(selected, item)
		}
	}
	start := min(max(0, req.Offset), len(selected))
	end := min(start+100, len(selected))
	record.Config = toolbox.RestoreDependencyPaths(record.Config)
	send(w, map[string]any{"Record": record, "Items": selected[start:end], "Total": len(selected), "Offset": start, "Remaining": remaining, "Warning": warning, "CanRetry": record.Independent && !record.Limited && record.Status != "running" && remaining > 0})
}
