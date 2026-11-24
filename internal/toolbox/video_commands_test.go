package toolbox

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPowerShellCommandLiterals(t *testing.T) {
	if powerShellLiteral("O'Brien $(1+1) & [x]") != "'O''Brien $(1+1) & [x]'" {
		t.Fatal("unsafe quoting")
	}
	c := videoPowerShell("ffmpeg.exe", "out.mp4", []string{"-i", "a'b $(1+1).mp4", "-n", "out.mp4"})
	if !strings.Contains(c, "& 'ffmpeg.exe' '-i' 'a''b $(1+1).mp4' '-n' 'out.mp4'") {
		t.Fatal(c)
	}
	if _, err := VideoCommands(context.Background(), Config{Tool: "rename"}, Item{}); err == nil {
		t.Fatal("non-video command allowed")
	}
	if _, err := VideoCommands(context.Background(), Config{Tool: "video"}, Item{Conflict: "skip"}); err == nil {
		t.Fatal("conflicting command allowed")
	}
}

func TestCopiedVideoCommandIntegration(t *testing.T) {
	if runtime.GOOS != "windows" || os.Getenv("TOOLBOX_INTEGRATION") != "1" {
		t.Skip("requires Windows and TOOLBOX_INTEGRATION=1")
	}
	ctx := context.Background()
	c := integrationConfig(t)
	c.Tool, c.VideoMode, c.VideoAcceleration = "video", "efficient", "cpu"
	source := filepath.Join(t.TempDir(), "样片 $(1+1) ' [x]&.mp4")
	if _, err := run(ctx, c.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24", "-t", "1", "-c:v", "libx264", source); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "新目录 $(1+1) ' [x]&", "output.mp4")
	commands, err := VideoCommands(ctx, c, Item{Source: source, Target: target})
	if err != nil || len(commands) < 1 {
		t.Fatal(commands, err)
	}
	script := filepath.Join(t.TempDir(), "command.ps1")
	if err = os.WriteFile(script, append([]byte{0xef, 0xbb, 0xbf}, []byte(commands[0].PowerShell+"\nexit $LASTEXITCODE\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = run(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	v, err := probeCompressionVideo(ctx, c.FFprobe, target)
	if err != nil || v.Video.Codec != "hevc" || v.Video.Width != 320 || v.Video.Height != 180 {
		t.Fatal(v, err)
	}
	beforeStat, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	// Some FFmpeg builds return success when -n skips an existing file. Verify
	// the actual protection rather than relying on its exit-code convention.
	_, _ = run(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script)
	after, err := os.ReadFile(target)
	if err != nil || string(before) != string(after) {
		t.Fatal("existing output changed", err)
	}
	afterStat, err := os.Stat(target)
	if err != nil || !beforeStat.ModTime().Equal(afterStat.ModTime()) {
		t.Fatal("existing output was rewritten", err)
	}
}
