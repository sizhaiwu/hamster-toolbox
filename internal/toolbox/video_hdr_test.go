package toolbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHDRPolicyAndMetadata(t *testing.T) {
	v := compressionVideo{Video: compressionStream{Transfer: "arib-std-b67", Primaries: "bt2020", Space: "bt2020nc", PixelFormat: "yuv420p10le"}}
	c := DefaultConfig()
	c.VideoMode, c.VideoAcceleration = "compatible", "intel"
	effective, err := effectiveHDRConfig(c, v)
	if err != nil || effective.VideoMode != "efficient" || effective.VideoAcceleration != "intel" {
		t.Fatal(effective, err)
	}
	c.VideoHDR = "sdr"
	effective, err = effectiveHDRConfig(c, v)
	if err != nil || effective.VideoMode != "compatible" {
		t.Fatal(effective, err)
	}
	c.VideoHDR = "skip"
	if _, err = effectiveHDRConfig(c, v); err == nil {
		t.Fatal("skip ignored")
	}
	c.VideoHDR = "preserve"
	v.Video.SideData = []compressionSideData{{Type: "Content light level metadata", MaxContent: 1000, MaxAverage: 400}}
	effective, err = effectiveHDRConfig(c, v)
	if err != nil || effective.VideoAcceleration != "cpu" {
		t.Fatal(effective, err)
	}
	v.Video.SideData = []compressionSideData{{Type: "HDR Dynamic Metadata SMPTE2094-40 (HDR10+)"}}
	if _, err = effectiveHDRConfig(c, v); err == nil {
		t.Fatal("dynamic metadata would be lost")
	}
	v.Video.SideData = []compressionSideData{{Type: "Dolby Vision RPU Data"}}
	if _, err = effectiveHDRConfig(c, v); err == nil {
		t.Fatal("Dolby information silently removed")
	}
	c.VideoHDR = "base"
	if effective, err = effectiveHDRConfig(c, v); err != nil || effective.VideoMode != "efficient" {
		t.Fatal(effective, err)
	}
	v.Video.Transfer = "smpte2084"
	if _, err = effectiveHDRConfig(c, v); err == nil {
		t.Fatal("unverified Dolby base accepted")
	}
	v.Video.Transfer = "arib-std-b67"
	v.Video.SideData = nil
	v.Video.PixelFormat = "yuv420p12le"
	if v.supported() == nil {
		t.Fatal("12-bit HDR silently reduced")
	}
	if _, err = hdrMasterDisplay(compressionSideData{GreenX: "0/0"}); err == nil {
		t.Fatal("invalid mastering display accepted")
	}
}

// Real encodes check the resulting bitstream, not only command text. Synthetic
// fixtures contain no private media and are removed by the test runner.
func TestHDRCompressionIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") != "1" {
		t.Skip("set TOOLBOX_INTEGRATION=1")
	}
	ctx := context.Background()
	c := integrationConfig(t)
	c.VideoAcceleration, c.VideoMode, c.VideoHDR, c.CRF = "cpu", "efficient", "preserve", 23
	for _, transfer := range []string{"arib-std-b67", "smpte2084"} {
		t.Run(transfer, func(t *testing.T) {
			source := filepath.Join(t.TempDir(), "hdr.mkv")
			params := "log-level=error:cll=0:colorprim=bt2020:transfer=" + transfer + ":colormatrix=bt2020nc"
			if transfer == "smpte2084" {
				params += ":master-display=G(13250,34500)B(7500,3000)R(34000,16000)WP(15635,16450)L(10000000,1):max-cll=1000,400"
			}
			if _, err := run(ctx, c.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24", "-t", "2", "-vf", "format=yuv420p10le,setparams=color_primaries=bt2020:color_trc="+transfer+":colorspace=bt2020nc:range=limited", "-c:v", "libx265", "-preset", "ultrafast", "-crf", "0", "-x265-params", params, "-pix_fmt", "yuv420p10le", "-color_trc", transfer, "-color_primaries", "bt2020", "-colorspace", "bt2020nc", "-color_range", "tv", source); err != nil {
				t.Fatal(err)
			}
			v, err := probeCompressionVideo(ctx, c.FFprobe, source)
			if err != nil || !v.hdr() {
				t.Fatal(v, err)
			}
			if transfer == "smpte2084" && !strings.Contains(hdrX265Parameters(v), "master-display=") {
				t.Fatal("frame SEI was not read", v.Video.SideData)
			}
			for _, policy := range []string{"preserve", "sdr"} {
				for _, mode := range []string{"compatible", "efficient"} {
					t.Run(policy+"/"+mode, func(t *testing.T) {
						active := c
						active.VideoHDR, active.VideoMode = policy, mode
						out := filepath.Join(t.TempDir(), "result.mp4")
						if err := newVideoSession(active, func(line string) { t.Log(line) }).compress(ctx, source, out); err != nil {
							t.Fatal(err)
						}
						effective, _ := effectiveHDRConfig(active, v)
						if err := validateCompressedVideo(ctx, active.FFprobe, out, v, effective.VideoMode, policy); err != nil {
							t.Fatal(err)
						}
						if _, err := run(ctx, active.FFmpeg, "-v", "error", "-xerror", "-i", out, "-f", "null", "-"); err != nil {
							t.Fatal("result decode", err)
						}
					})
				}
			}
			if transfer == "arib-std-b67" && os.Getenv("TOOLBOX_HARDWARE_INTEGRATION") == "1" {
				for _, device := range []string{"nvidia", "intel"} {
					t.Run(device, func(t *testing.T) {
						active := c
						active.VideoAcceleration = device
						logs := []string{}
						out := filepath.Join(t.TempDir(), "gpu.mp4")
						if err := newVideoSession(active, func(line string) { logs = append(logs, line) }).compress(ctx, source, out); err != nil {
							t.Fatal(err, logs)
						}
						want := "hevc_nvenc"
						if device == "intel" {
							want = "hevc_qsv"
						}
						if !strings.Contains(strings.Join(logs, "\n"), "视频编码完成："+want) {
							t.Fatal("hardware not used", logs)
						}
					})
				}
			}
		})
	}
}
