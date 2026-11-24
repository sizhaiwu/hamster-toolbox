package toolbox

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
)

type VideoCommand struct{ Label, PowerShell string }

func powerShellLiteral(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }

func videoPowerShell(exe, target string, args []string) string {
	words := []string{"&", powerShellLiteral(exe)}
	for _, arg := range args {
		words = append(words, powerShellLiteral(arg))
	}
	return "[System.IO.Directory]::CreateDirectory(" + powerShellLiteral(filepath.Dir(target)) + ") | Out-Null\n" + strings.Join(words, " ")
}
func platformVideoCommand(exe, target string, args []string) string {
	if runtime.GOOS != "windows" {
		return "mkdir -p -- " + posixQuote(filepath.Dir(target)) + " &&\n" + posixRun(exe, args)
	}
	return videoPowerShell(exe, target, args)
}

// Build only on demand, and use the same argument builder as actual execution.
// Commands target the visible output path, instead of the toolbox's temporary file.
func VideoCommands(ctx context.Context, c Config, it Item) ([]VideoCommand, error) {
	if c.Tool != "video" || it.Conflict != "" {
		return nil, errors.New("该项不能生成视频压缩命令，请先处理预览中的冲突")
	}
	if err := normalizeVideoOptions(&c); err != nil {
		return nil, err
	}
	if videoMuxer(it.Target) == "" {
		return nil, errors.New("此输出封装不支持，请选择统一 MP4 并重新扫描")
	}
	v, err := probeCompressionVideo(ctx, c.FFprobe, it.Source)
	if err != nil {
		return nil, err
	}
	if c, err = effectiveHDRConfig(c, v); err != nil {
		return nil, err
	}
	if c.VideoHDR == "base" && v.hlgDolbyBase() {
		if err = checkHLGBaseFilters(ctx, c.FFmpeg); err != nil {
			return nil, err
		}
	}
	if v.hdr() && c.VideoHDR == "sdr" {
		if err = checkHDRFilters(ctx, c.FFmpeg); err != nil {
			return nil, err
		}
	}
	s := newVideoSession(c, func(string) {})
	if err = s.loadCLI(ctx); err != nil {
		return nil, err
	}
	commands := []VideoCommand{}
	copyAudio := false
	for _, audio := range v.Audio {
		if audio.Codec == "aac" {
			copyAudio = true
		}
	}
	for _, a := range s.attempts() {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !s.available(ctx, a, v) {
			continue
		}
		label := a.encoder
		if len(commands) > 0 {
			label += "（备用）"
		}
		commands = append(commands, VideoCommand{label, platformVideoCommand(c.FFmpeg, it.Target, s.commandArgs(it.Source, it.Target, a, v, false))})
		if copyAudio {
			commands = append(commands, VideoCommand{a.encoder + "（音频重编码备用）", platformVideoCommand(c.FFmpeg, it.Target, s.commandArgs(it.Source, it.Target, a, v, true))})
		}
		if len(v.Audio) > 0 {
			commands = append(commands, VideoCommand{a.encoder + "（音轨时间戳修复备用）", platformVideoCommand(c.FFmpeg, it.Target, s.commandArgs(it.Source, it.Target, a, v, true, true))})
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if c.VideoMode != "compatible" && !v.tenBit() && !v.hdr() {
		backup := *s
		backup.config.VideoMode = "compatible"
		a := videoAttempt{"libx264"}
		if backup.available(ctx, a, v) {
			commands = append(commands, VideoCommand{"封装不支持 H.265 时的 H.264 / CPU 兼容备用", platformVideoCommand(c.FFmpeg, it.Target, backup.commandArgs(it.Source, it.Target, a, v, true))})
		}
	}
	if len(commands) == 0 {
		return nil, errors.New("没有可用编码器，请检查 FFmpeg 和显卡驱动")
	}
	return commands, nil
}
