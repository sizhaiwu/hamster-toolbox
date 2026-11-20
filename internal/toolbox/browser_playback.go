package toolbox

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"strconv"
)

type BrowserMediaInfo struct {
	Duration                                       float64
	Container, VideoCodec, AudioCodec, PixelFormat string
	Width, Height, AudioChannels                   int
	Fragmented                                     bool
}

func (p BrowserMediaInfo) CopyVideo() bool {
	return p.VideoCodec == "h264" && (p.PixelFormat == "yuv420p" || p.PixelFormat == "yuvj420p")
}

// A regular inherited stdin remains seekable through FFmpeg's fd protocol.
// No pathname or external protocol is available to the decoder.
func ProbeBrowserMedia(ctx context.Context, program string, input *os.File) (BrowserMediaInfo, error) {
	var result BrowserMediaInfo
	if _, err := input.Seek(0, 0); err != nil {
		return result, err
	}
	args := []string{"-v", "error", "-max_alloc", "268435456", "-protocol_whitelist", "fd,pipe", "-format_whitelist", browserMediaFormats, "-show_entries", "format=duration,format_name:stream=codec_type,codec_name,pix_fmt,width,height,channels", "-of", "json", "fd:"}
	cmd := exec.CommandContext(ctx, program, args...)
	hideWindow(cmd)
	cmd.Stdin = input
	out, stderr := &thumbnailBuffer{limit: 128 << 10}, &thumbnailBuffer{limit: 4096}
	cmd.Stdout, cmd.Stderr = out, stderr
	if err := cmd.Run(); err != nil {
		if browserFDUnavailable(fmt.Errorf("%s", stderr.String())) {
			return result, errors.New("当前 FFprobe 不支持安全预览，请升级 FFmpeg 组件")
		}
		return result, fmt.Errorf("无法读取媒体信息：%w", err)
	}
	var probe struct {
		Streams []struct {
			CodecType               string `json:"codec_type"`
			CodecName               string `json:"codec_name"`
			PixelFormat             string `json:"pix_fmt"`
			Width, Height, Channels int
		}
		Format struct {
			Duration string
			Name     string `json:"format_name"`
		}
	}
	if err := json.Unmarshal(out.Bytes(), &probe); err != nil {
		return result, errors.New("媒体信息无效")
	}
	for _, stream := range probe.Streams {
		if stream.CodecType == "video" && result.VideoCodec == "" {
			result.VideoCodec = stream.CodecName
			result.PixelFormat = stream.PixelFormat
			result.Width = stream.Width
			result.Height = stream.Height
		}
		if stream.CodecType == "audio" && result.AudioCodec == "" {
			result.AudioCodec = stream.CodecName
			result.AudioChannels = stream.Channels
		}
	}
	result.Container = probe.Format.Name
	result.Duration, _ = strconv.ParseFloat(probe.Format.Duration, 64)
	if math.IsNaN(result.Duration) || math.IsInf(result.Duration, 0) || result.Duration < 0 {
		result.Duration = 0
	}
	result.Fragmented = BrowserFragmentedMP4(input)
	if result.VideoCodec == "" && result.AudioCodec == "" {
		return result, errors.New("没有可播放的音视频轨道")
	}
	if result.Width < 0 || result.Height < 0 || result.Width > 16384 || result.Height > 16384 || int64(result.Width)*int64(result.Height) > 67_108_864 {
		return result, errors.New("视频尺寸过大，建议下载原文件查看")
	}
	return result, nil
}

func BrowserFragmentedMP4(input *os.File) bool {
	st, err := input.Stat()
	if err != nil {
		return false
	}
	var offset int64
	for i := 0; i < 64 && offset < 4<<20; i++ {
		var head [16]byte
		if _, err := input.ReadAt(head[:8], offset); err != nil {
			return false
		}
		size := int64(binary.BigEndian.Uint32(head[:4]))
		kind := string(head[4:8])
		if size == 1 {
			if _, err := input.ReadAt(head[8:], offset+8); err != nil {
				return false
			}
			wide := binary.BigEndian.Uint64(head[8:])
			if wide > uint64(st.Size()) {
				return false
			}
			size = int64(wide)
		}
		if size < 8 || size > st.Size()-offset {
			return false
		}
		if kind == "moof" {
			return true
		}
		if kind == "mdat" {
			return false
		}
		offset += size
	}
	return false
}

func StreamBrowserMedia(ctx context.Context, program string, input *os.File, info BrowserMediaInfo, start float64, forceConvert bool, output io.Writer) error {
	if start < 0 || math.IsNaN(start) || math.IsInf(start, 0) {
		return errors.New("播放时间无效")
	}
	if _, err := input.Seek(0, 0); err != nil {
		return err
	}
	args := browserSafeInputArgs()
	// Allow a short startup buffer, then read near playback speed instead of
	// decoding the entire file at maximum speed while other media needs CPU/I/O.
	args = append(args, "-readrate", "1.15", "-readrate_initial_burst", "3")
	if start > 0 {
		args = append(args, "-ss", strconv.FormatFloat(start, 'f', 3, 64))
	}
	args = append(args, "-i", "fd:", "-map", "0:v:0?", "-map", "0:a:0?", "-sn", "-dn", "-map_metadata", "-1", "-map_chapters", "-1")
	if info.CopyVideo() && !forceConvert {
		args = append(args, "-c:v", "copy")
	} else {
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-pix_fmt", "yuv420p", "-vf", "scale=w='min(1280,iw)':h='min(720,ih)':force_original_aspect_ratio=decrease:force_divisible_by=2", "-threads", "2", "-g", "60")
	}
	if info.AudioCodec == "aac" && info.AudioChannels <= 2 && !forceConvert {
		args = append(args, "-c:a", "copy", "-bsf:a", "aac_adtstoasc")
	} else {
		args = append(args, "-c:a", "aac", "-b:a", "128k", "-ac", "2")
	}
	args = append(args, "-avoid_negative_ts", "make_zero", "-movflags", "frag_keyframe+empty_moov+default_base_moof", "-frag_duration", "1000000", "-f", "mp4", "pipe:1")
	cmd := exec.CommandContext(ctx, program, args...)
	hideWindow(cmd)
	cmd.Stdin = input
	cmd.Stdout = output
	stderr := &thumbnailBuffer{limit: 4096}
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if browserFDUnavailable(fmt.Errorf("%s", stderr.String())) {
			return errors.New("当前 FFmpeg 不支持兼容播放，请升级组件")
		}
		return fmt.Errorf("兼容播放失败：%w %s", err, stderr.String())
	}
	return nil
}
