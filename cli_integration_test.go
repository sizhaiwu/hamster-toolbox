package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

// Opt in with HAMSTER_CLI_TEST_EXE: this tests the real Windows executable,
// bundled dependencies and PowerShell, using only newly created test data.
func TestCLIExecutableIntegration(t *testing.T) {
	exe := os.Getenv("HAMSTER_CLI_TEST_EXE")
	if exe == "" {
		t.Skip("set HAMSTER_CLI_TEST_EXE to a built executable with bundled tools")
	}
	exe, err := filepath.Abs(exe)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	base := filepath.Dir(exe)
	c := toolbox.DefaultConfig()
	c.FFmpeg = filepath.Join(base, "tools", "ffmpeg", "bin", "ffmpeg.exe")
	c.FFprobe = filepath.Join(base, "tools", "ffmpeg", "bin", "ffprobe.exe")
	c.Magick = filepath.Join(base, "tools", "imagemagick", "magick.exe")
	c.Exiftool = filepath.Join(base, "tools", "exiftool", "exiftool.exe")
	c.SevenZip = filepath.Join(base, "tools", "sevenzip", "7z.exe")
	c.Font = filepath.Join(base, "tools", "fonts", "NotoSansCJKsc-Regular.otf")
	c.Recursive, c.MinMB, c.MinKbps = false, 0, 0
	c.VideoAcceleration = "cpu"
	const password = "IntegrationTestOnly!"
	t.Setenv("TOOLBOX_ARCHIVE_PASSWORD", password)
	t.Setenv("TOOLBOX_EXTRACT_PASSWORD", password)
	run := func(t *testing.T, program string, stdin []byte, args ...string) []byte {
		t.Helper()
		cmd := exec.CommandContext(ctx, program, args...)
		cmd.Stdin = bytes.NewReader(stdin)
		var out, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &stderr
		if e := cmd.Run(); e != nil {
			t.Fatalf("%s failed: %v\n%s\n%s", filepath.Base(program), e, out.String(), stderr.String())
		}
		if strings.Contains(out.String()+stderr.String(), password) {
			t.Fatal("test password leaked into command output")
		}
		return out.Bytes()
	}
	root := t.TempDir()
	video := filepath.Join(root, "fixture.mp4")
	run(t, c.FFmpeg, nil, "-v", "error", "-nostdin", "-f", "lavfi", "-i", "testsrc=size=320x180:rate=12", "-t", "2", "-c:v", "libx264", "-crf", "1", "-pix_fmt", "yuv420p", video)
	img := image.NewNRGBA(image.Rect(0, 0, 320, 180))
	for y := 0; y < 180; y++ {
		for x := 0; x < 320; x++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(x), uint8(y), 80, 200})
		}
	}
	write := func(t *testing.T, path string, data []byte) {
		t.Helper()
		if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(path, data, 0600); e != nil {
			t.Fatal(e)
		}
	}
	writeImage := func(t *testing.T, path string) {
		t.Helper()
		var b bytes.Buffer
		var e error
		if filepath.Ext(path) == ".png" {
			e = (&png.Encoder{CompressionLevel: png.NoCompression}).Encode(&b, img)
		} else {
			e = jpeg.Encode(&b, img, &jpeg.Options{Quality: 100})
		}
		if e != nil {
			t.Fatal(e)
		}
		write(t, path, b.Bytes())
	}
	fixture := func(t *testing.T, tool string) toolbox.Config {
		t.Helper()
		active := c
		active.Tool, active.Root = tool, filepath.Join(t.TempDir(), "中文 ' [100%] 输入")
		if e := os.MkdirAll(active.Root, 0755); e != nil {
			t.Fatal(e)
		}
		switch tool {
		case "format", "pdf":
			writeImage(t, filepath.Join(active.Root, "示例相册", "cover.jpg"))
		case "rename":
			active.Prefix, active.Dirs = "new-", false
			write(t, filepath.Join(active.Root, "原文件.txt"), []byte("rename fixture"))
		case "number":
			active.NumberStyle, active.NumberName = "number", ""
			active.StartNumber, active.NumberKeepOriginal = 29, false
			if e := os.Mkdir(filepath.Join(active.Root, "原目录"), 0755); e != nil {
				t.Fatal(e)
			}
		case "describe", "video", "thumbnail":
			data, e := os.ReadFile(video)
			if e != nil {
				t.Fatal(e)
			}
			write(t, filepath.Join(active.Root, "中文视频 [100%].mp4"), data)
			active.CRF = 28
			active.ThumbnailColumns, active.ThumbnailRows, active.ThumbnailWidth = 2, 2, 480
		case "image":
			active.ImageFormats, active.PngMode = []string{"png"}, "webp-png"
			writeImage(t, filepath.Join(active.Root, "透明图片.png"))
		case "exif":
			src := filepath.Join(active.Root, "metadata.jpg")
			writeImage(t, src)
			run(t, active.Exiftool, nil, "-overwrite_original", "-Artist=CLIIntegrationFixture", src)
		case "archive":
			active.ArchivePassword, active.ArchiveSplit = password, "none"
			write(t, filepath.Join(active.Root, "中文文件.txt"), []byte(strings.Repeat("archive fixture\n", 1000)))
		case "extract":
			active.ExtractPassword = password
			input := filepath.Join(filepath.Dir(active.Root), "fixture.txt")
			write(t, input, []byte("exact extraction content"))
			run(t, active.SevenZip, nil, "a", "-t7z", "-p"+password, "-mhe=on", filepath.Join(active.Root, "中文压缩包.7z"), input)
		}
		return active
	}
	snapshot := func(t *testing.T, root string) map[string]string {
		t.Helper()
		r := map[string]string{}
		e := filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.IsDir() {
				r[path] = "directory"
				return nil
			}
			data, e := os.ReadFile(path)
			r[path] = fmt.Sprintf("%x", sha256.Sum256(data))
			return e
		})
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	powershell := func(t *testing.T, code string) []byte {
		t.Helper()
		script := filepath.Join(t.TempDir(), "command.ps1")
		code = "$ErrorActionPreference='Stop'\n[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false)\n$LASTEXITCODE=0\n" + code + "\nexit $LASTEXITCODE\n"
		write(t, script, append([]byte{0xef, 0xbb, 0xbf}, []byte(code)...))
		return run(t, "powershell.exe", nil, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script)
	}
	check := func(t *testing.T, p toolbox.Plan) {
		t.Helper()
		it := p.Items[0]
		target := it.Target
		if p.Config.Tool == "pdf" {
			target = p.Config.Output
		}
		if _, e := os.Stat(target); e != nil {
			t.Fatal("expected output missing", e)
		}
		switch p.Config.Tool {
		case "format", "rename", "number", "describe":
			if _, e := os.Stat(it.Source); !os.IsNotExist(e) {
				t.Fatal("source was not renamed", e)
			}
		case "image":
			out := run(t, c.Magick, nil, "identify", "-format", "%m %wx%h", target)
			if string(out) != "WEBP 320x180" {
				t.Fatal("image format/dimensions", string(out))
			}
		case "video":
			a, _ := os.Stat(it.Source)
			b, _ := os.Stat(target)
			if b.Size() >= a.Size() {
				t.Fatal("video did not shrink")
			}
			run(t, c.FFmpeg, nil, "-v", "error", "-nostdin", "-i", target, "-f", "null", "-")
		case "thumbnail":
			out := run(t, c.Magick, nil, "identify", "-format", "%w", target)
			if string(out) != "480" {
				t.Fatal("thumbnail width", string(out))
			}
		case "exif":
			out := run(t, c.Exiftool, nil, "-j", "-Artist", target)
			if strings.Contains(string(out), "CLIIntegrationFixture") {
				t.Fatal("metadata not removed")
			}
		case "pdf":
			data, e := os.ReadFile(target)
			if e != nil || !bytes.HasPrefix(data, []byte("%PDF-")) {
				t.Fatal("invalid PDF", e)
			}
		case "archive":
			run(t, c.SevenZip, nil, "t", "-p"+password, target)
		case "extract":
			data, e := os.ReadFile(filepath.Join(target, "fixture.txt"))
			if e != nil || string(data) != "exact extraction content" {
				t.Fatal("extracted contents differ", e)
			}
		}
	}
	for _, tool := range []string{"format", "rename", "number", "describe", "exif", "pdf", "image", "video", "thumbnail", "archive", "extract"} {
		for _, mode := range []string{"direct", "copied-powershell"} {
			t.Run(tool+"/"+mode, func(t *testing.T) {
				active := fixture(t, tool)
				commands, e := buildCLICommands(active, exe)
				if e != nil {
					t.Fatal(e)
				}
				config := active
				config.ArchivePassword, config.ExtractPassword = "", ""
				data, _ := json.Marshal(config)
				args := []string{"cli", tool, "--config", "-"}
				if tool == "archive" || tool == "extract" {
					args = append(args, "--password-env", "TOOLBOX_"+strings.ToUpper(tool)+"_PASSWORD")
				}
				before := snapshot(t, filepath.Dir(active.Root))
				var out []byte
				if mode == "direct" {
					out = run(t, exe, data, args...)
				} else {
					out = powershell(t, commands.Preview)
				}
				if !reflect.DeepEqual(before, snapshot(t, filepath.Dir(active.Root))) {
					t.Fatal("preview changed files or directories")
				}
				var p toolbox.Plan
				if e := json.Unmarshal(out, &p); e != nil || len(p.Items) != 1 || p.Items[0].Conflict != "" {
					t.Fatalf("invalid preview: %v %s", e, out)
				}
				if mode == "direct" {
					out = run(t, exe, data, append(args, "--execute")...)
				} else {
					out = powershell(t, commands.Execute)
				}
				var result toolbox.Result
				if e := json.Unmarshal(out, &result); e != nil || result.Success != 1 || result.Failed != 0 {
					t.Fatalf("execution result: %v %s", e, out)
				}
				check(t, p)
			})
		}
	}
	for _, tool := range []string{"image", "exif", "archive", "extract", "describe", "video"} {
		t.Run("native/"+tool, func(t *testing.T) {
			active := fixture(t, tool)
			p, e := toolbox.BuildPlan(ctx, active, func(string) {})
			if e != nil || len(p.Items) != 1 {
				t.Fatal("native command plan", e, p)
			}
			commands, e := toolbox.NativeCommands(ctx, p.Config, p.Items[0])
			if e != nil || len(commands) == 0 {
				t.Fatal("native command export", e)
			}
			before := snapshot(t, filepath.Dir(active.Root))
			out := powershell(t, commands[0].PowerShell)
			if tool == "describe" {
				if !json.Valid(out) || !reflect.DeepEqual(before, snapshot(t, filepath.Dir(active.Root))) {
					t.Fatal("FFprobe command should return JSON without renaming")
				}
			} else {
				check(t, *p)
			}
		})
	}
	t.Run("metadata-video-custom-program-flags", func(t *testing.T) {
		input := filepath.Join(t.TempDir(), "中文视频输入")
		if e := os.MkdirAll(input, 0755); e != nil {
			t.Fatal(e)
		}
		run(t, c.FFmpeg, nil, "-v", "error", "-nostdin", "-i", video, "-map", "0", "-c", "copy", "-metadata", "title=CLIIntegrationFixture", filepath.Join(input, "metadata.mkv"))
		args := []string{"cli", "exif", "--root", input, "--metadata-types", "videos", "--ffmpeg", c.FFmpeg, "--ffprobe", c.FFprobe, "--exiftool", c.Exiftool}
		out := run(t, exe, nil, args...)
		var p toolbox.Plan
		if e := json.Unmarshal(out, &p); e != nil || len(p.Items) != 1 || p.Items[0].Conflict != "" {
			t.Fatalf("video metadata preview: %v %s", e, out)
		}
		out = run(t, exe, nil, append(args, "--execute")...)
		var result toolbox.Result
		if e := json.Unmarshal(out, &result); e != nil || result.Success != 1 || result.Failed != 0 {
			t.Fatalf("video metadata execution: %v %s", e, out)
		}
		out = run(t, c.FFprobe, nil, "-v", "error", "-show_entries", "format_tags", "-of", "json", p.Items[0].Target)
		if strings.Contains(string(out), "CLIIntegrationFixture") {
			t.Fatal("video title metadata not removed")
		}
		run(t, c.FFmpeg, nil, "-v", "error", "-nostdin", "-i", p.Items[0].Target, "-f", "null", "-")
	})
}
