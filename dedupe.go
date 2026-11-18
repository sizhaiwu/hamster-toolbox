package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

func (s *server) dedupeAPI(w http.ResponseWriter, r *http.Request) {
	var req struct{ Path string }
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	if e := json.NewDecoder(r.Body).Decode(&req); e != nil {
		fail(w, fmt.Errorf("程序路径请求格式无效"))
		return
	}
	info := toolbox.InspectDedupeProgram(req.Path)
	if r.URL.Path == "/api/dedupe-info" {
		send(w, info)
		return
	}
	if !info.Available {
		fail(w, fmt.Errorf("%s", info.Message))
		return
	}
	cmd, e := toolbox.DedupeLaunchCommand(info.Path)
	if e != nil {
		fail(w, e)
		return
	}
	if e = cmd.Start(); e != nil {
		fail(w, fmt.Errorf("无法启动去重软件，请确认已完整解压，并能直接打开该程序"))
		return
	}
	go func() { _ = cmd.Wait() }()
	send(w, map[string]string{"Message": "已发送启动请求，请在 Video Duplicate Finder 窗口中添加目录并扫描。"})
}
