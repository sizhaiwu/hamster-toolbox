package toolbox

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestVideoModesAndSourceProtection(t *testing.T) {
	c := Config{CRF: 23}
	if e := normalizeVideoOptions(&c); e != nil || c.VideoMode != "compatible" || c.VideoAcceleration != "auto" {
		t.Fatal(c, e)
	}
	c.VideoMode = "invalid"
	if normalizeVideoOptions(&c) == nil {
		t.Fatal("invalid mode accepted")
	}
	c.VideoMode, c.VideoAcceleration = "efficient", "unknown"
	if normalizeVideoOptions(&c) == nil {
		t.Fatal("invalid accelerator accepted")
	}
	for _, transfer := range []string{"smpte2084", "arib-std-b67"} {
		v := compressionVideo{Video: compressionStream{Transfer: transfer}}
		if v.supported() == nil {
			t.Fatal("HDR not protected", transfer)
		}
	}
	if (compressionVideo{Video: compressionStream{Transfer: "bt709"}}).supported() != nil {
		t.Fatal("SDR rejected")
	}
	for _, pixel := range []string{"yuv420p10le", "yuv422p10le", "p010le", "yuv444p12le"} {
		if !(compressionVideo{Video: compressionStream{PixelFormat: pixel}}).tenBit() {
			t.Fatal(pixel)
		}
	}
}

func TestVideoFallbackOrderAndZeroQualityCPU(t *testing.T) {
	c := Config{VideoMode: "efficient", VideoAcceleration: "auto", CRF: 18}
	s := newVideoSession(c, quiet)
	if !reflect.DeepEqual(s.attempts(), []videoAttempt{{"hevc_nvenc"}, {"hevc_qsv"}, {"hevc_amf"}, {"libx265"}}) {
		t.Fatal(s.attempts())
	}
	s.config.VideoAcceleration = "nvidia"
	if !reflect.DeepEqual(s.attempts(), []videoAttempt{{"hevc_nvenc"}, {"libx265"}}) {
		t.Fatal(s.attempts())
	}
	s.config.VideoMode = "archive"
	if !reflect.DeepEqual(s.attempts(), []videoAttempt{{"libx265"}}) {
		t.Fatal(s.attempts())
	}
	s.config.VideoMode, s.config.CRF = "compatible", 0
	if !reflect.DeepEqual(s.attempts(), []videoAttempt{{"libx264"}}) {
		t.Fatal(s.attempts())
	}
	v := compressionVideo{Video: compressionStream{PixelFormat: "yuv420p10le"}}
	for _, encoder := range []string{"libx265", "hevc_nvenc", "hevc_qsv", "hevc_amf"} {
		s.config = c
		args := strings.Join(s.encoderArgs(videoAttempt{encoder}, v), " ")
		if !strings.Contains(args, "-tag:v hvc1") || (!strings.Contains(args, "p010le") && !strings.Contains(args, "yuv420p10le")) {
			t.Fatal(args)
		}
		if strings.Contains(args, "-r ") || strings.Contains(args, "scale=") {
			t.Fatal("resizes or changes frame rate", args)
		}
	}
	args := videoAudioArgs(compressionVideo{Audio: []compressionStream{{Codec: "aac", Channels: 2}, {Codec: "ac3", Channels: 6}}}, false)
	if strings.Join(args, " ") != "-c:a:0 copy -c:a:1 aac -b:a:1 384k" {
		t.Fatal(args)
	}
	if !strings.Contains(strings.Join(videoAudioArgs(compressionVideo{Audio: []compressionStream{{Codec: "aac", Channels: 2}}}, true), " "), "-c:a:0 aac") {
		t.Fatal("AAC retry still copies")
	}
}

// Exercise real encode, mux, validation and safe publication using generated
// media. Hardware checks are opt-in because CI has no physical GPU requirement.
func TestVideoCompressionIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") != "1" {
		t.Skip("set TOOLBOX_INTEGRATION=1")
	}
	c := integrationConfig(t)
	c.Tool, c.Root, c.MinMB, c.MinKbps = "video", t.TempDir(), 0, 0
	source := filepath.Join(c.Root, "中文 样片.mkv")
	_, e := run(context.Background(), c.FFmpeg, "-v", "error", "-nostdin", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "2", "-c:v", "libx264", "-crf", "0", "-pix_fmt", "yuv420p", "-c:a", "pcm_s16le", source)
	if e != nil {
		t.Fatal(e)
	}
	for _, mode := range []string{"compatible", "efficient", "archive"} {
		t.Run(mode, func(t *testing.T) {
			active := c
			active.VideoMode, active.VideoAcceleration, active.CRF = mode, "cpu", 23
			active.Output = filepath.Join(c.Root, "out-"+mode)
			p, e := BuildPlan(context.Background(), active, quiet)
			if e != nil || len(p.Items) != 1 || p.Items[0].Conflict != "" {
				t.Fatal(p, e)
			}
			logs := []string{}
			r := Execute(context.Background(), p, func(line string) { logs = append(logs, line) }, progress)
			if r.Success != 1 || r.Failed != 0 {
				t.Fatal(r, logs)
			}
			out, e := probeCompressionVideo(context.Background(), active.FFprobe, p.Items[0].Target)
			if e != nil || out.Duration < 1.9 || len(out.Audio) != 1 || out.Audio[0].Codec != "aac" {
				t.Fatal(out, e)
			}
			if strings.Contains(strings.Join(logs, "\n"), "_nvenc") {
				t.Fatal("CPU-only attempted GPU")
			}
		})
		// Keep the next scan limited to the source, as an arbitrary user-chosen
		// sibling output directory isn't a globally excluded directory name.
		c.Recursive = false
	}
	t.Run("failed hardware and AAC mux retry", func(t *testing.T) {
		aacSource := filepath.Join(t.TempDir(), "source.mp4")
		if _, e := run(context.Background(), c.FFmpeg, "-v", "error", "-i", source, "-c:v", "copy", "-c:a", "aac", aacSource); e != nil {
			t.Fatal(e)
		}
		active := c
		active.VideoMode, active.VideoAcceleration = "efficient", "nvidia"
		logs := []string{}
		s := newVideoSession(active, func(line string) { logs = append(logs, line) })
		s.loaded, s.encoders = true, map[string]bool{"hevc_nvenc": true, "libx265": true}
		s.checked["hevc_nvenc/false"], s.usable["hevc_nvenc/false"] = true, true
		encoders := []string{}
		s.encode = func(ctx context.Context, exe string, args []string, d float64, log func(string)) error {
			joined := strings.Join(args, " ")
			if strings.Contains(joined, "hevc_nvenc") {
				encoders = append(encoders, "hardware")
				// A failed encoder may leave a partial file; the next attempt
				// must remove it and produce a valid, complete CPU result.
				if e := os.WriteFile(args[len(args)-1], []byte("partial"), 0600); e != nil {
					t.Fatal(e)
				}
				return errors.New("simulated unsupported hardware profile")
			}
			encoders = append(encoders, "cpu")
			return runVideoProgress(ctx, exe, args, d, log)
		}
		out := filepath.Join(t.TempDir(), "result.mp4")
		if e := s.compress(context.Background(), aacSource, out); e != nil {
			t.Fatal(e, logs)
		}
		if !reflect.DeepEqual(encoders, []string{"hardware", "cpu"}) {
			t.Fatal(encoders)
		}
		if !strings.Contains(strings.Join(logs, " "), "正在回退") {
			t.Fatal(logs)
		}
		s.config.VideoAcceleration = "cpu"
		audioCalls := 0
		s.encode = func(ctx context.Context, exe string, args []string, d float64, log func(string)) error {
			audioCalls++
			if strings.Contains(strings.Join(args, " "), "-c:a:0 copy") {
				return errors.New("AAC audio cannot be muxed")
			}
			return runVideoProgress(ctx, exe, args, d, log)
		}
		if e := s.compress(context.Background(), aacSource, filepath.Join(t.TempDir(), "audio-retry.mp4")); e != nil || audioCalls != 2 {
			t.Fatal(e, audioCalls)
		}
		optionCalls := 0
		s.encode = func(context.Context, string, []string, float64, func(string)) error {
			optionCalls++
			return errors.New("Unrecognized option 'fps_mode'. Option not found")
		}
		if e := s.compress(context.Background(), aacSource, filepath.Join(t.TempDir(), "option-error.mp4")); e == nil || optionCalls != 1 {
			t.Fatal(e, optionCalls)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if !errors.Is(s.compress(ctx, aacSource, out), context.Canceled) {
			t.Fatal("cancellation not propagated")
		}
	})
	t.Run("10-bit and hardware", func(t *testing.T) {
		src10 := filepath.Join(t.TempDir(), "10bit.mkv")
		if _, e := run(context.Background(), c.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24", "-t", "1", "-c:v", "libx265", "-x265-params", "log-level=error", "-pix_fmt", "yuv420p10le", src10); e != nil {
			t.Fatal(e)
		}
		devices := []string{"cpu"}
		if os.Getenv("TOOLBOX_HARDWARE_INTEGRATION") == "1" {
			devices = append(devices, "nvidia", "intel")
		}
		for _, device := range devices {
			active := c
			active.VideoMode, active.VideoAcceleration = "efficient", device
			out := filepath.Join(t.TempDir(), "10bit.mp4")
			logs := []string{}
			if e := newVideoSession(active, func(line string) { logs = append(logs, line) }).compress(context.Background(), src10, out); e != nil {
				t.Fatal(device, e, logs)
			}
			result, e := probeCompressionVideo(context.Background(), c.FFprobe, out)
			if e != nil || !result.tenBit() {
				t.Fatal(device, result, e)
			}
			if device != "cpu" && !strings.Contains(strings.Join(logs, " "), map[string]string{"nvidia": "视频编码完成：hevc_nvenc", "intel": "视频编码完成：hevc_qsv"}[device]) {
				t.Fatal("hardware unexpectedly fell back", device, logs)
			}
			t.Log(device, logs)
		}
	})
	t.Run("variable frame rate rotation and AAC copy", func(t *testing.T) {
		vfr := filepath.Join(t.TempDir(), "vfr.mp4")
		if _, e := run(context.Background(), c.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "2", "-vf", "setpts=if(lt(N\\,24)\\,N/(24*TB)\\,(24/(24*TB)+(N-24)/(12*TB)))", "-fps_mode", "vfr", "-c:v", "libx264", "-crf", "1", "-c:a", "aac", vfr); e != nil {
			t.Fatal(e)
		}
		rotated := filepath.Join(t.TempDir(), "rotated.mp4")
		if _, e := run(context.Background(), c.FFmpeg, "-v", "error", "-i", vfr, "-c", "copy", "-metadata:s:v:0", "rotate=90", rotated); e != nil {
			t.Fatal(e)
		}
		active := c
		active.VideoMode, active.VideoAcceleration = "efficient", "cpu"
		out := filepath.Join(t.TempDir(), "output.mp4")
		if e := newVideoSession(active, quiet).compress(context.Background(), rotated, out); e != nil {
			t.Fatal(e)
		}
		for _, stream := range []string{"V:0", "a:0"} {
			var rows [][]byte
			for _, path := range []string{rotated, out} {
				entries := "frame=pts_time"
				if stream == "a:0" {
					entries = "packet=data_hash"
				}
				args := []string{"-v", "error", "-select_streams", stream, "-show_entries", entries, "-show_data_hash", "sha256", "-of", "json", path}
				b, e := run(context.Background(), c.FFprobe, args...)
				if e != nil {
					t.Fatal(e)
				}
				rows = append(rows, b)
			}
			if stream == "a:0" {
				if string(rows[0]) != string(rows[1]) {
					t.Fatal("AAC packet payload changed during stream copy")
				}
			} else {
				var before, after struct {
					Frames []struct {
						Time string `json:"pts_time"`
					}
				}
				if e := json.Unmarshal(rows[0], &before); e != nil {
					t.Fatal(e)
				}
				if e := json.Unmarshal(rows[1], &after); e != nil {
					t.Fatal(e)
				}
				if len(before.Frames) == 0 || !reflect.DeepEqual(before.Frames, after.Frames) {
					t.Fatal("VFR timestamps or frame count changed", before, after)
				}
			}
		}
	})
	t.Run("odd dimensions and 444 input", func(t *testing.T) {
		odd := filepath.Join(t.TempDir(), "odd.mkv")
		if _, e := run(context.Background(), c.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc=size=321x181:rate=12", "-t", "1", "-c:v", "ffv1", "-pix_fmt", "yuv444p", odd); e != nil {
			t.Fatal(e)
		}
		active := c
		active.VideoMode, active.VideoAcceleration = "compatible", "cpu"
		if e := newVideoSession(active, quiet).compress(context.Background(), odd, filepath.Join(t.TempDir(), "odd.mp4")); e != nil {
			t.Fatal(e)
		}
	})
	t.Run("HDR is previewed as skipped", func(t *testing.T) {
		root := t.TempDir()
		if _, e := run(context.Background(), c.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24", "-t", "1", "-c:v", "libx265", "-x265-params", "log-level=error:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc", "-pix_fmt", "yuv420p10le", "-color_trc", "smpte2084", "-color_primaries", "bt2020", filepath.Join(root, "hdr.mkv")); e != nil {
			t.Fatal(e)
		}
		active := c
		active.Root, active.Output = root, ""
		p, e := BuildPlan(context.Background(), active, quiet)
		if e != nil || len(p.Items) != 1 || !strings.Contains(p.Items[0].Conflict, "HDR") {
			t.Fatal(p, e)
		}
	})
}

func TestLegacyVideoCLIOptions(t *testing.T) {
	s := newVideoSession(Config{VideoMode: "efficient", CRF: 18}, quiet)
	s.cli = &videoCLIOptions{}
	args := strings.Join(s.outputArgs("out.mp4"), " ")
	if strings.Contains(args, "fps_mode") || strings.Contains(args, "stats_period") || !strings.Contains(args, "-vsync 0") {
		t.Fatal(args)
	}
	s.encoderHelp = map[string]string{"hevc_nvenc": "  -preset int\n     slow 1\n  -rc-lookahead int\n  -spatial_aq boolean\n"}
	args = strings.Join(s.encoderArgs(videoAttempt{"hevc_nvenc"}, compressionVideo{}), " ")
	if strings.Contains(args, "p6") || strings.Contains(args, "-tune") || !strings.Contains(args, "-preset slow") || !strings.Contains(args, "-spatial_aq 1") {
		t.Fatal(args)
	}
	if !videoOptionError(errors.New("Unrecognized option 'fps_mode'. Option not found")) || videoAudioError(errors.New("Unrecognized option 'fps_mode'.")) || videoAudioError(errors.New("unsupported hardware profile")) || !videoAudioError(errors.New("AAC audio not supported in container")) {
		t.Fatal("incorrect retry classification")
	}
}

func TestLegacyVideoCompressionIntegration(t *testing.T) {
	legacy := os.Getenv("TOOLBOX_LEGACY_FFMPEG")
	if legacy == "" {
		t.Skip("set TOOLBOX_LEGACY_FFMPEG and TOOLBOX_LEGACY_FFPROBE to test an older installed build")
	}
	c := integrationConfig(t)
	source := filepath.Join(t.TempDir(), "source.mp4")
	if _, e := run(context.Background(), c.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24", "-t", "1", "-c:v", "libx264", "-crf", "1", source); e != nil {
		t.Fatal(e)
	}
	c.FFmpeg, c.FFprobe = legacy, os.Getenv("TOOLBOX_LEGACY_FFPROBE")
	c.VideoAcceleration = "cpu"
	for _, mode := range []string{"compatible", "efficient", "archive"} {
		c.VideoMode = mode
		logs := []string{}
		if e := newVideoSession(c, func(line string) { logs = append(logs, line) }).compress(context.Background(), source, filepath.Join(t.TempDir(), "out.mp4")); e != nil {
			t.Fatal(mode, e, logs)
		}
		if !strings.Contains(strings.Join(logs, " "), "旧版 FFmpeg") {
			t.Fatal("legacy options not detected", logs)
		}
	}
}
