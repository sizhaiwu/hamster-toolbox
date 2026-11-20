package toolbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// Dependency is also the UI's single source of truth for download/help content.
type Dependency struct {
	Key, Name, Purpose, Path, DownloadURL, GuideURL, Instructions string
	Available, Verified                                           bool
	Message                                                       string
}

func ToolExists(tool string) bool {
	switch tool {
	case "format", "rename", "number", "describe", "exif", "pdf", "image", "video", "thumbnail", "archive", "extract", "dedupe":
		return true
	}
	return false
}

func RequiredDependencies(c Config) []Dependency {
	ffmpeg := Dependency{Key: "FFmpeg", Name: "FFmpeg", Purpose: "压缩视频", Path: c.FFmpeg,
		DownloadURL: "https://www.gyan.dev/ffmpeg/builds/", GuideURL: "https://ffmpeg.org/download.html#build-windows",
		Instructions: "打开下载页，选择 release essentials 的 ZIP 包。解压整个文件夹，在这里选择 bin 文件夹中的 ffmpeg.exe。FFprobe 在同一个 bin 文件夹里，不需要单独下载。"}
	ffprobe := Dependency{Key: "FFprobe", Name: "FFprobe", Purpose: "读取音视频信息", Path: c.FFprobe,
		DownloadURL: ffmpeg.DownloadURL, GuideURL: ffmpeg.GuideURL,
		Instructions: "与 FFmpeg 在同一个安装包中。解压后，在这里选择 bin 文件夹中的 ffprobe.exe。"}
	magick := Dependency{Key: "Magick", Name: "ImageMagick", Purpose: "处理图片", Path: c.Magick,
		DownloadURL: "https://imagemagick.org/download/#windows", GuideURL: "https://imagemagick.org/download/#windows",
		Instructions: "普通 Windows 电脑选择 x64 安装包，安装完成后点击“自动查找”。也可以解压 portable 便携版，再选择其中的 magick.exe；请保留整个程序文件夹。"}
	exif := Dependency{Key: "Exiftool", Name: "ExifTool", Purpose: "清理图片与视频元数据", Path: c.Exiftool,
		DownloadURL: "https://exiftool.org/", GuideURL: "https://exiftool.org/install.html#Windows",
		Instructions: "下载 Windows 版本并解压整个压缩包，将 exiftool(-k).exe 改名为 exiftool.exe，然后在这里选择它。若压缩包中含 exiftool_files 文件夹，请与 EXE 放在一起。"}
	font := Dependency{Key: "Font", Name: "中文字体", Purpose: "显示 PDF 目录的中文标题", Path: c.Font,
		DownloadURL: "https://github.com/notofonts/noto-cjk/releases", GuideURL: "https://github.com/notofonts/noto-cjk",
		Instructions: "通常会自动找到系统自带的中文字体，无需下载。未找到时，可下载 Noto CJK 的 TTF 字体，解压后选择 .ttf 或 .ttc 文件。"}

	if runtime.GOOS != "windows" {
		ffmpeg.DownloadURL = "https://ffmpeg.org/download.html"
		ffmpeg.GuideURL = ffmpeg.DownloadURL
		ffmpeg.Instructions = "安装当前系统版本的 FFmpeg（Linux 可用发行版包管理器，macOS 可用 Homebrew），然后自动查找或选择 ffmpeg；ffprobe 通常同时安装。"
		ffprobe.DownloadURL = ffmpeg.DownloadURL
		ffprobe.GuideURL = ffmpeg.GuideURL
		ffprobe.Instructions = "与 FFmpeg 一起安装，选择 ffprobe 可执行文件。"
		magick.DownloadURL = "https://imagemagick.org/script/download.php"
		magick.GuideURL = magick.DownloadURL
		magick.Instructions = "安装 ImageMagick 7，选择 magick 可执行文件；macOS 可用 Homebrew，Linux 请使用发行版或官方安装方式。"
		exif.GuideURL = "https://exiftool.org/install.html"
		exif.Instructions = "安装当前系统版本的 ExifTool，选择 exiftool 可执行文件；需要系统有 Perl。"
	}
	switch c.Tool {
	case "archive", "extract":
		if runtime.GOOS != "windows" {
			return []Dependency{{Key: "SevenZip", Name: "7-Zip", Purpose: "打包、解压和校验资源", Path: c.SevenZip, DownloadURL: "https://www.7-zip.org/download.html", GuideURL: "https://www.7-zip.org/", Instructions: "安装当前系统与架构的 7-Zip，选择 7zz 或 7z 可执行文件；RAR 需要包含对应格式支持的版本。"}}
		}
		return []Dependency{{Key: "SevenZip", Name: "7-Zip", Purpose: "使用 7-Zip 打包、解压与校验资源", Path: c.SevenZip, DownloadURL: "https://www.7-zip.org/download.html", GuideURL: "https://www.7-zip.org/", Instructions: "完整版已内置，无需安装。手动设置时，下载 Windows x64 版本并选择 7z.exe，保留同目录的 7z.dll。精简版 7za.exe 不支持 RAR。"}}
	case "describe":
		return []Dependency{ffprobe}
	case "video":
		return []Dependency{ffmpeg, ffprobe}
	case "image":
		return []Dependency{magick}
	case "exif":
		if c.MetadataTypes == "videos" || c.MetadataTypes == "all" {
			ffmpeg.Purpose = "以流复制方式清理视频容器元数据"
			return []Dependency{exif, ffmpeg, ffprobe}
		}
		return []Dependency{exif}
	case "pdf":
		return []Dependency{magick, font}
	case "thumbnail":
		ffmpeg.Purpose = "从视频中截取预览画面"
		magick.Purpose = "添加文字并保存缩略图拼图"
		font.Purpose = "显示缩略图中的中文文件名与时间"
		return []Dependency{ffmpeg, ffprobe, magick, font}
	default:
		return []Dependency{}
	}
}

func dependencyLocation(d Dependency) (string, error) {
	if strings.TrimSpace(d.Path) == "" {
		return "", fmt.Errorf("尚未配置")
	}
	if d.Key == "Font" {
		ext := strings.ToLower(filepath.Ext(d.Path))
		if ext != ".ttf" && ext != ".ttc" && ext != ".otf" {
			return "", fmt.Errorf("请选择 TTF、TTC 或 OTF 字体文件")
		}
		s, e := os.Stat(d.Path)
		if e != nil || !s.Mode().IsRegular() {
			return "", fmt.Errorf("字体文件不存在")
		}
		return d.Path, nil
	}
	p, e := exec.LookPath(d.Path)
	if e != nil {
		return "", fmt.Errorf("未找到程序，请重新选择")
	}
	s, e := os.Stat(p)
	if e != nil || !s.Mode().IsRegular() {
		return "", fmt.Errorf("请选择可执行程序文件")
	}
	return p, nil
}

func CheckDependencies(ctx context.Context, c Config, verify bool) []Dependency {
	c = ResolveDependencyPaths(c)
	deps := RequiredDependencies(c)
	for i := range deps {
		d := &deps[i]
		p, e := dependencyLocation(*d)
		if e != nil {
			d.Message = e.Error()
			continue
		}
		d.Available = true
		d.Message = "已找到"
		if !verify {
			continue
		}
		if d.Key == "Font" {
			d.Message = "字体文件已找到"
			continue
		}
		checkCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		arg := "-version"
		if d.Key == "Exiftool" {
			arg = "-ver"
		}
		if d.Key == "SevenZip" {
			arg = "i"
		}
		out, e := run(checkCtx, p, arg)
		timedOut := checkCtx.Err() != nil
		cancel()
		if e != nil {
			d.Available = false
			if timedOut {
				d.Message = "检测超时，请确认程序安装完整"
			} else {
				d.Message = "程序无法运行，请确认安装完整或重新选择"
			}
			continue
		}
		version := strings.TrimSpace(strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0])
		correct := false
		switch d.Key {
		case "FFmpeg":
			correct = strings.HasPrefix(strings.ToLower(version), "ffmpeg version")
		case "FFprobe":
			correct = strings.HasPrefix(strings.ToLower(version), "ffprobe version")
		case "SevenZip":
			correct = strings.Contains(version, "7-Zip")
		case "Magick":
			correct = strings.Contains(version, "ImageMagick")
		case "Exiftool":
			correct = regexp.MustCompile(`^\d+\.\d+`).MatchString(version)
		}
		if !correct {
			d.Available = false
			d.Message = "选中的文件不是 " + d.Name + "，请重新选择"
			continue
		}
		if len(version) > 180 {
			version = version[:180]
		}
		d.Verified = true
		d.Message = "可用 · " + version
	}
	return deps
}

// Ignore fields unrelated to the selected tool, even from older saved settings.
func ScopeConfig(c Config) Config {
	r := Config{Tool: c.Tool, Root: c.Root, Recursive: c.Recursive}
	if c.Tool == "image" || c.Tool == "exif" || c.Tool == "thumbnail" || c.Tool == "video" {
		r.Workers = c.Workers
	}
	switch c.Tool {
	case "format":
		r.FormatCleanOther, r.FormatDeleteMode = c.FormatCleanOther, c.FormatDeleteMode
	case "dedupe":
		return Config{Tool: c.Tool, DedupeProgram: c.DedupeProgram}
	case "extract":
		r.Output, r.SevenZip = c.Output, c.SevenZip
		r.ExtractConflict, r.ExtractLayout, r.ExtractPassword = c.ExtractConflict, c.ExtractLayout, c.ExtractPassword
		r.ExtractDeleteSource = c.ExtractDeleteSource
		r.ExtractDeleteMode = c.ExtractDeleteMode
	case "archive":
		r.Output, r.SevenZip = c.Output, c.SevenZip
		r.Files, r.Dirs = c.Files, c.Dirs
		r.ArchiveMode, r.ArchivePassword, r.ArchiveExtras = c.ArchiveMode, c.ArchivePassword, c.ArchiveExtras
		r.ArchiveSplit, r.ArchiveVolume, r.ArchiveFirstVolume = c.ArchiveSplit, c.ArchiveVolume, c.ArchiveFirstVolume
		r.ArchiveDirPrefix, r.ArchiveFilePrefix = c.ArchiveDirPrefix, c.ArchiveFilePrefix
		r.ArchiveStart, r.ArchiveDigits, r.ArchiveLevel = c.ArchiveStart, c.ArchiveDigits, c.ArchiveLevel
		r.ArchiveMarker = c.ArchiveMarker
		r.Recursive = false
	case "thumbnail":
		r.Output, r.FFmpeg, r.FFprobe, r.Magick = c.Output, c.FFmpeg, c.FFprobe, c.Magick
		r.ThumbnailColumns, r.ThumbnailRows = c.ThumbnailColumns, c.ThumbnailRows
		r.ThumbnailWidth, r.ThumbnailGap = c.ThumbnailWidth, c.ThumbnailGap
		r.ThumbnailQuality, r.ThumbnailFormat = c.ThumbnailQuality, c.ThumbnailFormat
		r.ThumbnailBackground = c.ThumbnailBackground
		r.ThumbnailInfo, r.ThumbnailTime = c.ThumbnailInfo, c.ThumbnailTime
		r.Font = c.Font
	case "number":
		r.NumberName = c.NumberName
		r.NumberStyle = c.NumberStyle
		r.NumberTemplate = c.NumberTemplate
		r.NumberOrder = c.NumberOrder
		r.NumberFilter = c.NumberFilter
		r.StartNumber = c.StartNumber
		r.NumberDigits = c.NumberDigits
		r.NumberKeepOriginal = c.NumberKeepOriginal
		r.Recursive = false
	case "rename":
		r.RenameMode = c.RenameMode
		r.ExampleNumber = c.ExampleNumber
		r.ExampleAtStart = c.ExampleAtStart
		r.Files = c.Files
		r.Dirs = c.Dirs
		r.Find = c.Find
		r.Replace = c.Replace
		r.Regex = c.Regex
		r.Prefix = c.Prefix
		r.Suffix = c.Suffix
	case "describe":
		r.FFprobe = c.FFprobe
	case "image":
		r.PngMode = c.PngMode
		r.PngQuality = c.PngQuality
		if c.ImageFormats != nil {
			r.ImageFormats = append([]string{}, c.ImageFormats...)
		}
		r.Output = c.Output
		r.Magick = c.Magick
		r.Quality = c.Quality
	case "exif":
		r.Output = c.Output
		r.Exiftool = c.Exiftool
		r.MetadataTypes = c.MetadataTypes
		if c.MetadataTypes == "videos" || c.MetadataTypes == "all" {
			r.FFmpeg, r.FFprobe = c.FFmpeg, c.FFprobe
		}
	case "video":
		r.Output = c.Output
		r.VideoMode, r.VideoAcceleration = c.VideoMode, c.VideoAcceleration
		r.VideoHDR = c.VideoHDR
		r.FFmpeg = c.FFmpeg
		r.FFprobe = c.FFprobe
		r.CRF = c.CRF
		r.MinMB = c.MinMB
		r.MinKbps = c.MinKbps
	case "pdf":
		r.Output = c.Output
		r.Magick = c.Magick
		r.Font = c.Font
		r.Recursive = false
	}
	return r
}

func programName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func detectProgram(name, portableFolder string) string {
	base, _ := os.Executable()
	base = filepath.Dir(base)
	for _, p := range []string{filepath.Join(base, "tools", portableFolder, programName(name)), filepath.Join(base, "tools", portableFolder, "bin", programName(name)), filepath.Join(base, "tools", programName(name))} {
		if s, e := os.Stat(p); e == nil && s.Mode().IsRegular() {
			return p
		}
	}
	if p, e := exec.LookPath(name); e == nil {
		return p
	}
	if runtime.GOOS != "windows" {
		for _, dir := range []string{"/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin"} {
			p := filepath.Join(dir, name)
			if st, e := os.Stat(p); e == nil && st.Mode().IsRegular() && st.Mode().Perm()&0111 != 0 {
				return p
			}
		}
	}
	if name == "magick" {
		for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
			root := os.Getenv(env)
			if root == "" {
				continue
			}
			matches, _ := filepath.Glob(filepath.Join(root, "ImageMagick*", "magick.exe"))
			if len(matches) > 0 {
				return matches[len(matches)-1]
			}
		}
	}
	return ""
}

func detectFont() string {
	if path := bundledDependency(executableDirectory(), "Font"); path != "" {
		return path
	}
	if runtime.GOOS != "windows" {
		home, _ := os.UserHomeDir()
		for _, p := range []string{"/System/Library/Fonts/PingFang.ttc", "/System/Library/Fonts/STHeiti Medium.ttc", "/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc", "/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc", filepath.Join(home, ".local/share/fonts/NotoSansCJKsc-Regular.otf"), filepath.Join(home, "Library/Fonts/NotoSansCJKsc-Regular.otf")} {
			if regularFile(p) {
				return p
			}
		}
		return ""
	}
	root := os.Getenv("WINDIR")
	if root == "" {
		root = "C:/Windows"
	}
	base, _ := os.Executable()
	for _, p := range []string{filepath.Join(filepath.Dir(base), "tools", "fonts", "NotoSansCJKsc-Regular.ttf"), filepath.Join(root, "Fonts", "msyh.ttc"), filepath.Join(root, "Fonts", "simsun.ttc"), filepath.Join(root, "Fonts", "simhei.ttf")} {
		if s, e := os.Stat(p); e == nil && s.Mode().IsRegular() {
			return p
		}
	}
	return ""
}

func detectSevenZip() string {
	if runtime.GOOS != "windows" {
		if p := detectProgram("7zz", "sevenzip"); p != "" {
			return p
		}
	}
	if p := detectProgram("7z", "sevenzip"); p != "" {
		return p
	}
	return detectProgram("7za", "sevenzip")
}
