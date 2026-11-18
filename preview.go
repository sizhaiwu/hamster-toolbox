package main

import (
	"encoding/json"
	"fmt"
	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
	"net/http"
)

type previewRequest struct {
	PlanID, Filter string
	Offset, Limit  int
}

func (s *server) videoCommands(w http.ResponseWriter, r *http.Request) {
	var req struct{ PlanID, Source string }
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	if e := json.NewDecoder(r.Body).Decode(&req); e != nil {
		fail(w, e)
		return
	}
	s.mu.Lock()
	p, id := s.plan, s.state.PlanID
	s.mu.Unlock()
	if p == nil || id == "" || req.PlanID != id {
		fail(w, fmt.Errorf("预览已更新，请重新扫描"))
		return
	}
	for _, item := range p.Items {
		if item.Source == req.Source {
			commands, e := toolbox.VideoCommands(r.Context(), p.Config, item)
			if e != nil {
				fail(w, e)
				return
			}
			send(w, commands)
			return
		}
	}
	fail(w, fmt.Errorf("该文件不在当前预览中"))
}

type previewPage struct {
	PlanID               string
	Items                []toolbox.Item
	Total, Offset, Limit int
}

func (s *server) preview(w http.ResponseWriter, r *http.Request) {
	var req previewRequest
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	if e := json.NewDecoder(r.Body).Decode(&req); e != nil {
		fail(w, e)
		return
	}
	if req.Filter != "" && req.Filter != "all" && req.Filter != "conflicts" && req.Filter != "ready" {
		fail(w, fmt.Errorf("未知预览筛选条件"))
		return
	}
	if req.Offset < 0 {
		req.Offset = 0
	}
	if req.Limit <= 0 {
		req.Limit = 100
	}
	if req.Limit > 500 {
		req.Limit = 500
	}
	s.mu.Lock()
	p, id := s.plan, s.state.PlanID
	s.mu.Unlock()
	if p == nil || id == "" || req.PlanID != id {
		fail(w, fmt.Errorf("预览已更新，请重新扫描"))
		return
	}
	page := previewPage{PlanID: id, Offset: req.Offset, Limit: req.Limit, Items: []toolbox.Item{}}
	for _, it := range p.Items {
		if req.Filter == "conflicts" && it.Conflict == "" || req.Filter == "ready" && it.Conflict != "" {
			continue
		}
		if page.Total >= req.Offset && len(page.Items) < req.Limit {
			page.Items = append(page.Items, it)
		}
		page.Total++
	}
	send(w, page)
}
