package toolbox

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVideoEncoderTimeBase(t *testing.T) {
	for _, input := range []string{"", "0/0", "-1/100", "1/0", "x/10", "1/999999999999"} {
		if videoEncoderTimeBase(input) != "" {
			t.Fatal(input)
		}
	}
	if videoEncoderTimeBase("1/1000000") != "1:1000000" {
		t.Fatal("source precision not retained")
	}
	c := DefaultConfig()
	c.VideoMode = "efficient"
	s := newVideoSession(c, func(string) {})
	args := strings.Join(s.commandArgs("input.mp4", "output.mp4", videoAttempt{encoder: "libx265"}, compressionVideo{Video: compressionStream{TimeBase: "1/1000000"}}, false), " ")
	if !strings.Contains(args, "-enc_time_base:v 1:1000000") {
		t.Fatal(args)
	}
}
func TestVideoTimestampPrecisionIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") != "1" {
		t.Skip("set TOOLBOX_INTEGRATION=1 for real FFmpeg")
	}
	c := integrationConfig(t)
	ctx := context.Background()
	dir := t.TempDir()
	source, target := filepath.Join(dir, "vfr.mp4"), filepath.Join(dir, "compressed.mp4")
	_, err := run(ctx, c.FFmpeg, "-v", "error", "-nostdin", "-n", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=30", "-t", "2", "-vf", `settb=1/1000000,setpts=N/(30*TB)+if(eq(mod(N\,30)\,10)\,0.020/TB\,0)`, "-fps_mode", "passthrough", "-enc_time_base:v", "1:1000000", "-c:v", "libx264", "-preset", "ultrafast", source)
	if err != nil {
		t.Fatal(err)
	}
	c.VideoMode = "efficient"
	c.VideoAcceleration = "cpu"
	c.CRF = 23
	s := newVideoSession(c, func(line string) { t.Log(line) })
	if err = s.compress(ctx, source, target); err != nil {
		t.Fatal(err)
	}
	frames := func(path string) string {
		out, err := run(ctx, c.FFprobe, "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=nb_frames", "-of", "json", path)
		if err != nil {
			t.Fatal(err)
		}
		var v struct {
			Streams []struct {
				Frames string `json:"nb_frames"`
			}
		}
		if json.Unmarshal(out, &v) != nil || len(v.Streams) != 1 {
			t.Fatal(string(out))
		}
		return v.Streams[0].Frames
	}
	if before, after := frames(source), frames(target); before == "" || before != after {
		t.Fatal("frame count changed", before, after)
	}
}
