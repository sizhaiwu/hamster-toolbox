package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

// Optional browser acceptance fixture. All task states, paths and data are
// synthetic; no user files are processed and no real sharing settings are read.
func TestToolTasksBrowserAcceptance(t *testing.T) {
	output := os.Getenv("TOOLBOX_MULTI_UI")
	if output == "" {
		t.Skip("optional browser fixture")
	}
	owner, _, _ := workspaceFixture(t)
	repo, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"image", "video"} {
		c := toolbox.DefaultConfig()
		c.Tool = tool
		c.Root = `D:\示例资源\素材`
		c.Output = `D:\示例结果\` + tool
		c.FFmpeg = filepath.Join(repo, "tools", "ffmpeg", "bin", "ffmpeg.exe")
		c.FFprobe = filepath.Join(repo, "tools", "ffmpeg", "bin", "ffprobe.exe")
		c.Magick = filepath.Join(repo, "tools", "imagemagick", "magick.exe")
		slot := owner.toolTask(tool)
		slot.reserved = &c
		slot.state = state{Tool: tool, Status: "running", TaskID: "synthetic-" + tool, PlanID: randomID(), Config: publicTaskConfig(c), Total: 12, Done: 5, Logs: []string{"12:00:00 合成演示任务正在运行；未处理私人资源"}}
		if tool == "video" {
			slot.state.Total = 1
			slot.state.Done = 0
			slot.state.Operation = &toolbox.OperationProgress{Tool: tool, Phase: "encoding", PercentKnown: true, Percent: 61}
		}
		slot.plan = &toolbox.Plan{Config: c}
		for i := 0; i < slot.state.Total; i++ {
			ext := ".jpg"
			if tool == "video" {
				ext = ".mp4"
			}
			name := fmt.Sprintf("演示素材-%02d%s", i+1, ext)
			slot.plan.Items = append(slot.plan.Items, toolbox.Item{Source: filepath.Join(c.Root, name), Target: filepath.Join(c.Output, name), Size: 8 * 1024 * 1024, Note: "合成示例：保留原件"})
		}
		slot.state.Actionable = slot.state.Total
	}
	srv := httptest.NewServer(owner.httpHandler())
	defer srv.Close()
	data, _ := json.Marshal(map[string]string{"URL": srv.URL + "/?session=" + owner.token, "StopFile": output + ".stop", "CompleteFile": output + ".complete"})
	if err = os.WriteFile(output, data, 0600); err != nil {
		t.Fatal(err)
	}
	completed := false
	for end := time.Now().Add(8 * time.Minute); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
		if _, err := os.Stat(output + ".stop"); err == nil {
			return
		}
		if !completed {
			if _, err := os.Stat(output + ".complete"); err == nil {
				for _, tool := range []string{"image", "video"} {
					slot := owner.toolTask(tool)
					slot.mu.Lock()
					slot.state.Status = "finished"
					slot.state.Done = slot.state.Total
					slot.state.Result.Success = slot.state.Total
					slot.state.Operation = nil
					snapshot := slot.state
					slot.mu.Unlock()
					owner.recordTaskCompletion(snapshot)
				}
				completed = true
			}
		}
	}
	t.Fatal("browser fixture timed out")
}
