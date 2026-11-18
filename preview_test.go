package main

import (
	"encoding/json"
	"fmt"
	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
	"testing"
)

func TestPreviewPaginationAndFilteringAcrossFullPlan(t *testing.T) {
	p := &toolbox.Plan{Items: []toolbox.Item{}}
	for i := 0; i < 1205; i++ {
		it := toolbox.Item{Source: fmt.Sprintf("source-%d", i), Target: fmt.Sprintf("target-%d", i)}
		if i%10 == 0 {
			it.Conflict = "冲突"
		}
		p.Items = append(p.Items, it)
	}
	s := &server{token: "test-session", plan: p, state: state{PlanID: "plan", Items: p.Items[:1000], Status: "ready"}}
	for _, tc := range []struct {
		filter                string
		offset, total, length int
		first                 string
	}{
		{"all", 1100, 1205, 100, "source-1100"}, {"all", 1200, 1205, 5, "source-1200"}, {"conflicts", 100, 121, 21, "source-1000"}, {"ready", 1000, 1084, 84, "source-1112"},
	} {
		w := requestAPI(t, s, "/api/preview", previewRequest{PlanID: "plan", Filter: tc.filter, Offset: tc.offset, Limit: 100})
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var page previewPage
		if e := json.Unmarshal(w.Body.Bytes(), &page); e != nil {
			t.Fatal(e)
		}
		if page.Total != tc.total || len(page.Items) != tc.length || page.Items[0].Source != tc.first {
			t.Fatal(tc, page.Total, len(page.Items), page.Items)
		}
	}
	w := requestAPI(t, s, "/api/preview", previewRequest{PlanID: "outdated"})
	if w.Code != 400 {
		t.Fatal("stale plan accepted")
	}
	w = requestAPI(t, s, "/api/state", map[string]bool{"Summary": true})
	var summary state
	if e := json.Unmarshal(w.Body.Bytes(), &summary); e != nil {
		t.Fatal(e)
	}
	if len(summary.Items) != 0 || summary.PlanID != "plan" {
		t.Fatal(summary)
	}
	if len(s.state.Items) != 1000 {
		t.Fatal("summary mutated server state")
	}
}

func TestVideoCommandRequiresCurrentPlanMembership(t *testing.T) {
	s := &server{token: "test-session", plan: &toolbox.Plan{Config: toolbox.Config{Tool: "video"}, Items: []toolbox.Item{{Source: "allowed", Conflict: "skip"}}}, state: state{PlanID: "current"}}
	for _, body := range []map[string]string{{"PlanID": "stale", "Source": "allowed"}, {"PlanID": "current", "Source": "outside"}, {"PlanID": "current", "Source": "allowed"}} {
		w := requestAPI(t, s, "/api/video-commands", body)
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
