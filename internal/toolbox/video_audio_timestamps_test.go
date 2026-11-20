package toolbox

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestVideoAudioTimestampErrorClassification(t *testing.T) {
	for _, line := range []string{
		`[aost#0:1/copy @ 0000] Non-monotonic DTS; previous: 3670976, current: 3624951; Error submitting a packet to the muxer: Invalid argument`,
		`[aost#0:2/aac @ 0000] Non-monotonous DTS in output stream 0:2`,
		`[mp4 @ 0000] Non-monotonous DTS in output stream 0:1; previous: 9, current: 7`,
		`[aist#0:0/aac @ 0000] Application provided invalid, non monotonically increasing dts to muxer`,
	} {
		if !videoAudioTimestampError(errors.New(line)) || !videoAudioError(errors.New(line)) {
			t.Fatal("audio DTS failure not recognized", line)
		}
	}
	for _, line := range []string{
		`[vost#0:0/hevc_nvenc @ 0000] Non-monotonic DTS; previous: 9, current: 7`,
		`[mp4 @ 0000] Non-monotonous DTS in output stream 0:0; previous: 9, current: 7`,
		`[out#0/mp4 @ 0000] Error muxing a packet: Invalid argument`,
		`[aost#0:1/copy @ 0000] No space left on device`,
		`Unrecognized option 'fps_mode'. Option not found`,
	} {
		if videoAudioTimestampError(errors.New(line)) || videoAudioError(errors.New(line)) {
			t.Fatal("non-audio failure triggers audio retry", line)
		}
	}
}

func TestVideoAudioRepairPreservesNormalCopyAndOffsets(t *testing.T) {
	v := compressionVideo{Video: compressionStream{TimeBase: "1/90000"}, Audio: []compressionStream{{Codec: "aac", Channels: 2}, {Codec: "aac", Channels: 6}}}
	s := newVideoSession(DefaultConfig(), quiet)
	normal := strings.Join(s.commandArgs("in.mp4", "out.mp4", videoAttempt{"libx265"}, v, false), " ")
	if !strings.Contains(normal, "-c:a:0 copy") || strings.Contains(normal, "aresample") {
		t.Fatal(normal)
	}
	repair := strings.Join(s.commandArgs("in.mp4", "out.mp4", videoAttempt{"libx265"}, v, true, true), " ")
	for _, want := range []string{"-c:a:0 aac", "-c:a:1 aac", "-filter:a:0 aresample=async=1", "-filter:a:1 aresample=async=1", "-enc_time_base:v 1:90000"} {
		if !strings.Contains(repair, want) {
			t.Fatal(want, repair)
		}
	}
	if strings.Contains(repair, "first_pts") || strings.Contains(repair, "asetpts") || strings.Contains(repair, "-c:a:0 copy") {
		t.Fatal("repair resets intentional offsets or still copies", repair)
	}
}

// Make a synthetic fragmented MP4 with an overlapping audio fragment. Unlike
// a mock encoder error, this exercises FFmpeg demuxing, real failed stream copy,
// timeline compensation and output muxing together.
func corruptAudioFragmentForTest(data []byte) bool {
	type atom struct {
		kind    string
		payload []byte
	}
	children := func(data []byte) []atom {
		var out []atom
		for offset := 0; offset+8 <= len(data); {
			size := int(binary.BigEndian.Uint32(data[offset:]))
			if size < 8 || size > len(data)-offset {
				break
			}
			out = append(out, atom{string(data[offset+4 : offset+8]), data[offset+8 : offset+size]})
			offset += size
		}
		return out
	}
	for _, top := range children(data) {
		if top.kind != "moof" {
			continue
		}
		for _, fragment := range children(top.payload) {
			if fragment.kind != "traf" {
				continue
			}
			var track uint32
			for _, part := range children(fragment.payload) {
				if part.kind == "tfhd" && len(part.payload) >= 8 {
					track = binary.BigEndian.Uint32(part.payload[4:8])
				}
				if part.kind != "tfdt" || track != 2 || len(part.payload) < 8 {
					continue
				}
				if part.payload[0] == 1 && len(part.payload) >= 12 {
					base := binary.BigEndian.Uint64(part.payload[4:12])
					if base > 22050 {
						binary.BigEndian.PutUint64(part.payload[4:12], base-22050)
						return true
					}
				} else if part.payload[0] == 0 {
					base := binary.BigEndian.Uint32(part.payload[4:8])
					if base > 22050 {
						binary.BigEndian.PutUint32(part.payload[4:8], base-22050)
						return true
					}
				}
			}
		}
	}
	return false
}

func TestVideoAudioTimestampRepairIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") != "1" {
		t.Skip("requires FFmpeg")
	}
	c := integrationConfig(t)
	c.VideoMode, c.VideoAcceleration, c.CRF = "compatible", "cpu", 23
	ctx := context.Background()
	source := filepath.Join(t.TempDir(), "overlapping-audio.mp4")
	_, err := run(ctx, c.FFmpeg, "-v", "error", "-nostdin", "-n", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100", "-t", "4", "-c:v", "libx264", "-g", "24", "-c:a", "aac", "-movflags", "frag_keyframe+empty_moov", "-frag_duration", "1000000", source)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(source)
	if err != nil || !corruptAudioFragmentForTest(data) {
		t.Fatal("could not create overlapping audio fixture", err)
	}
	if err = os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	before := sha256.Sum256(data)
	logs := []string{}
	s := newVideoSession(c, func(line string) { logs = append(logs, line) })
	target := filepath.Join(t.TempDir(), "repaired.mp4")
	if err = s.compress(ctx, source, target); err != nil {
		t.Fatal(err, logs)
	}
	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, "Non-monotonic DTS") || !strings.Contains(joined, "修复音轨衔接") || strings.Count(joined, "正在压缩视频：") != 2 {
		t.Fatal("real audio retry was not exercised", logs)
	}
	if _, err = run(ctx, c.FFmpeg, "-v", "error", "-nostdin", "-xerror", "-i", target, "-f", "null", "-"); err != nil {
		t.Fatal("output decode failed", err)
	}
	packets, err := run(ctx, c.FFprobe, "-v", "error", "-select_streams", "a:0", "-show_packets", "-show_entries", "packet=dts", "-of", "json", target)
	if err != nil {
		t.Fatal(err)
	}
	var output struct {
		Packets []struct {
			DTS json.Number `json:"dts"`
		}
	}
	if err = json.Unmarshal(packets, &output); err != nil {
		t.Fatal(err)
	}
	var previous int64
	for i, p := range output.Packets {
		current, e := strconv.ParseInt(string(p.DTS), 10, 64)
		if e != nil || i > 0 && current <= previous {
			t.Fatal("non-monotonic output audio", i, p, e)
		}
		previous = current
	}
	after, _ := os.ReadFile(source)
	if sha256.Sum256(after) != before {
		t.Fatal("input changed")
	}
	t.Run("retain audio repair when the video encoder falls back", func(t *testing.T) {
		s.config.VideoAcceleration = "nvidia"
		s.loaded = true
		s.encoders = map[string]bool{"h264_nvenc": true, "libx264": true}
		s.checked["h264_nvenc/false"], s.usable["h264_nvenc/false"] = true, true
		calls := []string{}
		s.encode = func(ctx context.Context, exe string, args []string, duration float64, log func(string)) error {
			command := strings.Join(args, " ")
			calls = append(calls, command)
			if strings.Contains(command, "h264_nvenc") {
				if strings.Contains(command, "-c:a:0 copy") {
					return errors.New("[aost#0:1/copy @ test] Non-monotonic DTS; previous: 99, current: 80")
				}
				return errors.New("simulated unsupported hardware profile")
			}
			return runVideoProgress(ctx, exe, args, duration, log)
		}
		if err = s.compress(ctx, source, filepath.Join(t.TempDir(), "fallback.mp4")); err != nil {
			t.Fatal(err)
		}
		if len(calls) != 3 || !strings.Contains(calls[2], "libx264") || !strings.Contains(calls[2], "-filter:a:0 aresample=async=1") || strings.Contains(calls[2], "-c:a:0 copy") {
			t.Fatal("fallback repeated faulty audio copy", calls)
		}
	})
}
