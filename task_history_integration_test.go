package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

func TestTaskHistoryObservedMediaIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") != "1" {
		t.Skip("set TOOLBOX_INTEGRATION=1")
	}
	c := toolbox.DefaultConfig()
	c.Tool = "image"
	c.Root = t.TempDir()
	c.Output = t.TempDir()
	c.Magick, _ = filepath.Abs("tools/imagemagick/magick.exe")
	c.ImageFormats = []string{".png"}
	c.PngMode = "webp-png"
	c.Workers = 2
	img := image.NewNRGBA(image.Rect(0, 0, 320, 180))
	for y := 0; y < 180; y++ {
		for x := 0; x < 320; x++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(x), uint8(y), 90, 255})
		}
	}
	var buffer bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.NoCompression}
	if err := encoder.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"completed.png", "failed.png"} {
		if err := os.WriteFile(filepath.Join(c.Root, name), buffer.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	p, err := toolbox.BuildPlan(context.Background(), c, func(string) {})
	if err != nil || len(p.Items) != 2 {
		t.Fatal(p, err)
	}
	// Change a source after preview: the ordinary execution guard must fail it,
	// while another worker publishes the unchanged source successfully.
	f, _ := os.OpenFile(filepath.Join(c.Root, "failed.png"), os.O_APPEND|os.O_WRONLY, 0600)
	f.Write([]byte("changed"))
	f.Close()
	s := &server{token: "test-session", dataDir: t.TempDir(), plan: p, state: state{Status: "ready", PlanID: "media-plan", Tool: "image", Total: 2, Items: p.Items}}
	w := requestAPI(t, s, "/api/execute", map[string]string{"PlanID": "media-plan"})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var record taskRecord
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		records, _ := s.listTaskHistory()
		if len(records) == 1 && records[0].Status == "finished" {
			record = records[0]
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if record.ID == "" || record.Result.Success != 1 || record.Result.Failed != 1 {
		t.Fatal(record)
	}
	response := requestAPI(t, s, "/api/task-history-detail", map[string]any{"ID": record.ID})
	var detail struct {
		Remaining int
		CanRetry  bool
		Items     []historyItem
	}
	if err = json.Unmarshal(response.Body.Bytes(), &detail); err != nil || detail.Remaining != 1 || !detail.CanRetry || len(detail.Items) != 1 {
		t.Fatal(response.Body.String(), err)
	}
	response = requestAPI(t, s, "/api/plan", map[string]any{"Tool": "image", "Root": c.Root, "Output": c.Output, "Magick": c.Magick, "ImageFormats": c.ImageFormats, "PngMode": c.PngMode, "PngQuality": 90, "Workers": 2, "Recursive": true, "RetryTaskID": record.ID})
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		ready := s.state.Status == "ready"
		failed := s.state.Error
		p = s.plan
		s.mu.Unlock()
		if ready {
			break
		}
		if failed != "" {
			t.Fatal(failed)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if p == nil || len(p.Items) != 1 || filepath.Base(p.Items[0].Source) != "failed.png" {
		t.Fatal(p)
	}
	if _, err = os.Stat(filepath.Join(c.Output, "completed.png")); err != nil {
		t.Fatal("successful output disappeared", err)
	}
}
