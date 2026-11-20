package toolbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type compressionSideData struct {
	Type         string `json:"side_data_type"`
	Rotation     float64
	RedX         string `json:"red_x"`
	RedY         string `json:"red_y"`
	GreenX       string `json:"green_x"`
	GreenY       string `json:"green_y"`
	BlueX        string `json:"blue_x"`
	BlueY        string `json:"blue_y"`
	WhiteX       string `json:"white_point_x"`
	WhiteY       string `json:"white_point_y"`
	MinLuminance string `json:"min_luminance"`
	MaxLuminance string `json:"max_luminance"`
	MaxContent   int    `json:"max_content"`
	MaxAverage   int    `json:"max_average"`
}

func (v compressionVideo) hdr() bool {
	if v.Video.Transfer == "smpte2084" || v.Video.Transfer == "arib-std-b67" {
		return true
	}
	for _, side := range v.Video.SideData {
		if hdrSideData(side.Type) {
			return true
		}
	}
	return false
}
func hdrSideData(kind string) bool {
	kind = strings.ToLower(kind)
	return strings.Contains(kind, "dovi") || strings.Contains(kind, "dolby") || strings.Contains(kind, "mastering display") || strings.Contains(kind, "content light") || strings.Contains(kind, "dynamic") && strings.Contains(kind, "hdr")
}
func (v compressionVideo) dynamicHDR() bool {
	for _, side := range v.Video.SideData {
		t := strings.ToLower(side.Type)
		if strings.Contains(t, "dovi") || strings.Contains(t, "dolby") || strings.Contains(t, "dynamic") && strings.Contains(t, "hdr") {
			return true
		}
	}
	return false
}
func probeHDRFrames(ctx context.Context, exe, path string, v *compressionVideo) error {
	if v.hdrScanned {
		return nil
	}
	out, err := run(ctx, exe, "-v", "error", "-select_streams", "v:0", "-read_intervals", "%+#32", "-show_frames", "-show_entries", "frame=side_data_list", "-of", "json", path)
	if err != nil {
		return fmt.Errorf("HDR 帧信息读取失败：%w", err)
	}
	var info struct {
		Frames []struct {
			SideData []compressionSideData `json:"side_data_list"`
		}
	}
	if err = json.Unmarshal(out, &info); err != nil {
		return err
	}
	if len(info.Frames) == 0 {
		return errors.New("HDR 视频没有可校验的解码帧")
	}
	for _, frame := range info.Frames {
		for _, side := range frame.SideData {
			if !hdrSideData(side.Type) {
				continue
			}
			exists := false
			for _, current := range v.Video.SideData {
				if current.Type == side.Type {
					exists = true
					break
				}
			}
			if !exists {
				v.Video.SideData = append(v.Video.SideData, side)
			}
		}
	}
	v.hdrScanned = true
	return nil
}
func knownVideoColor(value string) bool {
	return value != "" && value != "unknown" && value != "unspecified"
}
func (v compressionVideo) supported() error {
	if v.dynamicHDR() {
		return errors.New("Dolby Vision / HDR10+ 等动态 HDR 暂不转换，保留原视频")
	}
	if !v.hdr() {
		return nil
	}
	bits, _ := strconv.Atoi(v.Video.Bits)
	if bits > 10 || strings.Contains(v.Video.PixelFormat, "12") || strings.Contains(v.Video.PixelFormat, "14") || strings.Contains(v.Video.PixelFormat, "16") {
		return errors.New("高于 10-bit 的 HDR 暂不转换，保留原视频")
	}
	if (v.Video.Transfer != "smpte2084" && v.Video.Transfer != "arib-std-b67") || !knownVideoColor(v.Video.Primaries) || !knownVideoColor(v.Video.Space) {
		return errors.New("HDR 色彩信息不完整，暂保留原视频")
	}
	for _, side := range v.Video.SideData {
		if strings.Contains(strings.ToLower(side.Type), "mastering display") {
			if _, err := hdrMasterDisplay(side); err != nil {
				return err
			}
		}
		if strings.Contains(strings.ToLower(side.Type), "content light") && (side.MaxContent < 0 || side.MaxContent > 65535 || side.MaxAverage < 0 || side.MaxAverage > 65535) {
			return errors.New("HDR 亮度元数据无效，保留原视频")
		}
	}
	return nil
}
func effectiveHDRConfig(c Config, v compressionVideo) (Config, error) {
	if c.VideoHDR == "skip" && v.hdr() {
		return c, errors.New("已选择跳过 HDR，保留原视频")
	}
	checked := v
	if c.VideoHDR == "base" && v.hlgDolbyBase() {
		checked.Video.SideData = nil
	}
	if err := checked.supported(); err != nil {
		if v.hlgDolbyBase() {
			return c, errors.New("含 Dolby Vision 动态信息；如接受移除它，请选择“保留 HLG 基层”")
		}
		return c, err
	}
	if !v.hdr() {
		return c, nil
	}
	mode := c.VideoHDR
	if mode == "" {
		mode = "preserve"
	}
	if mode == "skip" {
		return c, errors.New("已选择跳过 HDR，保留原视频")
	}
	if (mode == "preserve" || mode == "base") && c.VideoMode == "compatible" {
		c.VideoMode = "efficient"
	}
	if mode == "preserve" || mode == "base" {
		for _, side := range v.Video.SideData {
			if strings.Contains(strings.ToLower(side.Type), "mastering display") || strings.Contains(strings.ToLower(side.Type), "content light") {
				c.VideoAcceleration = "cpu"
				break
			}
		}
	}
	return c, nil
}

// HLG signals can be viewed as their compatible base layer. This deliberately
// requires explicit opt-in; it does not attempt to re-create Dolby dynamic data.
func (v compressionVideo) hlgDolbyBase() bool {
	if v.Video.Transfer != "arib-std-b67" || v.Video.Primaries != "bt2020" || v.Video.Space != "bt2020nc" || !v.dynamicHDR() {
		return false
	}
	for _, side := range v.Video.SideData {
		t := strings.ToLower(side.Type)
		if strings.Contains(t, "dynamic") && strings.Contains(t, "hdr") {
			return false
		}
	}
	return true
}
func hdrMasterDisplay(side compressionSideData) (string, error) {
	values := []string{side.GreenX, side.GreenY, side.BlueX, side.BlueY, side.RedX, side.RedY, side.WhiteX, side.WhiteY, side.MaxLuminance, side.MinLuminance}
	numbers := make([]int64, len(values))
	for i, value := range values {
		parts := strings.Split(value, "/")
		if len(parts) != 2 {
			return "", errors.New("HDR 母版信息不完整")
		}
		n, ne := strconv.ParseFloat(parts[0], 64)
		d, de := strconv.ParseFloat(parts[1], 64)
		scale := 50000.0
		limit := 50000.0
		if i >= 8 {
			scale = 10000
			limit = 4294967295
		}
		result := n / d * scale
		if ne != nil || de != nil || n < 0 || d <= 0 || math.IsNaN(result) || math.IsInf(result, 0) || result > limit {
			return "", errors.New("HDR 母版信息无效")
		}
		numbers[i] = int64(math.Round(result))
	}
	return fmt.Sprintf("G(%d,%d)B(%d,%d)R(%d,%d)WP(%d,%d)L(%d,%d)", numbers[0], numbers[1], numbers[2], numbers[3], numbers[4], numbers[5], numbers[6], numbers[7], numbers[8], numbers[9]), nil
}
func hdrX265Parameters(v compressionVideo) string {
	params := ""
	for _, side := range v.Video.SideData {
		t := strings.ToLower(side.Type)
		if strings.Contains(t, "mastering display") {
			if master, err := hdrMasterDisplay(side); err == nil {
				params += ":master-display=" + master
			}
		}
		if strings.Contains(t, "content light") {
			params += fmt.Sprintf(":max-cll=%d,%d", side.MaxContent, side.MaxAverage)
		}
	}
	if v.Video.Transfer == "smpte2084" && v.Video.Primaries == "bt2020" {
		params += ":hdr10-opt=1"
	}
	return params
}
func hdrToneMapFilter() string {
	return "zscale=t=linear:npl=100,format=gbrpf32le,zscale=p=bt709,tonemap=tonemap=mobius:desat=2:peak=0,zscale=t=bt709:m=bt709:r=limited:dither=error_diffusion,format=yuv420p,sidedata=mode=delete:type=MASTERING_DISPLAY_METADATA,sidedata=mode=delete:type=CONTENT_LIGHT_LEVEL"
}
func (s *videoSession) preserve10Bit(v compressionVideo) bool {
	return (v.tenBit() || v.hdr()) && s.config.VideoMode != "compatible" && !(v.hdr() && s.config.VideoHDR == "sdr")
}
func checkHDRFilters(ctx context.Context, exe string) error {
	out, err := run(ctx, exe, "-hide_banner", "-filters")
	if err != nil {
		return err
	}
	for _, name := range []string{"zscale", "tonemap", "sidedata"} {
		if !strings.Contains(string(out), " "+name+" ") {
			return errors.New("HDR 转 SDR 缺少 " + name + " 滤镜，请使用随包完整版 FFmpeg")
		}
	}
	return nil
}
func checkHLGBaseFilters(ctx context.Context, exe string) error {
	out, err := run(ctx, exe, "-hide_banner", "-h", "filter=sidedata")
	if err != nil {
		return err
	}
	if !strings.Contains(string(out), "DOVI_RPU_BUFFER") || !strings.Contains(string(out), "DOVI_METADATA") {
		return errors.New("保留 HLG 基层需要新版 sidedata 滤镜，请使用随包完整版 FFmpeg")
	}
	return nil
}
func hdrPlanNote(c Config, v compressionVideo) string {
	if !v.hdr() {
		return videoPlanNote(c)
	}
	effective, err := effectiveHDRConfig(c, v)
	if err != nil {
		return err.Error()
	}
	if c.VideoHDR == "sdr" {
		return videoPlanNote(effective) + "；HDR → SDR BT.709，亮度与色彩映射"
	}
	note := "保留 HDR · H.265 10-bit · " + v.Video.Transfer + " / " + v.Video.Primaries
	if c.VideoHDR == "base" && v.hlgDolbyBase() {
		note += "；保留 HLG 基层，移除 Dolby Vision 动态信息"
	}
	if effective.VideoAcceleration == "cpu" && c.VideoAcceleration != "cpu" {
		note += "；静态 HDR 元数据使用 CPU 保留"
	}
	return note
}
func validateHDRResult(ctx context.Context, exe, target string, source compressionVideo, out compressionVideo, mode string) error {
	if !source.hdr() {
		return nil
	}
	if err := probeHDRFrames(ctx, exe, target, &out); err != nil {
		return err
	}
	if mode == "sdr" {
		if out.Video.Transfer != "bt709" || out.Video.Primaries != "bt709" || out.Video.Space != "bt709" || out.hdr() {
			return errors.New("HDR 转 SDR 后的色彩信息或元数据不符合预期")
		}
		return nil
	}
	if out.dynamicHDR() {
		return errors.New("输出含有未处理的动态 HDR 信息，停止发布")
	}
	if !out.tenBit() || out.Video.Transfer != source.Video.Transfer || out.Video.Primaries != source.Video.Primaries || out.Video.Space != source.Video.Space || knownVideoColor(source.Video.Range) && out.Video.Range != source.Video.Range {
		return errors.New("HDR 位深、亮度曲线、色域或色彩范围未保留，停止发布")
	}
	for _, side := range source.Video.SideData {
		t := strings.ToLower(side.Type)
		if !strings.Contains(t, "mastering display") && !strings.Contains(t, "content light") {
			continue
		}
		found := false
		for _, other := range out.Video.SideData {
			if side.Type != other.Type {
				continue
			}
			if strings.Contains(t, "mastering display") {
				a, _ := hdrMasterDisplay(side)
				b, _ := hdrMasterDisplay(other)
				found = a == b
			} else {
				found = side.MaxContent == other.MaxContent && side.MaxAverage == other.MaxAverage
			}
			if found {
				break
			}
		}
		if !found {
			return errors.New("HDR 静态元数据未完整保留，停止发布")
		}
	}
	return nil
}
