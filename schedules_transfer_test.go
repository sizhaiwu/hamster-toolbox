package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScheduleBTImport(t *testing.T) {
	raw := json.RawMessage(`[{"name":"Sunday GET","sType":"toUrl","type":"week","where1":"7","where_hour":"9","where_minute":30,"status":1,"urladdress":"https://example.invalid/health","user_agent":"test-agent"},{"name":"POST JSON","sType":"to_post","type":"minute-n","where1":"30","where_hour":"0","where_minute":"0","urladdress":"https://example.invalid/api","post_param":"[{\"paramName\":\"flag\",\"paramValue\":true}]"},{"name":"second task","sType":"toShell","type":"minute-n","where1":"1","where_hour":"1","where_minute":"1","second":"5","sBody":"printf hello"},{"name":"backup site","sType":"site","type":"day"}]`)
	r, e := decodeScheduleImport(raw, nil, "skip")
	if e != nil || len(r.Jobs) != 3 || len(r.Issues) != 1 {
		t.Fatalf("%+v %v", r, e)
	}
	for _, j := range r.Jobs {
		if j.Enabled || j.ID != "" {
			t.Fatal("import enabled/preserved ID")
		}
	}
	if r.Jobs[0].Weekday != 0 || r.Jobs[0].Headers["User-Agent"] != "test-agent" {
		t.Fatal(r.Jobs[0])
	}
	if r.Jobs[1].Method != "POST" || r.Jobs[1].Body != `{"flag":true}` {
		t.Fatal(r.Jobs[1])
	}
	if r.Jobs[2].Interval != 5 || r.Jobs[2].Shell != "bash" {
		t.Fatal(r.Jobs[2])
	}
	if _, e := decodeScheduleImport(json.RawMessage(`{"Version":99,"Jobs":[]}`), nil, "skip"); e == nil {
		t.Fatal("future version accepted")
	}
}
func TestScheduleTransferRoundTrip(t *testing.T) {
	j := scheduledJob{ID: "old", Name: "json request", Kind: "http", Method: "POST", URL: "https://example.invalid/api", Headers: map[string]string{"Content-Type": "application/json"}, Body: `{"count":9007199254740993,"enabled":false,"nested":{"a":"b"}}`, Period: "week", Weekday: 0, AtHour: 9, Timeout: 600, Enabled: true}
	if e := validateScheduledJob(&j); e != nil {
		t.Fatal(e)
	}
	bt, e := exportBTJob(j)
	if e != nil {
		t.Fatal(e)
	}
	if bt["where1"] != "7" || bt["sType"] != "to_post" || bt["status"] != 0 {
		t.Fatal(bt)
	}
	for _, key := range []string{"sBody", "urladdress", "post_param", "type_id", "params", "second", "stop_site", "where_hour", "where_minute", "save", "backupTo", "time_type"} {
		if _, ok := bt[key]; !ok {
			t.Fatal("missing BT field", key)
		}
	}
	b, _ := json.Marshal([]any{bt})
	round, e := decodeScheduleImport(b, nil, "skip")
	if e != nil || len(round.Jobs) != 1 {
		t.Fatal(round, e)
	}
	if !strings.Contains(round.Jobs[0].Body, "9007199254740993") {
		t.Fatal("large JSON integer changed", round.Jobs[0].Body)
	}
	var x, y any
	json.Unmarshal([]byte(j.Body), &x)
	json.Unmarshal([]byte(round.Jobs[0].Body), &y)
	xb, _ := json.Marshal(x)
	yb, _ := json.Marshal(y)
	if string(xb) != string(yb) {
		t.Fatal("POST types lost", string(yb))
	}
	m := newScheduleManager(t.TempDir())
	defer m.stop()
	m.store.Jobs = []scheduledJob{j}
	w := httptest.NewRecorder()
	m.transferAPI(w, "export", "toolbox", "", nil, nil)
	var exp struct {
		Count int
		Data  json.RawMessage
	}
	json.Unmarshal(w.Body.Bytes(), &exp)
	r, e := decodeScheduleImport(exp.Data, nil, "skip")
	if e != nil || exp.Count != 1 || len(r.Jobs) != 1 || r.Jobs[0].Enabled || r.Jobs[0].Headers["Content-Type"] != "application/json" {
		t.Fatal("native export/import", r, e)
	}
	skip, e := decodeScheduleImport(exp.Data, m.store.Jobs, "skip")
	if e != nil || len(skip.Jobs) != 0 || len(skip.Issues) != 1 {
		t.Fatal("duplicate skip")
	}
	rename, e := decodeScheduleImport(exp.Data, m.store.Jobs, "rename")
	if e != nil || len(rename.Jobs) != 1 || rename.Jobs[0].Name == j.Name {
		t.Fatal("duplicate rename")
	}
	unsafe := j
	unsafe.Headers["Authorization"] = "secret"
	if _, e := exportBTJob(unsafe); e == nil {
		t.Fatal("lost auth header")
	}
}
func TestScheduleImportPreviewAndAtomicCommit(t *testing.T) {
	m := newScheduleManager(t.TempDir())
	defer m.stop()
	s := &server{scheduler: m}
	payload := json.RawMessage(`[{"name":"new","sType":"toUrl","type":"day","where_hour":"9","where_minute":"0","urladdress":"https://example.invalid/"}]`)
	q := map[string]any{"Action": "import-preview", "Payload": payload}
	w := scheduleRequest(t, s, q)
	if w.Code != 200 || len(m.store.Jobs) != 0 {
		t.Fatal(w.Body.String())
	}
	if _, e := os.Stat(filepath.Join(m.dir, "schedules.json")); !os.IsNotExist(e) {
		t.Fatal("preview saved")
	}
	q["Action"] = "import"
	w = scheduleRequest(t, s, q)
	if w.Code != 200 || len(m.store.Jobs) != 1 || m.store.Jobs[0].ID == "" || m.store.Jobs[0].Enabled {
		t.Fatal(w.Body.String())
	}
	w = scheduleRequest(t, s, q)
	if len(m.store.Jobs) != 1 {
		t.Fatal("duplicate import")
	}
	// Reject an invalid selection rather than interpreting it as export all.
	w = scheduleRequest(t, s, map[string]any{"Action": "export", "Format": "toolbox", "IDs": []string{"missing"}})
	if w.Code == 200 {
		t.Fatal("missing export selection accepted")
	}
	w = scheduleRequest(t, s, map[string]any{"Action": "export", "Format": "bt"})
	var out struct {
		Count    int
		Warnings []string
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	if out.Count != 1 || len(out.Warnings) == 0 {
		t.Fatal("BT execution differences not disclosed")
	}
}
func TestScheduleBatchOperations(t *testing.T) {
	m := newScheduleManager(t.TempDir())
	defer m.stop()
	for _, id := range []string{"a", "b", "c"} {
		m.store.Jobs = append(m.store.Jobs, scheduledJob{ID: id, Name: id, Kind: "http", Method: "GET", URL: "https://example.invalid/", Interval: 60, Timeout: 1})
	}
	m.active["c"] = func() {}
	w := httptest.NewRecorder()
	m.batchAPI(w, "enable", []string{"a", "b", "a"})
	if !m.store.Jobs[0].Enabled || !m.store.Jobs[1].Enabled {
		t.Fatal("not enabled")
	}
	w = httptest.NewRecorder()
	m.batchAPI(w, "delete", []string{"a", "c", "missing"})
	var result struct{ Results []scheduleBatchResult }
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result.Results) != 3 || !result.Results[0].OK || result.Results[1].OK || len(m.store.Jobs) != 2 {
		t.Fatal(w.Body.String())
	}
	root := t.TempDir()
	block := filepath.Join(root, "file")
	os.WriteFile(block, []byte("x"), 0600)
	m.dir = filepath.Join(block, "child")
	w = httptest.NewRecorder()
	m.batchAPI(w, "disable", []string{"b"})
	if !m.store.Jobs[0].Enabled || !strings.Contains(w.Body.String(), "保存失败") {
		t.Fatal("batch write not rolled back")
	}
}
func TestScheduleExportRejectsUnsupportedWithoutLoss(t *testing.T) {
	j := scheduledJob{Name: "x", Kind: "script", Shell: "powershell", Script: "Write-Output hello", Period: "day", Timeout: 600}
	if _, e := exportBTJob(j); e == nil {
		t.Fatal("PowerShell treated as bash")
	}
	j.Shell = "bash"
	j.Period = "cron"
	j.Cron = "0 9 * * 1"
	if _, e := exportBTJob(j); e == nil {
		t.Fatal("cron silently changed")
	}
}
func TestScheduleBTExampleFixture(t *testing.T) {
	b, e := os.ReadFile("samples/schedules/baota-example.json")
	if e != nil {
		t.Fatal(e)
	}
	r, e := decodeScheduleImport(b, nil, "skip")
	if e != nil || len(r.Jobs) != 3 || len(r.Issues) != 0 {
		t.Fatal(r, e)
	}
	for _, j := range r.Jobs {
		if j.Enabled {
			t.Fatal("fixture enabled")
		}
	}
}
