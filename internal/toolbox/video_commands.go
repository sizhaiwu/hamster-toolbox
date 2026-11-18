package toolbox

import (
	"context"
	"errors"
	"path/filepath"
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

// Build only on demand, and use the same argument builder as actual execution.
// Commands target the visible output path, instead of the toolbox's temporary file.
func VideoCommands(ctx context.Context, c Config, it Item) ([]VideoCommand, error) {
	if c.Tool != "video" || it.Conflict != "" {
		return nil, errors.New("该项不能生成视频压缩命令，请先处理预览中的冲突")
	}
	if err := normalizeVideoOptions(&c); err != nil {
		return nil, err
	}
	v, err := probeCompressionVideo(ctx, c.FFprobe, it.Source)
	if err != nil {
		return nil, err
	}
	if err = v.supported(); err != nil {
		return nil, err
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
		commands = append(commands, VideoCommand{label, videoPowerShell(c.FFmpeg, it.Target, s.commandArgs(it.Source, it.Target, a, v, false))})
		if copyAudio {
			commands = append(commands, VideoCommand{a.encoder + "（音频重编码备用）", videoPowerShell(c.FFmpeg, it.Target, s.commandArgs(it.Source, it.Target, a, v, true))})
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if len(commands) == 0 {
		return nil, errors.New("没有可用编码器，请检查 FFmpeg 和显卡驱动")
	}
	return commands, nil
}
