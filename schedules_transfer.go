package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type scheduleTransferIssue struct{ Name, Reason string }
type scheduleImportResult struct {
	Jobs   []scheduledJob
	Issues []scheduleTransferIssue
}

func transferText(m map[string]json.RawMessage, key string) string {
	v := m[key]
	if len(v) == 0 || bytes.Equal(v, []byte("null")) {
		return ""
	}
	var s string
	if json.Unmarshal(v, &s) == nil {
		return s
	}
	return string(v)
}
func transferInt(m map[string]json.RawMessage, key string) (int, error) {
	s := transferText(m, key)
	if s == "" {
		return 0, nil
	}
	return strconv.Atoi(s)
}
func importBTJob(m map[string]json.RawMessage) (scheduledJob, error) {
	j := scheduledJob{Name: transferText(m, "name"), Timeout: 600, Headers: map[string]string{}}
	period := transferText(m, "type")
	j.Period = period
	var e error
	if j.AtHour, e = transferInt(m, "where_hour"); e != nil {
		return j, errors.New("小时不是整数")
	}
	if j.AtMinute, e = transferInt(m, "where_minute"); e != nil {
		return j, errors.New("分钟不是整数")
	}
	where, e := transferInt(m, "where1")
	if e != nil {
		return j, errors.New("周期值不是整数")
	}
	if transferText(m, "keyword") != "" {
		return j, errors.New("宝塔响应关键字断言暂不支持，未忽略该条件")
	}
	if transferText(m, "time_type") != "" || transferText(m, "special_time") != "" || transferText(m, "time_set") != "" {
		return j, errors.New("暂不支持宝塔特殊时段规则，请改为 Cron 后迁移")
	}
	second, e := transferInt(m, "second")
	if e != nil {
		return j, errors.New("秒数无效")
	}
	if second > 0 {
		j.Period = "second-n"
		j.Every = second
		j.AtHour = 0
		j.AtMinute = 0
	}
	switch j.Period {
	case "day", "hour":
	case "day-n", "hour-n", "minute-n":
		j.Every = where
	case "second-n":
		if j.Every == 0 {
			j.Every = where
		}
	case "week":
		j.Weekday = where
		if j.Weekday == 7 {
			j.Weekday = 0
		}
	case "month":
		j.Day = where
	default:
		return j, errors.New("不支持此宝塔执行周期")
	}
	switch transferText(m, "sType") {
	case "toShell":
		j.Kind = "script"
		j.Shell = "bash"
		j.Script = transferText(m, "sBody")
	case "toUrl", "to_post":
		j.Kind = "http"
		j.Method = "GET"
		j.URL = transferText(m, "urladdress")
		if j.URL == "" {
			j.URL = transferText(m, "sBody")
		}
		if ua := transferText(m, "user_agent"); ua != "" {
			j.Headers["User-Agent"] = ua
		}
		if transferText(m, "sType") == "to_post" {
			j.Method = "POST"
			j.Headers["Content-Type"] = "application/json"
			var params []struct {
				ParamName  string `json:"paramName"`
				ParamValue any    `json:"paramValue"`
			}
			raw := transferText(m, "post_param")
			if raw == "" {
				raw = "[]"
			}
			decoder := json.NewDecoder(strings.NewReader(raw))
			decoder.UseNumber()
			if e := decoder.Decode(&params); e != nil || !json.Valid([]byte(raw)) {
				return j, errors.New("POST 参数不是宝塔参数列表")
			}
			body := map[string]any{}
			for _, p := range params {
				if p.ParamName == "" {
					return j, errors.New("POST 参数名为空")
				}
				body[p.ParamName] = p.ParamValue
			}
			b, _ := json.Marshal(body)
			j.Body = string(b)
		}
	default:
		return j, errors.New("仅支持 Shell、GET 和 POST；此任务依赖宝塔专用功能")
	}
	return j, nil
}
func decodeScheduleImport(payload json.RawMessage, existing []scheduledJob, conflict string) (scheduleImportResult, error) {
	result := scheduleImportResult{Jobs: []scheduledJob{}, Issues: []scheduleTransferIssue{}}
	if conflict != "" && conflict != "skip" && conflict != "rename" {
		return result, errors.New("重名处理无效")
	}
	raw := bytes.TrimPrefix(bytes.TrimSpace(payload), []byte{0xef, 0xbb, 0xbf})
	var rows []json.RawMessage
	if len(raw) > 0 && raw[0] == '[' {
		if e := json.Unmarshal(raw, &rows); e != nil {
			return result, errors.New("JSON 数组无效")
		}
	} else {
		var envelope map[string]json.RawMessage
		if json.Unmarshal(raw, &envelope) != nil {
			return result, errors.New("请导入工具箱或宝塔 JSON 文件")
		}
		if v, ok := envelope["Jobs"]; ok {
			version, e := transferInt(envelope, "Version")
			if e != nil || version != 1 {
				return result, errors.New("工具箱导入文件版本不支持")
			}
			if e = json.Unmarshal(v, &rows); e != nil {
				return result, errors.New("任务列表无效")
			}
		} else if _, ok := envelope["sType"]; ok {
			rows = []json.RawMessage{raw}
		} else {
			return result, errors.New("不支持的导入文件结构")
		}
	}
	if len(rows) == 0 || len(rows) > 100 {
		return result, errors.New("一次需导入 1～100 个任务")
	}
	names := map[string]bool{}
	for _, j := range existing {
		names[j.Name] = true
	}
	for i, row := range rows {
		var m map[string]json.RawMessage
		var j scheduledJob
		var e error
		if len(row) > 160*1024 {
			e = errors.New("单个任务超过 160 KB")
		} else if json.Unmarshal(row, &m) != nil || m == nil {
			e = errors.New("任务条目无效")
		} else if _, ok := m["sType"]; ok {
			j, e = importBTJob(m)
		} else {
			e = json.Unmarshal(row, &j)
			if e == nil && j.Kind == "" {
				e = errors.New("不是工具箱任务或宝塔任务")
			}
		}
		name := j.Name
		if name == "" {
			name = fmt.Sprintf("第 %d 项", i+1)
		}
		if e == nil {
			j.ID = ""
			j.Error = ""
			j.Queued = false
			j.Next = time.Time{}
			j.Enabled = false
			e = validateScheduledJob(&j)
		}
		if e == nil {
			_, e = nextScheduledTime(j, time.Now())
		}
		if e == nil && names[j.Name] {
			if conflict != "rename" {
				e = errors.New("同名任务已存在，未覆盖")
			} else {
				base := j.Name
				for n := 2; ; n++ {
					suffix := fmt.Sprintf("（导入%d）", n)
					head := []rune(base)
					for len(string(head))+len(suffix) > 200 {
						head = head[:len(head)-1]
					}
					candidate := string(head) + suffix
					if !names[candidate] {
						j.Name = candidate
						break
					}
				}
			}
		}
		if e == nil && len(existing)+len(result.Jobs) >= 100 {
			e = errors.New("已达到最多 100 个任务")
		}
		if e != nil {
			result.Issues = append(result.Issues, scheduleTransferIssue{Name: name, Reason: e.Error()})
			continue
		}
		names[j.Name] = true
		result.Jobs = append(result.Jobs, j)
	}
	return result, nil
}
func exportBTJob(j scheduledJob) (map[string]any, error) {
	if len(j.ReadPaths)+len(j.WritePaths) > 0 {
		return nil, errors.New("目录互斥配置不能完整迁移到宝塔，请用工具箱格式")
	}

	if j.ExpectedStatus != 0 || j.ResponseContains != "" || j.RetryCount > 0 {
		return nil, errors.New("HTTP 检查和重试无法完整转换，请用工具箱格式")
	}
	if j.Period == "" || j.Period == "cron" {
		return nil, errors.New("自定义 Cron 或旧固定间隔无法无损转换，请用工具箱格式")
	}
	if j.Directory != "" {
		return nil, errors.New("宝塔无法保留工作目录，请用工具箱格式")
	}
	row := map[string]any{}
	for _, key := range strings.Split("echo,backupTo,sName,notice,notice_channel,db_type,split_type,split_value,keyword,post_param,time_set,backup_mode,db_backup_path,time_type,special_time,user_agent,version,table_list,log_cut_path,rname,type_id,second,stop_site", ",") {
		row[key] = ""
	}
	row["name"] = j.Name
	row["type"] = j.Period
	row["where_hour"] = strconv.Itoa(j.AtHour)
	row["where_minute"] = strconv.Itoa(j.AtMinute)
	row["where1"] = ""
	row["addtime"] = time.Now().Format("2006-01-02 15:04:05")
	row["status"] = 0
	row["save"] = 3
	row["save_local"] = 1
	row["flock"] = 1
	row["result"] = 1
	row["params"] = "{}"
	row["sBody"] = ""
	row["urladdress"] = ""
	switch j.Period {
	case "day-n", "hour-n", "minute-n":
		row["where1"] = strconv.Itoa(j.Every)
	case "week":
		week := j.Weekday
		if week == 0 {
			week = 7
		}
		row["where1"] = strconv.Itoa(week)
	case "month":
		row["where1"] = strconv.Itoa(j.Day)
	case "second-n":
		row["type"] = "minute-n"
		row["where1"] = "1"
		row["second"] = strconv.Itoa(j.Every)
		row["where_hour"] = "1"
		row["where_minute"] = "1"
	}
	if j.Kind == "script" {
		if j.Shell != "sh" && j.Shell != "bash" {
			return nil, errors.New("PowerShell / CMD 不能转换为 Linux Shell，请用工具箱格式")
		}
		row["sType"] = "toShell"
		row["sBody"] = j.Script
	} else {
		row["sType"] = "toUrl"
		row["urladdress"] = j.URL
		for k, v := range j.Headers {
			if strings.EqualFold(k, "User-Agent") {
				row["user_agent"] = v
			} else if !strings.EqualFold(k, "Content-Type") {
				return nil, errors.New("宝塔无法完整保留自定义请求头，请用工具箱格式")
			} else if j.Method != "POST" || !strings.EqualFold(strings.TrimSpace(v), "application/json") {
				return nil, errors.New("Content-Type 无法无损转换")
			}
		}
		if j.Method == "POST" {
			contentType := ""
			for k, v := range j.Headers {
				if strings.EqualFold(k, "Content-Type") {
					contentType = v
				}
			}
			if !strings.EqualFold(contentType, "application/json") {
				return nil, errors.New("宝塔 POST 固定使用 application/json，请用工具箱格式保留原请求头")
			}
			var body map[string]any
			dec := json.NewDecoder(strings.NewReader(j.Body))
			dec.UseNumber()
			if !json.Valid([]byte(j.Body)) || dec.Decode(&body) != nil || body == nil {
				return nil, errors.New("宝塔 POST 需要 JSON 对象请求体")
			}
			params := []map[string]any{}
			for k, v := range body {
				params = append(params, map[string]any{"paramName": k, "paramValue": v})
			}
			b, _ := json.Marshal(params)
			row["post_param"] = string(b)
			row["sType"] = "to_post"
		}
	}
	return row, nil
}
func (m *scheduleManager) transferAPI(w http.ResponseWriter, action, format, conflict string, ids []string, payload json.RawMessage) {
	selected := map[string]bool{}
	for _, id := range ids {
		if selected[id] {
			continue
		}
		selected[id] = true
	}
	if len(ids) > 100 {
		fail(w, errors.New("一次最多选择 100 项"))
		return
	}
	if action == "import-preview" || action == "import" {
		result, e := decodeScheduleImport(payload, m.store.Jobs, conflict)
		if e != nil {
			fail(w, e)
			return
		}
		if action == "import" {
			before := append([]scheduledJob(nil), m.store.Jobs...)
			for i := range result.Jobs {
				result.Jobs[i].ID = randomID()
				result.Jobs[i].Next, _ = nextScheduledTime(result.Jobs[i], time.Now())
				m.store.Jobs = append(m.store.Jobs, result.Jobs[i])
			}
			if len(result.Jobs) > 0 {
				if e = m.save(); e != nil {
					m.store.Jobs = before
					fail(w, e)
					return
				}
			}
		}
		// Preview reports metadata only; never echo scripts or credentials into summaries.
		names := []string{}
		for _, j := range result.Jobs {
			names = append(names, j.Name)
		}
		send(w, map[string]any{"Count": len(result.Jobs), "Names": names, "Issues": result.Issues})
		return
	}
	if action == "export" {
		jobs := []scheduledJob{}
		for _, j := range m.store.Jobs {
			if len(ids) == 0 || selected[j.ID] {
				jobs = append(jobs, j)
				delete(selected, j.ID)
			}
		}
		if len(selected) > 0 {
			fail(w, errors.New("部分所选任务已不存在，请刷新"))
			return
		}
		issues := []scheduleTransferIssue{}
		warnings := []string{}
		var output any
		count := len(jobs)
		if format == "bt" {
			warnings = append(warnings, "宝塔部分版本导入后会立即启用任务，即使文件标记为暂停；请在目标面板核对并控制启用时间。", "脚本解释器、HTTP 成功条件、超时和重试由宝塔控制，不能保留工具箱的全部执行语义；时间按目标服务器时区计算。")
			rows := []map[string]any{}
			for _, j := range jobs {
				row, e := exportBTJob(j)
				if e != nil {
					issues = append(issues, scheduleTransferIssue{j.Name, e.Error()})
					continue
				}
				rows = append(rows, row)
			}
			count = len(rows)
			output = rows
		} else if format == "toolbox" {
			output = struct {
				Format  string
				Version int
				Jobs    []scheduledJob
			}{"hamster-toolbox.schedules", 1, jobs}
		} else {
			fail(w, errors.New("导出格式无效"))
			return
		}
		send(w, map[string]any{"Count": count, "Data": output, "Issues": issues, "Warnings": warnings})
		return
	}
	fail(w, errors.New("导入导出操作无效"))
}

type scheduleBatchResult struct {
	ID, Name, Error string
	OK, Queued      bool
}

func (m *scheduleManager) batchAPI(w http.ResponseWriter, operation string, ids []string) {
	if len(ids) == 0 || len(ids) > 100 {
		fail(w, errors.New("请明确选择 1～100 个任务"))
		return
	}
	if operation != "enable" && operation != "disable" && operation != "run" && operation != "cancel" && operation != "delete" {
		fail(w, errors.New("批量操作无效"))
		return
	}
	results := []scheduleBatchResult{}
	seen := map[string]bool{}
	before := append([]scheduledJob(nil), m.store.Jobs...)
	beforeQueue := append([]string(nil), m.queue...)
	changed := false
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		idx := -1
		for i, j := range m.store.Jobs {
			if j.ID == id {
				idx = i
				break
			}
		}
		r := scheduleBatchResult{ID: id}
		if idx < 0 {
			r.Error = "任务不存在"
			results = append(results, r)
			continue
		}
		r.Name = m.store.Jobs[idx].Name
		switch operation {
		case "run":
			if e := m.submit(id, time.Now()); e != nil {
				r.Error = e.Error()
			}
		case "cancel":
			if m.isQueued(id) {
				m.unqueue(id)
			} else if cancel := m.active[id]; cancel != nil {
				cancel()
			} else {
				r.Error = "任务未在运行"
			}
		case "delete":
			if m.active[id] != nil {
				r.Error = "请先停止正在运行的任务"
			} else {
				m.unqueue(id)
				m.store.Jobs = append(m.store.Jobs[:idx], m.store.Jobs[idx+1:]...)
				changed = true
			}
		case "enable", "disable":
			if operation == "enable" {
				check := m.store.Jobs[idx]
				if e := validateScheduledJob(&check); e != nil {
					r.Error = e.Error()
					break
				}
			} else {
				m.unqueue(id)
			}
			m.store.Jobs[idx].Error = ""
			m.store.Jobs[idx].Enabled = operation == "enable"
			m.store.Jobs[idx].Next, _ = nextScheduledTime(m.store.Jobs[idx], time.Now())
			changed = true
		}
		r.OK = r.Error == ""
		r.Queued = m.isQueued(id)
		results = append(results, r)
	}
	if changed {
		if e := m.save(); e != nil {
			m.store.Jobs = before
			m.queue = beforeQueue
			for i := range results {
				if results[i].OK {
					results[i].OK = false
					results[i].Error = "保存失败，设置未更改：" + e.Error()
				}
			}
		}
	}
	send(w, map[string]any{"Results": results})
}
