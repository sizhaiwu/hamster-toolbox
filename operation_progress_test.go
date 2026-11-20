package main

import (
	"encoding/json"
	"testing"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

func TestOperationProgressStateAndSummary(t *testing.T) {
	s := &server{token: "progress-test", state: state{Status: "running", Tool: "extract", Total: 2}}
	p := toolbox.OperationProgress{Tool: "extract", Source: "synthetic.001", Phase: "extract", ItemNumber: 1, ItemTotal: 2, Percent: 25, PercentKnown: true}
	s.operationProgress(p)
	p.Percent = 90
	if s.state.Operation == nil || s.state.Operation.Percent != 25 {
		t.Fatal("event snapshot was not independent")
	}
	w := requestAPI(t, s, "/api/state", map[string]bool{"Summary": true})
	var got state
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil || got.Operation == nil || got.Operation.Percent != 25 || got.Done != 0 {
		t.Fatal(w.Body.String())
	}
	s.progress(1, 2)
	if s.state.Operation != nil {
		t.Fatal("previous archive progress leaked into next item")
	}
	s.state.Status = "finished"
	s.operationProgress(p)
	if s.state.Operation != nil {
		t.Fatal("late callback changed completed task")
	}
	s.state.Status = "running"
	s.state.Tool = "image"
	s.operationProgress(p)
	if s.state.Operation != nil {
		t.Fatal("different tool received stale progress")
	}
}
