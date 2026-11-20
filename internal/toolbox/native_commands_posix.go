package toolbox

import (
	"errors"
	"path/filepath"
	"strings"
)

func posixQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'" }
func posixRun(exe string, args []string) string {
	words := []string{posixQuote(exe)}
	for _, arg := range args {
		words = append(words, posixQuote(arg))
	}
	return strings.Join(words, " ")
}
func nativePOSIXCommands(c Config, it Item) ([]VideoCommand, error) {
	guard := "if [ -e " + posixQuote(it.Target) + " ] || [ -L " + posixQuote(it.Target) + " ]; then printf '%s\\n' '目标已存在，请换输出路径' >&2; exit 1; fi\nmkdir -p -- " + posixQuote(filepath.Dir(it.Target)) + " &&\n"
	var code, label string
	switch c.Tool {
	case "image":
		code = guard + posixRun(c.Magick, imageCompressionArgs(c, it.Source, it.Target))
		label = "ImageMagick · 单张压缩"
	case "describe":
		code = posixRun(c.FFprobe, []string{"-v", "error", "-show_entries", "format=duration,size:stream=width,height", "-of", "json", it.Source})
		label = "FFprobe · 读取媒体信息（不改名）"
	case "exif":
		if has(it.Source, metadataVideos) && !has(it.Source, metadataExifVideos) {
			return nil, errors.New("此容器需要多步校验，请使用工具箱 CLI 命令")
		}
		code = guard + posixRun(c.Exiftool, []string{"-all=", "-o", it.Target, it.Source})
		label = "ExifTool · 清理副本元数据"
	case "archive":
		if c.ArchiveMode == "extras" || c.ArchiveMarker {
			return nil, errors.New("附加文件与标记需要多步处理，请使用工具箱 CLI")
		}
		args := archiveArgs(c, it.Target, false)
		args[len(args)-1] = filepath.Base(it.Source)
		words := []string{posixQuote(c.SevenZip)}
		for _, arg := range args {
			if strings.HasPrefix(arg, "-p") {
				continue
			}
			if arg == "--" && c.ArchivePassword != "" {
				words = append(words, `"-p${TOOLBOX_ARCHIVE_PASSWORD:?请先设置 TOOLBOX_ARCHIVE_PASSWORD}"`)
			}
			words = append(words, posixQuote(arg))
		}
		code = guard + "(cd -- " + posixQuote(filepath.Dir(it.Source)) + " && " + strings.Join(words, " ") + ")"
		label = "7-Zip · 纯净打包（不含后续校验）"
	case "extract":
		args := []string{"x", "-y", "-bd", map[string]string{"rename": "-aou", "skip": "-aos", "overwrite": "-aoa"}[c.ExtractConflict], "-o" + it.Target}
		types, err := extractTypeOptions(it.Source)
		if err != nil {
			return nil, err
		}
		args = append(args, types...)
		code = posixRun(c.SevenZip, args)
		if c.ExtractPassword != "" {
			code += ` "-p${TOOLBOX_EXTRACT_PASSWORD:?请先设置 TOOLBOX_EXTRACT_PASSWORD}"`
		}
		code += " -- " + posixQuote(it.Source)
		label = "7-Zip · 原生解压（保留原包，绕过工具箱校验）"
	default:
		return nil, errors.New("此工具需要多步处理，请使用工具箱 CLI 命令")
	}
	return []VideoCommand{{Label: label, PowerShell: code}}, nil
}
