package toolbox

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func normalizeVideoOptions(c *Config) error {
	if c.VideoContainer == "" {
		c.VideoContainer = "original"
	}
	if c.VideoContainer != "original" && c.VideoContainer != "mp4" {
		return errors.New("请选择有效的视频输出格式")
	}
	if c.VideoHDR == "" {
		c.VideoHDR = "preserve"
	}
	if c.VideoHDR != "preserve" && c.VideoHDR != "sdr" && c.VideoHDR != "skip" && c.VideoHDR != "base" {
		return errors.New("请选择有效的 HDR 处理方式")
	}

	if c.VideoMode == "" {
		c.VideoMode = "compatible"
	}
	if c.VideoAcceleration == "" {
		c.VideoAcceleration = "auto"
	}
	switch c.VideoMode {
	case "compatible", "efficient", "archive":
	default:
		return errors.New("请选择有效的视频压缩模式")
	}
	switch c.VideoAcceleration {
	case "auto", "nvidia", "intel", "amd", "cpu":
	default:
		return errors.New("请选择有效的视频加速方式")
	}
	return nil
}

func videoPlanNote(c Config) string {
	mode := map[string]string{"compatible": "兼容模式 H.264", "efficient": "高效模式 H.265", "archive": "精细模式 H.265 / CPU 慢速"}[c.VideoMode]
	if mode == "" {
		mode = "兼容模式 H.264"
	}
	container := "保留原名称和封装"
	if c.VideoContainer == "mp4" {
		container = "统一 MP4，后缀可能改变"
	}
	return mode + "；" + container + "；不缩放、不降帧，保留原件；硬件失败自动尝试 CPU"
}

type compressionStream struct {
	Index                   int
	Type                    string `json:"codec_type"`
	Codec                   string `json:"codec_name"`
	Width, Height, Channels int
	PixelFormat             string `json:"pix_fmt"`
	TimeBase                string `json:"time_base"`
	Transfer                string `json:"color_transfer"`
	Primaries               string `json:"color_primaries"`
	Space                   string `json:"color_space"`
	Range                   string `json:"color_range"`
	Bits                    string `json:"bits_per_raw_sample"`
	Bitrate                 string `json:"bit_rate"`
	Disposition             struct {
		AttachedPic int `json:"attached_pic"`
	}
	SideData []compressionSideData `json:"side_data_list"`
}

type compressionVideo struct {
	hdrScanned bool
	Video      compressionStream
	Audio      []compressionStream
	Duration   float64
	Bitrate    int64
}

func probeCompressionVideo(ctx context.Context, exe, source string) (compressionVideo, error) {
	var raw struct {
		Streams []compressionStream
		Format  struct {
			Duration string
			Bitrate  string `json:"bit_rate"`
		}
	}
	v := compressionVideo{}
	out, e := run(ctx, exe, "-v", "error", "-show_streams", "-show_format", "-of", "json", source)
	if e != nil {
		return v, e
	}
	if e = json.Unmarshal(out, &raw); e != nil {
		return v, e
	}
	v.Duration, _ = strconv.ParseFloat(raw.Format.Duration, 64)
	v.Bitrate, _ = strconv.ParseInt(raw.Format.Bitrate, 10, 64)
	for _, s := range raw.Streams {
		if s.Type == "video" && s.Disposition.AttachedPic == 0 && v.Video.Width == 0 {
			v.Video = s
			if bitrate, err := strconv.ParseInt(s.Bitrate, 10, 64); err == nil && bitrate > 0 {
				v.Bitrate = bitrate
			}
		}
		if s.Type == "audio" {
			v.Audio = append(v.Audio, s)
		}
	}
	if v.Video.Width <= 0 || v.Video.Height <= 0 {
		return v, errors.New("没有可压缩的视频画面")
	}
	if v.hdr() {
		if err := probeHDRFrames(ctx, exe, source, &v); err != nil {
			return v, err
		}
	}
	return v, nil
}

func (v compressionVideo) tenBit() bool {
	n, _ := strconv.Atoi(v.Video.Bits)
	p := v.Video.PixelFormat
	return n > 8 || strings.Contains(p, "10") || strings.Contains(p, "12") || strings.Contains(p, "16") || strings.HasPrefix(p, "p010")
}

func (v compressionVideo) displaySize() (int, int) {
	w, h := v.Video.Width, v.Video.Height
	for _, side := range v.Video.SideData {
		if int(math.Round(math.Abs(side.Rotation)))%180 == 90 {
			return h, w
		}
	}
	return w, h
}

type videoAttempt struct{ encoder string }

type videoSession struct {
	config          Config
	log             func(string)
	loaded          bool
	encoders        map[string]bool
	checked, usable map[string]bool
	encode          func(context.Context, string, []string, float64, func(string)) error
	cli             *videoCLIOptions
	encoderHelp     map[string]string
}

type videoCLIOptions struct{ fpsMode, statsPeriod bool }

func videoHelpFlag(help, flag string) bool {
	return regexp.MustCompile(`(?m)^\s*-` + regexp.QuoteMeta(flag) + `(?:\s|\[|$)`).MatchString(help)
}

func (s *videoSession) loadCLI(ctx context.Context) error {
	if s.cli != nil {
		return nil
	}
	out, e := run(ctx, s.config.FFmpeg, "-hide_banner", "-h", "full")
	if e != nil {
		return fmt.Errorf("读取 FFmpeg 参数能力失败：%w", e)
	}
	s.cli = &videoCLIOptions{videoHelpFlag(string(out), "fps_mode"), videoHelpFlag(string(out), "stats_period")}
	if !s.cli.fpsMode {
		s.log("检测到旧版 FFmpeg：使用 -vsync 0 保留帧时间戳，自动省略不支持的新参数")
	}
	return nil
}

func (s *videoSession) outputArgs(target string) []string {
	args := []string{}
	if s.cli == nil || s.cli.fpsMode {
		args = append(args, "-fps_mode", "passthrough")
	} else {
		args = append(args, "-vsync", "0")
	}
	muxer := videoMuxer(target)
	if muxer == "mov" || muxer == "mp4" {
		args = append(args, "-movflags", "+faststart")
		if s.config.VideoMode != "compatible" {
			args = append(args, "-tag:v", "hvc1")
		}
	}
	if muxer != "" {
		args = append(args, "-f", muxer)
	}
	args = append(args, "-progress", "pipe:1")
	if s.cli == nil || s.cli.statsPeriod {
		args = append(args, "-stats_period", "1")
	}
	return append(args, target)
}

func videoOptionError(e error) bool {
	if e == nil {
		return false
	}
	t := strings.ToLower(e.Error())
	return strings.Contains(t, "unrecognized option") || strings.Contains(t, "option not found") || strings.Contains(t, "error splitting the argument list")
}

func videoAudioError(e error) bool {
	if e == nil || videoOptionError(e) {
		return false
	}
	t := strings.ToLower(e.Error())
	if videoAudioTimestampError(e) {
		return true
	}
	for _, hint := range []string{"aac", "audio", "could not find tag for codec", "not currently supported in container"} {
		if strings.Contains(t, hint) {
			return true
		}
	}
	return false
}

var videoLegacyAudioOutput = regexp.MustCompile(`output stream 0:[1-9][0-9]*(?:[^0-9]|$)`)

// The output maps one video stream first, followed only by audio streams.
// New FFmpeg identifies audio outputs as aost; older muxers report 0:1, etc.
// A generic DTS/muxing failure alone must not be mistaken for an audio error.
func videoAudioTimestampError(e error) bool {
	if e == nil || videoOptionError(e) {
		return false
	}
	for _, line := range strings.Split(strings.ToLower(e.Error()), "\n") {
		nonMonotonic := strings.Contains(line, "non-monotonic dts") || strings.Contains(line, "non-monotonous dts") || strings.Contains(line, "non monotonically increasing dts")
		if nonMonotonic && (strings.Contains(line, "[aost#") || strings.Contains(line, "[aist#") || videoLegacyAudioOutput.MatchString(line)) {
			return true
		}
	}
	return false
}

func newVideoSession(c Config, log func(string)) *videoSession {
	return &videoSession{config: c, log: log, checked: map[string]bool{}, usable: map[string]bool{}, encode: runVideoProgress}
}

func (s *videoSession) attempts() []videoAttempt {
	c := s.config
	codec := "h264"
	software := "libx264"
	if c.VideoMode != "compatible" {
		codec, software = "hevc", "libx265"
	}
	a := []videoAttempt{}
	// CPU decoding accepts more source formats than GPU decoding. Only encoding
	// is offloaded; no -hwaccel dependency is imposed on each input codec/profile.
	if c.VideoMode != "archive" && c.CRF != 0 {
		for _, device := range []struct{ name, suffix string }{{"nvidia", "nvenc"}, {"intel", "qsv"}, {"amd", "amf"}} {
			if c.VideoAcceleration == "auto" || c.VideoAcceleration == device.name {
				a = append(a, videoAttempt{codec + "_" + device.suffix})
			}
		}
	}
	return append(a, videoAttempt{software})
}

func (s *videoSession) quality(encoder string) int {
	q := s.config.CRF
	if q == 0 {
		return 0
	}
	if s.config.VideoMode != "compatible" {
		q += 2
	}
	if strings.HasSuffix(encoder, "_nvenc") {
		q += 3
	}
	if q > 51 {
		q = 51
	}
	return q
}

func (s *videoSession) encoderArgs(a videoAttempt, v compressionVideo) []string {
	q := s.quality(a.encoder)
	toneSDR := v.hdr() && s.config.VideoHDR == "sdr"
	colors := v.Video
	if toneSDR {
		colors.Transfer = "bt709"
		colors.Primaries = "bt709"
		colors.Space = "bt709"
		colors.Range = "tv"
	}

	args := []string{"-c:v", a.encoder}
	if s.config.Workers > 1 && strings.HasPrefix(a.encoder, "lib") {
		args = append(args, "-threads", strconv.Itoa(max(1, runtime.NumCPU()/s.config.Workers)))
	}
	switch {
	case strings.HasSuffix(a.encoder, "_nvenc"):
		help, known := s.encoderHelp[a.encoder]
		preset := "p6"
		if known && !regexp.MustCompile(`(?m)^\s+p6\s`).MatchString(help) {
			preset = "slow"
		}
		args = append(args, "-preset", preset)
		if !known || videoHelpFlag(help, "tune") {
			args = append(args, "-tune", "hq")
		}
		args = append(args, "-rc", "vbr", "-cq", strconv.Itoa(q), "-b:v", "0")
		if !known || videoHelpFlag(help, "rc-lookahead") {
			args = append(args, "-rc-lookahead", "20")
		}
		if !known || videoHelpFlag(help, "spatial-aq") {
			args = append(args, "-spatial-aq", "1")
		} else if videoHelpFlag(help, "spatial_aq") {
			args = append(args, "-spatial_aq", "1")
		}
	case strings.HasSuffix(a.encoder, "_qsv"):
		args = append(args, "-preset", "slow", "-global_quality", strconv.Itoa(q), "-b:v", "0")
	case strings.HasSuffix(a.encoder, "_amf"):
		args = append(args, "-quality", "quality", "-rc", "cqp", "-qp_i", strconv.Itoa(max(0, q-2)), "-qp_p", strconv.Itoa(q), "-qp_b", strconv.Itoa(min(51, q+2)))
	default:
		preset := "medium"
		if s.config.VideoMode == "archive" {
			preset = "slow"
		}
		args = append(args, "-preset", preset, "-crf", strconv.Itoa(q))
		if a.encoder == "libx265" {
			params := "log-level=error"
			if s.config.Workers > 1 {
				threads := max(1, runtime.NumCPU()/s.config.Workers)
				params += fmt.Sprintf(":pools=%d:frame-threads=%d", threads, min(threads, 4))
			}
			for _, color := range []struct{ key, value string }{{"colorprim", colors.Primaries}, {"transfer", colors.Transfer}, {"colormatrix", colors.Space}} {
				if color.value != "" && color.value != "unknown" && color.value != "unspecified" {
					params += ":" + color.key + "=" + color.value
				}
			}
			if v.hdr() && !toneSDR {
				params += hdrX265Parameters(v)
			}
			if toneSDR {
				params += ":cll=0"
			}
			args = append(args, "-x265-params", params)
		}
	}
	pixel := "yuv420p"
	if s.preserve10Bit(v) {
		pixel = "yuv420p10le"
		if !strings.HasPrefix(a.encoder, "lib") {
			pixel = "p010le"
		}
	}
	// Pad by at most one pixel, rather than resize odd-sized sources.
	filter := "pad=ceil(iw/2)*2:ceil(ih/2)*2"
	if toneSDR {
		filter = hdrToneMapFilter() + "," + filter
	}
	if s.config.VideoHDR == "base" && v.hlgDolbyBase() {
		filter += ",sidedata=mode=delete:type=DOVI_RPU_BUFFER,sidedata=mode=delete:type=DOVI_METADATA"
	}
	args = append(args, "-vf", filter, "-pix_fmt", pixel)
	for _, color := range []struct{ key, value string }{{"color_trc", colors.Transfer}, {"color_primaries", colors.Primaries}, {"colorspace", colors.Space}, {"color_range", colors.Range}} {
		if color.value != "" && color.value != "unknown" && color.value != "unspecified" {
			args = append(args, "-"+color.key, color.value)
		}
	}
	return args
}

func videoAudioArgs(v compressionVideo, forceAAC bool, repairTimeline ...bool) []string {
	args := []string{}
	for i, stream := range v.Audio {
		index := strconv.Itoa(i)
		codec := "copy"
		if stream.Codec != "aac" || forceAAC {
			codec = "aac"
		}
		args = append(args, "-c:a:"+index, codec)
		if codec == "aac" {
			rate := "192k"
			if stream.Channels > 2 {
				rate = "384k"
			}
			args = append(args, "-b:a:"+index, rate)
			if len(repairTimeline) > 0 && repairTimeline[0] {
				// Fill/trim discontinuities using the original audio timestamps.
				// Do not reset first_pts: intentional delayed tracks keep their offset.
				args = append(args, "-filter:a:"+index, "aresample=async=1")
			}
		}
	}
	return args
}

func (s *videoSession) available(ctx context.Context, a videoAttempt, v compressionVideo) bool {
	if !s.loaded {
		s.loaded = true
		s.encoders = map[string]bool{}
		out, e := run(ctx, s.config.FFmpeg, "-hide_banner", "-encoders")
		if e == nil {
			for _, line := range strings.Split(string(out), "\n") {
				fields := strings.Fields(line)
				if len(fields) > 1 && strings.HasPrefix(fields[0], "V") {
					s.encoders[fields[1]] = true
				}
			}
		}
	}
	if !s.encoders[a.encoder] {
		return false
	}
	if strings.HasPrefix(a.encoder, "lib") {
		return true
	}
	if s.encoderHelp == nil {
		s.encoderHelp = map[string]string{}
	}
	if _, ok := s.encoderHelp[a.encoder]; !ok {
		out, e := run(ctx, s.config.FFmpeg, "-hide_banner", "-h", "encoder="+a.encoder)
		if e != nil {
			s.log("读取编码器参数失败：" + a.encoder + " · " + CleanText(e.Error()))
			return false
		}
		s.encoderHelp[a.encoder] = string(out)
	}
	key := fmt.Sprintf("%s/%t", a.encoder, s.preserve10Bit(v))
	if !s.checked[key] {
		probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=30", "-frames:v", "8", "-an"}
		// Device capability is independent of one source's color metadata.
		probeVideo := compressionVideo{Video: compressionStream{PixelFormat: "yuv420p"}}
		if s.preserve10Bit(v) {
			probeVideo.Video.PixelFormat = "yuv420p10le"
		}
		args = append(args, s.encoderArgs(a, probeVideo)...)
		args = append(args, "-f", "null", "-")
		_, e := run(probeCtx, s.config.FFmpeg, args...)
		s.checked[key], s.usable[key] = true, e == nil
		if e != nil && ctx.Err() == nil {
			s.log("加速不可用：" + a.encoder + " · " + CleanText(e.Error()) + "；将尝试其他编码方式")
		}
	}
	return s.usable[key]
}

func (s *videoSession) compress(ctx context.Context, source, target string) error {
	if e := normalizeVideoOptions(&s.config); e != nil {
		return e
	}
	if videoMuxer(target) == "" {
		return fmt.Errorf("输出封装不支持，保留原视频；请选择统一 MP4")
	}
	v, e := probeCompressionVideo(ctx, s.config.FFprobe, source)
	if e != nil {
		return e
	}
	original := s.config
	defer func() { s.config = original }()
	s.config, e = effectiveHDRConfig(s.config, v)
	if e != nil {
		return e
	}
	if v.hdr() {
		s.log(hdrPlanNote(original, v))
		if s.config.VideoHDR == "base" && v.hlgDolbyBase() {
			if e = checkHLGBaseFilters(ctx, s.config.FFmpeg); e != nil {
				return e
			}
		}
		if s.config.VideoHDR == "sdr" {
			if e = checkHDRFilters(ctx, s.config.FFmpeg); e != nil {
				return e
			}
		}
	}

	if e = s.loadCLI(ctx); e != nil {
		return e
	}
	if v.tenBit() && !v.hdr() && s.config.VideoMode == "compatible" {
		s.log("兼容模式：10-bit SDR 转为 8-bit H.264；需要保留位深时请选择高效或精细模式")
	}
	var lastError error
	// 0: copy eligible audio, 1: AAC, 2: AAC with timestamp compensation.
	// Retain a diagnosed audio fix if a later video encoder fallback is needed.
	audioMode := 0
	for _, a := range s.attempts() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !s.available(ctx, a, v) {
			continue
		}
		for audioTry := audioMode; audioTry < 3; audioTry++ {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if e = os.Remove(target); e != nil && !os.IsNotExist(e) {
				return e
			}
			label := a.encoder
			if strings.HasPrefix(a.encoder, "lib") {
				label += "（CPU）"
			}
			s.log("正在压缩视频：" + source + " · " + label)
			args := s.commandArgs(source, target, a, v, audioTry >= 1, audioTry == 2)
			e = s.encode(ctx, s.config.FFmpeg, args, v.Duration, s.log)
			if e == nil {
				e = validateCompressedVideo(ctx, s.config.FFprobe, target, v, s.config.VideoMode, s.config.VideoHDR)
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if e == nil {
				s.log("视频编码完成：" + label)
				return nil
			}
			lastError = e
			s.log("当前编码失败：" + label + " · " + CleanText(e.Error()))
			if videoOptionError(e) {
				return fmt.Errorf("FFmpeg 参数不兼容，请使用随包 FFmpeg 或升级所选程序：%w", e)
			}
			if videoContainerCodecError(e) && s.config.VideoMode != "compatible" {
				if v.tenBit() || v.hdr() {
					return fmt.Errorf("当前封装不支持 H.265；为保留位深和 HDR 不自动降级，原件保留：%w", e)
				}
				if s.config.VideoMode == "archive" {
					s.config.VideoAcceleration = "cpu"
				}
				s.config.VideoMode = "compatible"
				s.log("封装兼容回退：保持输出名称与后缀，改用 H.264；显卡不可用时回退 CPU")
				return s.compress(ctx, source, target)
			}
			copying := false
			for _, stream := range v.Audio {
				if stream.Codec == "aac" {
					copying = true
				}
			}
			if audioTry < 2 && len(v.Audio) > 0 && videoAudioTimestampError(e) {
				audioMode = 2
				audioTry = 1 // advance directly to the timeline-repair attempt
				s.log("正在重试：音轨时间戳不连续，保留当前视频编码器，重编码 AAC 并修复音轨衔接（异常处可能补静音或裁去重叠音频）")
				continue
			}
			if audioTry == 0 && copying && videoAudioError(e) {
				audioMode = 1
				s.log("正在重试：将音频转换为 AAC，排除直接复制音轨的兼容问题")
				continue
			}
			break
		}
		s.log("正在回退：保持所选输出格式，尝试其他设备或 CPU 编码")
	}
	if lastError == nil {
		lastError = errors.New("FFmpeg 没有可用的所选编码器，请使用配套完整版中的 FFmpeg")
	}
	return fmt.Errorf("所有编码方式均未成功，保留原视频：%w", lastError)
}

func (s *videoSession) commandArgs(source, target string, a videoAttempt, v compressionVideo, forceAAC bool, repairTimeline ...bool) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-nostats", "-xerror", "-n", "-i", source, "-map", "0:" + strconv.Itoa(v.Video.Index), "-map", "0:a?", "-map_metadata", "0", "-map_chapters", "0"}
	args = append(args, s.encoderArgs(a, v)...)
	args = append(args, videoAudioArgs(v, forceAAC, repairTimeline...)...)
	// Passthrough alone still rounds frames to the encoder's default time base.
	// Preserve source precision so variable-rate frames do not acquire equal DTS.
	if base := videoEncoderTimeBase(v.Video.TimeBase); base != "" {
		args = append(args, "-enc_time_base:v", base)
	}
	return append(args, s.outputArgs(target)...)
}

func videoEncoderTimeBase(value string) string {
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return ""
	}
	n, ne := strconv.ParseInt(parts[0], 10, 32)
	d, de := strconv.ParseInt(parts[1], 10, 32)
	if ne != nil || de != nil || n <= 0 || d <= 0 {
		return ""
	}
	return strconv.FormatInt(n, 10) + ":" + strconv.FormatInt(d, 10)
}

func validateCompressedVideo(ctx context.Context, exe, target string, source compressionVideo, mode string, hdrModes ...string) error {
	out, e := probeCompressionVideo(ctx, exe, target)
	if e != nil {
		return fmt.Errorf("输出验证失败：%w", e)
	}
	codec := "hevc"
	if mode == "compatible" {
		codec = "h264"
	}
	w, h := source.displaySize()
	w, h = (w+1)/2*2, (h+1)/2*2
	if out.Video.Codec != codec || out.Video.Width != w || out.Video.Height != h || len(out.Audio) != len(source.Audio) {
		return errors.New("输出编码、分辨率或音轨数量不符合预期")
	}
	hdrMode := "preserve"
	if len(hdrModes) > 0 {
		hdrMode = hdrModes[0]
	}
	if source.tenBit() && mode != "compatible" && !(source.hdr() && hdrMode == "sdr") && !out.tenBit() {
		return errors.New("输出未保留 10-bit 位深")
	}
	if source.Duration > 0 && (out.Duration <= 0 || math.Abs(out.Duration-source.Duration) > math.Max(1, source.Duration*0.005)) {
		return errors.New("输出时长不符合预期，可能不完整")
	}
	if err := validateHDRResult(ctx, exe, target, source, out, hdrMode); err != nil {
		return err
	}
	return nil
}

// Cap stderr memory even if a broken input produces a large number of errors.
type videoErrorBuffer struct{ bytes.Buffer }

func (b *videoErrorBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if b.Len() < 8192 {
		_, _ = b.Buffer.Write(p[:min(len(p), 8192-b.Len())])
	}
	return n, nil
}

func runVideoProgress(ctx context.Context, exe string, args []string, duration float64, log func(string)) error {
	cmd := exec.CommandContext(ctx, exe, args...)
	hideWindow(cmd)
	var stderr videoErrorBuffer
	cmd.Stderr = &stderr
	pipe, e := cmd.StdoutPipe()
	if e != nil {
		return e
	}
	if e = cmd.Start(); e != nil {
		return e
	}
	scanner := bufio.NewScanner(pipe)
	last := time.Now().Add(-10 * time.Second)
	seconds, speed := float64(0), ""
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if !ok {
			continue
		}
		switch key {
		case "out_time_us":
			us, _ := strconv.ParseFloat(value, 64)
			seconds = us / 1e6
		case "speed":
			speed = strings.TrimSpace(value)
		case "progress":
			if duration > 0 && seconds >= 0 {
				reportOperationProgress(ctx, OperationProgress{Tool: "video", Phase: "encoding", Percent: math.Min(100, math.Max(0, seconds/duration*100)), PercentKnown: true})
			}
			if time.Since(last) >= 10*time.Second && duration > 0 && seconds >= 0 {
				percent := math.Min(99.9, 100*seconds/duration)
				message := fmt.Sprintf("视频进度：%.1f%% · 速度 %s", percent, speed)
				if x, err := strconv.ParseFloat(strings.TrimSuffix(speed, "x"), 64); err == nil && x > 0 && seconds < duration {
					message += fmt.Sprintf(" · 预计剩余 %s", (time.Duration((duration-seconds)/x) * time.Second).Round(time.Second))
				}

				log(message)
				last = time.Now()
			}
		}
	}
	scanError := scanner.Err()
	e = cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if e != nil {
		return fmt.Errorf("%w：%s", e, strings.TrimSpace(stderr.String()))
	}
	return scanError
}
