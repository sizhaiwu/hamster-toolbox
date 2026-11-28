package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const scheduleCombinedLogLimit = 1024 * 1024

// Date-only filters are evaluated in the same server timezone as scheduling.
func scheduleLogWindow(preset, from, to string, now time.Time) (time.Time, time.Time, error) {
	var start, end time.Time
	switch preset {
	case "", "all":
	case "7", "30":
		days := 7
		if preset == "30" {
			days = 30
		}
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -days+1)
	case "custom":
		var e error
		if from == "" || to == "" {
			return start, end, errors.New("请选择开始与结束日期")
		}
		start, e = time.ParseInLocation("2006-01-02", from, now.Location())
		if e != nil {
			return start, end, errors.New("开始日期无效")
		}
		end, e = time.ParseInLocation("2006-01-02", to, now.Location())
		if e != nil {
			return start, end, errors.New("结束日期无效")
		}
		if end.Before(start) {
			return start, end, errors.New("结束日期不能早于开始日期")
		}
		end = end.AddDate(0, 0, 1)
	default:
		return start, end, errors.New("日志时间范围无效")
	}
	return start, end, nil
}
func (m *scheduleManager) scheduleLogs(id string, params map[string]string, now time.Time) (map[string]any, error) {
	start, end, e := scheduleLogWindow(params["range"], params["from"], params["to"], now)
	if e != nil {
		return nil, e
	}
	exists := false
	for _, j := range m.store.Jobs {
		if j.ID == id {
			exists = true
			break
		}
	}
	total, matched, completed := 0, 0, 0
	var size int64
	selected := []scheduleRun{}
	for _, r := range m.store.Runs {
		if r.JobID != id {
			continue
		}
		exists = true
		total++
		if r.Status != "running" && m.live[r.ID] == nil {
			completed++
		}
		if safeScheduleLogID(r.ID) {
			if st, err := os.Stat(m.logPath(r.ID)); err == nil {
				size += st.Size()
			}
		}
		if !start.IsZero() && r.Started.Before(start) || !end.IsZero() && !r.Started.Before(end) {
			continue
		}
		matched++
		selected = append(selected, r)
	}
	if !exists {
		return nil, errors.New("任务或记录不存在")
	}
	// Histories are newest first; stop reading old files after the display budget.
	chunks := []string{}
	used := 0
	truncated := false
	labels := map[string]string{"running": "运行中", "success": "成功", "failed": "失败", "timeout": "超时", "cancelled": "已停止", "interrupted": "中断"}
	for _, r := range selected {
		label := labels[r.Status]
		if label == "" {
			label = r.Status
		}
		output := m.readRunLog(r)
		if output == "" {
			output = "（无输出）"
			if r.Status == "running" {
				output = "（正在执行，暂无输出）"
			}
		}
		chunk := fmt.Sprintf("── [%s] %s ──\n%s\n\n", r.Started.In(now.Location()).Format("2006-01-02 15:04:05"), label, output)
		if used+len(chunk) > scheduleCombinedLogLimit {
			budget := scheduleCombinedLogLimit - used
			if budget > 0 {
				chunk = strings.ToValidUTF8(chunk[len(chunk)-budget:], "�")
				chunks = append(chunks, chunk)
			}
			truncated = true
			break
		}
		chunks = append(chunks, chunk)
		used += len(chunk)
	}
	var b strings.Builder
	if truncated {
		b.WriteString("[合并日志较长，仅显示末尾约 1 MB；可缩小时间范围或切换单次记录]\n\n")
	}
	for i := len(chunks) - 1; i >= 0; i-- {
		b.WriteString(chunks[i])
	}
	zone, offset := now.Zone()
	return map[string]any{"Output": b.String(), "Total": total, "Matched": matched, "Completed": completed, "Bytes": size, "Directory": filepath.Join(m.dir, "schedule-logs"), "Truncated": truncated, "Timezone": fmt.Sprintf("%s UTC%+03d:%02d", zone, offset/3600, absScheduleOffset(offset)%3600/60)}, nil
}
func absScheduleOffset(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Keep active executions intact. Commit the new history before removing files.
func (m *scheduleManager) clearScheduleLogs(id string) (map[string]any, error) {
	exists := false
	for _, j := range m.store.Jobs {
		if j.ID == id {
			exists = true
			break
		}
	}
	before := append([]scheduleRun(nil), m.store.Runs...)
	keep := []scheduleRun{}
	removed := []scheduleRun{}
	for _, r := range m.store.Runs {
		if r.JobID == id {
			exists = true
		}
		if r.JobID == id && r.Status != "running" && m.live[r.ID] == nil {
			removed = append(removed, r)
		} else {
			keep = append(keep, r)
		}
	}
	if !exists {
		return nil, errors.New("任务或记录不存在")
	}
	if len(removed) == 0 {
		return map[string]any{"Count": 0, "PendingFiles": 0}, nil
	}
	m.store.Runs = keep
	if e := m.save(); e != nil {
		m.store.Runs = before
		return nil, e
	}
	pending := 0
	for _, r := range removed {
		if !safeScheduleLogID(r.ID) {
			continue
		}
		if e := os.Remove(m.logPath(r.ID)); e != nil && !errors.Is(e, os.ErrNotExist) {
			pending++
		}
	}
	return map[string]any{"Count": len(removed), "PendingFiles": pending}, nil
}
