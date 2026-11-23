package toolbox

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Native commands are an advanced escape hatch, not the transactional executor.
func NativeCommands(ctx context.Context, c Config, it Item) ([]VideoCommand, error) {
	if it.Conflict != "" {
		return nil, errors.New("请先处理预览中的冲突")
	}
	if c.Tool == "video" {
		return VideoCommands(ctx, c, it)
	}
	if c.Tool == "extract" {
		if _, format := compressedTAR(it.Source); format != "" {
			return nil, errors.New("复合 TAR 使用两步流式解压，请复制页面顶部的工具箱 CLI 命令；普通 PowerShell 二进制管道可能损坏数据")
		}
	}
	if e := validateItemPaths(c, it); e != nil {
		return nil, e
	}
	if c.Tool == "exif" && !has(it.Source, metadataVideos) {
		ext, err := metadataImageExtension(it.Source)
		if err != nil {
			return nil, err
		}
		if !strings.EqualFold(ext, filepath.Ext(it.Source)) || !strings.EqualFold(ext, filepath.Ext(it.Target)) {
			return nil, errors.New("图片实际格式需要临时副本处理，请复制页面顶部的工具箱 CLI 命令")
		}
	}
	if runtime.GOOS != "windows" {
		return nativePOSIXCommands(c, it)
	}
	quote := powerShellLiteral
	run := func(exe string, args []string) string {
		parts := []string{"&", quote(exe)}
		for _, a := range args {
			parts = append(parts, quote(a))
		}
		return strings.Join(parts, " ")
	}
	guard := "if (Test-Path -LiteralPath " + quote(it.Target) + ") { throw '目标已存在，请换输出路径' }\n[System.IO.Directory]::CreateDirectory(" + quote(filepath.Dir(it.Target)) + ") | Out-Null\n"
	var command, label string
	switch c.Tool {
	case "image":
		command = guard + run(c.Magick, imageCompressionArgs(c, it.Source, it.Target))
		label = "ImageMagick · 单张压缩"
	case "describe":
		command = run(c.FFprobe, []string{"-v", "error", "-show_entries", "format=duration,size:stream=width,height", "-of", "json", it.Source})
		label = "FFprobe · 读取媒体信息（不改名）"
	case "exif":
		if has(it.Source, metadataVideos) && !has(it.Source, metadataExifVideos) {
			return nil, errors.New("此容器需要多步轨道校验，请使用页面顶部的工具箱 CLI 命令")
		}
		command = guard + run(c.Exiftool, []string{"-charset", "filename=UTF8", "-all=", "-o", it.Target, it.Source})
		label = "ExifTool · 清理副本元数据"
	case "archive":
		if c.ArchiveMode == "extras" || c.ArchiveMarker {
			return nil, errors.New("附加文件与打包标记需要临时准备，请使用页面顶部的完整 CLI 命令")
		}
		args := archiveArgs(c, it.Target, false)
		args[len(args)-1] = filepath.Base(it.Source)
		for i, a := range args {
			if strings.HasPrefix(a, "-p") {
				args = append(args[:i], args[i+1:]...)
				break
			}
		}
		command = run(c.SevenZip, args)
		if c.ArchivePassword != "" {
			for i, a := range args {
				if a == "--" {
					command = run(c.SevenZip, args[:i]) + " ('-p'+$env:TOOLBOX_ARCHIVE_PASSWORD)"
					for _, tail := range args[i:] {
						command += " " + quote(tail)
					}
					break
				}
			}
			command = "if ([string]::IsNullOrEmpty($env:TOOLBOX_ARCHIVE_PASSWORD)) { throw '请先设置 TOOLBOX_ARCHIVE_PASSWORD' }\n" + command
		}
		command = guard + "Push-Location -LiteralPath " + quote(filepath.Dir(it.Source)) + "\ntry {\n" + command + "\n} finally { Pop-Location }"
		label = "7-Zip · 纯净打包（不含后续校验）"
	case "extract":
		args := []string{"x", "-y", "-bd", map[string]string{"rename": "-aou", "skip": "-aos", "overwrite": "-aoa"}[c.ExtractConflict], "-o" + it.Target}
		types, err := extractTypeOptions(it.Source)
		if err != nil {
			return nil, err
		}
		args = append(args, types...)
		command = run(c.SevenZip, args)
		if c.ExtractPassword != "" {
			command += " ('-p'+$env:TOOLBOX_EXTRACT_PASSWORD)"
			command = "if ([string]::IsNullOrEmpty($env:TOOLBOX_EXTRACT_PASSWORD)) { throw '请先设置 TOOLBOX_EXTRACT_PASSWORD' }\n" + command
		}
		command += " -- " + quote(it.Source)
		label = "7-Zip · 原生解压（保留原包，绕过工具箱校验）"
	default:
		return nil, errors.New("此工具需要多步处理，请使用页面顶部的工具箱 CLI 命令")
	}
	return []VideoCommand{{Label: label, PowerShell: command}}, nil
}

func imageCompressionArgs(c Config, source, target string) []string {
	return append([]string{"-limit", "thread", "2"}, imageCompressionBaseArgs(c, source, target)...)
}
func imageCompressionBaseArgs(c Config, source, target string) []string {
	if !strings.EqualFold(filepath.Ext(source), ".png") {
		format := "JPEG:"
		if strings.EqualFold(filepath.Ext(source), ".webp") {
			format = "WEBP:"
		}
		return []string{source, "-auto-orient", "-quality", strconv.Itoa(c.Quality), format + target}
	}
	if c.PngMode == "" || c.PngMode == "lossless" {
		return []string{source, "-define", "png:compression-level=9", "-define", "png:compression-filter=5", "PNG00:" + target}
	}
	return []string{source, "-quality", strconv.Itoa(c.PngQuality), "-define", "webp:lossless=false", "-define", "webp:method=6", "-define", "webp:alpha-quality=100", "-define", "webp:use-sharp-yuv=true", "WEBP:" + target}
}
