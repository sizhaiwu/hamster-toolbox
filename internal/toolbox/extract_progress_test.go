package toolbox

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSevenZipProgressChunksAndNames(t *testing.T) {
	stream := "  0%\b\b\b\b 12% 0 - name100%.txt\r  42% 1\nERROR: 99% not progress\n101%\r-1%\r9999999999999999999999999%\r 100%"
	for chunk := 1; chunk <= len(stream); chunk++ {
		var got []int
		w := sevenZipProgressWriter{notify: func(n int) { got = append(got, n) }}
		for i := 0; i < len(stream); i += chunk {
			if _, err := w.Write([]byte(stream[i:min(i+chunk, len(stream))])); err != nil {
				t.Fatal(err)
			}
		}
		if !reflect.DeepEqual(got, []int{0, 12, 42, 100}) {
			t.Fatal(chunk, got)
		}
	}
}
func TestExtractProgressThrottleAndStageSemantics(t *testing.T) {
	var events []OperationProgress
	ctx := WithOperationProgress(context.Background(), func(p OperationProgress) { events = append(events, p) })
	p := newExtractProgress(ctx, "synthetic.001", func(string) {})
	p.phase("extract", 0, 0)
	p.percent(0)
	count := len(events)
	for i := 1; i < 99; i++ {
		p.percent(i)
	}
	if len(events) != count {
		t.Fatal("progress was not throttled")
	}
	p.percent(100)
	if events[len(events)-1].Percent != 99.9 {
		t.Fatal("reported success before 7-Zip exit")
	}
	p.phase("validate", 0, 0)
	if events[len(events)-1].PercentKnown {
		t.Fatal("fake validation percentage")
	}
	p.phase("publish", 2, 1000)
	p.lastEmit = time.Time{}
	p.copyBytes(500)
	if events[len(events)-1].Percent != 50 || events[len(events)-1].BytesDone != 500 {
		t.Fatal("copy progress missing")
	}
	// These are immutable event values, including the original stage snapshot.
	if events[0].Phase != "extract" || events[0].PercentKnown {
		t.Fatal("old events mutated")
	}
}
func TestExtractProgressLiveIntegration(t *testing.T) {
	c := extractFixture(t)
	body := strings.Repeat("synthetic resource content ", 200000)
	writeExtractZIP(t, filepath.Join(c.Root, "large.zip"), map[string]string{"large.txt": body, "sub/empty.txt": ""})
	p, err := BuildPlan(context.Background(), c, quiet)
	if err != nil || len(p.Items) != 1 || p.Items[0].Conflict != "" {
		t.Fatal(p, err)
	}
	var events []OperationProgress
	ctx := WithOperationProgress(context.Background(), func(v OperationProgress) { events = append(events, v) })
	var logs []string
	r := Execute(ctx, p, func(s string) { logs = append(logs, s) }, progress)
	if r.Success != 1 || r.Failed != 0 {
		t.Fatal(r, logs)
	}
	seen := map[string]bool{}
	known := false
	for _, event := range events {
		if event.ItemNumber != 1 || event.ItemTotal != 1 || event.Source != p.Items[0].Source {
			t.Fatal(event)
		}
		seen[event.Phase] = true
		if event.Phase == "extract" && event.PercentKnown {
			known = true
			if event.Percent >= 100 {
				t.Fatal("premature completion", event)
			}
		}
	}
	for _, phase := range []string{"extract", "validate", "publish", "complete"} {
		if !seen[phase] {
			t.Fatal("missing phase", phase, events)
		}
	}
	if !known {
		t.Fatal("7-Zip output yielded no real progress", events)
	}
	last := events[len(events)-1]
	if last.Phase != "complete" || last.Percent != 100 {
		t.Fatal(last)
	}
	actual, err := os.ReadFile(filepath.Join(p.Items[0].Target, "large.txt"))
	if err != nil || string(actual) != body {
		t.Fatal("published bytes differ", err)
	}
}
func TestExtractProgressCancellationIntegration(t *testing.T) {
	c := extractFixture(t)
	writeExtractZIP(t, filepath.Join(c.Root, "cancel.zip"), map[string]string{"content.txt": strings.Repeat("cancel safely", 50000)})
	p, err := BuildPlan(context.Background(), c, quiet)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	completed := false
	ctx = WithOperationProgress(ctx, func(v OperationProgress) {
		if v.Phase == "extract" {
			cancel()
		}
		if v.Phase == "complete" {
			completed = true
		}
	})
	r := Execute(ctx, p, quiet, progress)
	if completed || r.Success != 0 || !regularFile(p.Items[0].Source) {
		t.Fatal("cancelled extraction reported success", r)
	}
	if _, err = os.Stat(p.Items[0].Target); !os.IsNotExist(err) {
		t.Fatal("cancelled extraction published output", err)
	}
}
func TestExtractCopyByteObserver(t *testing.T) {
	var got int64
	ctx := context.WithValue(context.Background(), copyProgressKey{}, func(n int64) { got += n })
	target := filepath.Join(t.TempDir(), "copied.bin")
	data := bytes.Repeat([]byte("x"), 200000)
	if err := publishReader(ctx, bytes.NewReader(data), target); err != nil {
		t.Fatal(err)
	}
	if got != int64(len(data)) {
		t.Fatal("copy progress did not include a large in-flight file", got)
	}
}
