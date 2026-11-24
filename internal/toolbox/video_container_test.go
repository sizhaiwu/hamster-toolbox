package toolbox

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVideoContainerOptions(t *testing.T) {
	c := DefaultConfig()
	for _, name := range []string{"clip.mov", "clip.MOV", "clip.m4v", "clip.qt", "clip.mkv", "clip.ts", "clip.mp4"} {
		got, err := videoOutputPath(c, name)
		if err != nil || got != name {
			t.Fatal(got, err)
		}
		s := newVideoSession(c, quiet)
		args := strings.Join(s.commandArgs("input.mp4", name, videoAttempt{"libx265"}, compressionVideo{}, false), " ")
		iso := videoMuxer(name) == "mp4" || videoMuxer(name) == "mov"
		if strings.Contains(args, "-movflags") != iso || strings.Contains(args, "hvc1") != iso {
			t.Fatal(name, args)
		}
		if !strings.Contains(args, "-f "+videoMuxer(name)) {
			t.Fatal(args)
		}
	}
	for _, name := range []string{"clip.webm", "clip.avi", "clip.wmv", "clip.flv", "clip.mpg"} {
		if _, err := videoOutputPath(c, name); err == nil {
			t.Fatal("unsupported original accepted", name)
		}
	}
	c.VideoContainer = "mp4"
	if out, err := videoOutputPath(c, "clip.mov"); err != nil || out != "clip.mp4" {
		t.Fatal(out, err)
	}
	c.VideoContainer = "invalid"
	if normalizeVideoOptions(&c) == nil {
		t.Fatal("invalid option accepted")
	}
}

func TestVideoContainerIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") != "1" {
		t.Skip("requires FFmpeg")
	}
	ctx := context.Background()
	base := integrationConfig(t)
	for _, ext := range []string{"mov", "mp4", "m4v", "qt", "mkv", "ts"} {
		t.Run(ext, func(t *testing.T) {
			c := base
			c.Tool, c.Root, c.MinMB, c.MinKbps = "video", t.TempDir(), 0, 0
			c.VideoAcceleration = "cpu"
			c.CRF = 28
			src := filepath.Join(c.Root, "中文 样片."+ext)
			if _, err := run(ctx, c.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=160x96:rate=24", "-f", "lavfi", "-i", "sine=sample_rate=48000", "-t", "2", "-c:v", "libx264", "-crf", "0", "-c:a", "aac", "-f", videoMuxer(src), src); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(src)
			if err != nil {
				t.Fatal(err)
			}
			for _, mode := range []string{"compatible", "efficient"} {
				c.VideoMode = mode
				c.Output = filepath.Join(t.TempDir(), "out-"+mode)
				p, err := BuildPlan(ctx, c, quiet)
				if err != nil || len(p.Items) != 1 || p.Items[0].Conflict != "" {
					t.Fatal(p, err)
				}
				if filepath.Base(p.Items[0].Target) != filepath.Base(src) {
					t.Fatal("filename changed", p.Items)
				}
				logs := []string{}
				r := Execute(ctx, p, func(s string) { logs = append(logs, s) }, progress)
				if r.Success != 1 || r.Failed != 0 {
					t.Fatal(r, logs)
				}
				meta, err := run(ctx, c.FFprobe, "-v", "error", "-show_entries", "format=format_name:format_tags=major_brand", "-of", "json", p.Items[0].Target)
				if err != nil {
					t.Fatal(err)
				}
				var container struct {
					Format struct {
						Name string `json:"format_name"`
						Tags map[string]string
					}
				}
				if err = json.Unmarshal(meta, &container); err != nil {
					t.Fatal(err)
				}
				if ext == "mov" || ext == "qt" {
					if strings.TrimSpace(container.Format.Tags["major_brand"]) != "qt" {
						t.Fatal("not a real MOV", string(meta))
					}
				} else if ext == "mkv" {
					if !strings.Contains(container.Format.Name, "matroska") {
						t.Fatal(string(meta))
					}
				} else if ext == "ts" {
					if container.Format.Name != "mpegts" {
						t.Fatal(string(meta))
					}
				} else if !strings.Contains(container.Format.Name, "mp4") || strings.TrimSpace(container.Format.Tags["major_brand"]) == "qt" {
					t.Fatal(string(meta))
				}
				if _, err = run(ctx, c.FFmpeg, "-v", "error", "-xerror", "-i", p.Items[0].Target, "-f", "null", "-"); err != nil {
					t.Fatal("cannot decode", err)
				}
				r = Execute(ctx, p, quiet, progress)
				if r.Success != 0 || r.Failed != 1 {
					t.Fatal("overwrite allowed", r)
				}
			}
			after, _ := os.ReadFile(src)
			if string(after) != string(before) {
				t.Fatal("source changed")
			}
		})
	}
	t.Run("container codec fallback", func(t *testing.T) {
		c := base
		c.VideoMode, c.VideoAcceleration = "efficient", "cpu"
		c.CRF = 28
		src := filepath.Join(t.TempDir(), "source.mov")
		target := filepath.Join(t.TempDir(), "result.mov")
		if _, err := run(ctx, c.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=160x96:rate=24", "-t", "1", "-c:v", "libx264", src); err != nil {
			t.Fatal(err)
		}
		logs := []string{}
		s := newVideoSession(c, func(line string) { logs = append(logs, line) })
		s.encode = func(ctx context.Context, exe string, args []string, duration float64, log func(string)) error {
			if strings.Contains(strings.Join(args, " "), "libx265") {
				return errors.New("Could not find tag for codec hevc in stream #0, codec not currently supported in container")
			}
			return runVideoProgress(ctx, exe, args, duration, log)
		}
		if err := s.compress(ctx, src, target); err != nil {
			t.Fatal(err, logs)
		}
		v, err := probeCompressionVideo(ctx, c.FFprobe, target)
		if err != nil || v.Video.Codec != "h264" {
			t.Fatal(v, err)
		}
		if !strings.Contains(strings.Join(logs, "\n"), "封装兼容回退") {
			t.Fatal(logs)
		}
		if s.config.VideoMode != "efficient" {
			t.Fatal("fallback leaked into next file")
		}
	})
}
