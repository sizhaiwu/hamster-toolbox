package toolbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

type Config struct {
	ExtractSource                                                                                          string
	Workers                                                                                                int
	FormatCleanOther                                                                                       bool
	FormatDeleteMode                                                                                       string
	ExtractDeleteMode                                                                                      string
	ExtractDeleteSource                                                                                    bool
	DedupeProgram                                                                                          string
	ExtractConflict, ExtractLayout, ExtractPassword                                                        string
	SevenZip, ArchiveMode, ArchivePassword, ArchiveExtras, ArchiveSplit, ArchiveVolume, ArchiveFirstVolume string
	ArchiveDirPrefix, ArchiveFilePrefix                                                                    string
	ArchiveStart, ArchiveDigits, ArchiveLevel                                                              int
	ArchiveDirStart, ArchiveFileStart                                                                      int
	ArchiveMarker                                                                                          bool
	ThumbnailColumns, ThumbnailRows, ThumbnailWidth, ThumbnailGap, ThumbnailQuality                        int
	ThumbnailFormat, ThumbnailBackground                                                                   string
	ThumbnailInfo, ThumbnailTime                                                                           bool
	PngMode                                                                                                string
	PngQuality                                                                                             int
	ImageFormats                                                                                           []string
	Tool, Root, Output, Find, Replace, Prefix, Suffix, FFmpeg, FFprobe, Magick, Exiftool, Font             string
	Recursive, Files, Dirs, Regex                                                                          bool
	Quality, CRF, MinMB, MinKbps                                                                           int
	VideoHDR                                                                                               string
	VideoContainer                                                                                         string
	VideoMode, VideoAcceleration                                                                           string
	MetadataTypes                                                                                          string
	NumberName, NumberStyle, NumberOrder, NumberFilter                                                     string
	NumberTemplate                                                                                         string
	StartNumber, NumberDigits                                                                              int
	NumberKeepOriginal                                                                                     bool
	RenameMode                                                                                             string
	ExampleNumber, ExampleAtStart                                                                          bool
}
type Item struct {
	Action                         string `json:",omitempty"`
	Source, Target, Note, Conflict string
	Size                           int64
	Modified                       time.Time
}
type Plan struct {
	formatStats map[string]directoryStats
	safety      *planSafety
	archives    []archiveJob
	extracts    []extractJob
	Config      Config
	Items       []Item
	Created     time.Time
}

const Images = ".jpg .jpeg .jpe .jfif .png .gif .bmp .tif .tiff .webp .heic .heif .avif .psd .ico .svg .wmf .jpg2 .dif"
const Videos = ".mp4 .mpg .mpe .mpeg .qt .mov .m4v .wmv .avi .webm .flv .mkv .ts"
const Audios = ".mp3 .mid .midi .wav .m4a .ogg .ra .flac .aac .wma .opus"

func has(p, list string) bool {
	e := strings.ToLower(filepath.Ext(p))
	return e != "" && strings.Contains(" "+list+" ", " "+e+" ")
}
func Human(n int64) string {
	u := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	v := float64(n)
	i := 0
	for v >= 1024 && i < 5 {
		v /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%dB", n)
	}
	return fmt.Sprintf("%.2f%s", v, u[i])
}
func within(root, p string) bool {
	r, e := filepath.Rel(root, p)
	return e == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator))
}
func validName(n string) bool {
	if runtime.GOOS != "windows" && len(n) > 255 {
		return false
	}
	if n == "" || n == "." || n == ".." || strings.TrimRight(n, " .") != n || len(utf16.Encode([]rune(n))) > 255 || strings.ContainsAny(n, "<>:\"/\\|?*") {
		return false
	}
	for _, r := range n {
		if r < 32 {
			return false
		}
	}
	return !regexp.MustCompile("^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])$").MatchString(strings.ToUpper(strings.Split(n, ".")[0]))
}
func DefaultConfig() Config {
	return Config{VideoContainer: "original", VideoHDR: "preserve", Workers: 1, FormatDeleteMode: "recycle", ExtractDeleteMode: "recycle", DedupeProgram: DetectDedupeProgram(), ExtractConflict: "rename", ExtractLayout: "separate", SevenZip: detectSevenZip(), ArchiveMode: "pure", ArchiveSplit: "script", ArchiveVolume: "3g", ArchiveFirstVolume: "10k", ArchiveDirPrefix: "D_", ArchiveFilePrefix: "F_", ArchiveStart: 1, ArchiveDigits: 3, ThumbnailColumns: 5, ThumbnailRows: 5, ThumbnailWidth: 1024, ThumbnailGap: 5, ThumbnailQuality: 90, ThumbnailFormat: "jpg", ThumbnailBackground: "#ffffff", ThumbnailInfo: true, ThumbnailTime: true, PngMode: "lossless", PngQuality: 90, ImageFormats: defaultImageFormats(), ExampleNumber: true, ExampleAtStart: true, Recursive: true, Files: true, Dirs: true, Quality: 60, MetadataTypes: "images", VideoMode: "efficient", VideoAcceleration: "auto", CRF: 18, MinMB: 100, MinKbps: 1000, StartNumber: 1, NumberDigits: 3, NumberStyle: "dash", NumberOrder: "name", NumberKeepOriginal: true, Font: detectFont(), FFmpeg: detectProgram("ffmpeg", "ffmpeg"), FFprobe: detectProgram("ffprobe", "ffmpeg"), Magick: detectProgram("magick", "imagemagick"), Exiftool: detectProgram("exiftool", "exiftool")}
}
func Normalize(c Config) (Config, error) {
	if c.Tool == "dedupe" {
		return c, errors.New("文件去重使用外部软件，请点击打开去重软件，无需扫描预览")
	}
	c = ResolveDependencyPaths(ScopeConfig(c))
	if strings.TrimSpace(c.Root) == "" {
		return c, errors.New("请选择输入目录")
	}
	if _, e := configRootPath(c); e != nil {
		return c, fmt.Errorf("输入路径：%w", e)
	}
	var e error
	c.Root, e = filepath.Abs(c.Root)
	if e != nil {
		return c, e
	}
	s, e := os.Stat(c.Root)
	if e != nil || !s.IsDir() {
		return c, errors.New("输入目录不存在")
	}
	switch c.Tool {
	case "format", "rename", "number", "describe", "exif", "pdf", "image", "video", "thumbnail", "archive", "extract":
	default:
		return c, errors.New("未知工具")
	}
	if c.Tool == "extract" {
		if e := normalizeExtractOptions(&c); e != nil {
			return c, e
		}
		if c.ExtractSource != "" {
			c.ExtractSource, e = safePath(c.ExtractSource)
			if e != nil {
				return c, e
			}
			c.Root = filepath.Dir(c.ExtractSource)
		}

	}
	if c.Tool == "format" {
		if c.FormatDeleteMode == "" {
			c.FormatDeleteMode = "recycle"
		}
		if c.FormatDeleteMode != "recycle" && c.FormatDeleteMode != "permanent" {
			if c.FormatCleanOther {
				return c, errors.New("请选择移入回收站或永久删除")
			}
			c.FormatDeleteMode = "recycle"
		}
	}
	if c.Tool == "archive" {
		if e := normalizeArchiveOptions(&c); e != nil {
			return c, e
		}
	}
	if c.Tool == "number" {
		if e := validateNumberConfig(c); e != nil {
			return c, e
		}
	}
	if c.Tool == "thumbnail" {
		if e := normalizeThumbnailOptions(&c); e != nil {
			return c, e
		}
	}
	if c.Tool == "rename" {
		switch c.RenameMode {
		case "", "text", "example", "regex":
		default:
			return c, errors.New("请选择有效的查找方式")
		}
		if c.RenameMode != "" {
			c.Regex = c.RenameMode == "regex"
		}
		if !c.Files && !c.Dirs {
			return c, errors.New("至少选择文件或文件夹")
		}
		if _, e := compileRenameRule(c); e != nil {
			return c, e
		}
	}
	if c.Tool == "image" || c.Tool == "exif" || c.Tool == "thumbnail" || c.Tool == "video" {
		if c.Workers == 0 {
			c.Workers = 1
		}
		limit := 8
		if c.Tool == "video" {
			limit = 3
		}
		if c.Workers < 1 || c.Workers > limit {
			return c, fmt.Errorf("并发数须为 1–%d", limit)
		}
	}
	if c.Tool == "image" {

		c.ImageFormats, e = normalizeImageFormats(c.ImageFormats)
		if e != nil {
			return c, e
		}
		if e = normalizePNGOptions(&c); e != nil {
			return c, e
		}
		otherFormats := false
		for _, ext := range c.ImageFormats {
			if ext != ".png" {
				otherFormats = true
			}
		}
		if c.Quality < 1 || c.Quality > 100 {
			if otherFormats {
				return c, errors.New("图片质量必须为 1–100")
			}
			c.Quality = 60
		}
	}
	if c.Tool == "video" && (c.CRF < 0 || c.CRF > 51 || c.MinMB < 0 || c.MinKbps < 0) {
		return c, errors.New("CRF 必须为 0–51，阈值不能为负数")
	}
	if c.Tool == "video" {
		if e = normalizeVideoOptions(&c); e != nil {
			return c, e
		}
	}
	if c.Tool == "exif" {
		if c.MetadataTypes == "" {
			c.MetadataTypes = "images"
		}
		if c.MetadataTypes != "images" && c.MetadataTypes != "videos" && c.MetadataTypes != "all" {
			return c, errors.New("请选择要清理的文件类型")
		}
	}
	for _, d := range RequiredDependencies(c) {
		if _, e = dependencyLocation(d); e != nil {
			return c, fmt.Errorf("%s：%s。请展开“本工具需要的程序”，选择文件或查看下载步骤", d.Name, e)
		}
	}
	if c.Tool == "image" || c.Tool == "video" || c.Tool == "exif" || c.Tool == "pdf" || c.Tool == "thumbnail" || c.Tool == "archive" || c.Tool == "extract" {
		if strings.TrimSpace(c.Output) == "" {
			c.Output = filepath.Join(c.Root, "toolbox-output", c.Tool)
			if c.Tool == "extract" {
				c.Output = filepath.Join(c.Root, "解压资源")
			}
			if c.Tool == "archive" {
				c.Output = filepath.Join(c.Root, "0utput_7z")
			}
			if c.Tool == "pdf" {
				c.Output = filepath.Join(c.Root, "toolbox-output", "目录索引.pdf")
			}
			if c.Tool == "thumbnail" {
				c.Output = filepath.Join(c.Root, "视频预览图")
			}
		}
		// UI users enter folder names such as "south". Resolve those inside
		// the selected input directory, never against the launcher's working
		// directory (which is commonly the protected application directory).
		if !filepath.IsAbs(c.Output) {
			if filepath.VolumeName(c.Output) != "" || runtime.GOOS == "windows" && strings.HasPrefix(c.Output, `\`) {
				return c, errors.New("输出路径：请填写完整绝对路径，或相对于输入目录的文件夹名")
			}
			c.Output = filepath.Join(c.Root, c.Output)
		}
		c.Output, e = filepath.Abs(c.Output)
		if e != nil {
			return c, e
		}
		if c.Tool != "pdf" && within(c.Output, c.Root) {
			return c, errors.New("输出不能为输入目录或其上级")
		}
		if c.Tool == "pdf" && strings.ToLower(filepath.Ext(c.Output)) != ".pdf" {
			return c, errors.New("PDF 路径必须以 .pdf 结尾")
		}
	}
	if c.Output != "" {
		output, e := safePath(c.Output)
		if e != nil {
			return c, fmt.Errorf("输出路径 %q：%w", c.Output, e)
		}
		root, e := configRootPath(c)
		if e != nil {
			return c, e
		}
		if c.Tool != "pdf" && pathContains(output, root) {
			return c, errors.New("输出不能为输入目录或其上级（包括路径别名）")
		}
	}
	return c, nil
}
func excluded(n string) bool {
	return n == "toolbox-output" || n == "0utput_7z" || n == "compress_output" || n == "exif_clear_output" || n == ".build_index_tmp" || n == ".git" || strings.HasPrefix(n, ".extract-") || strings.HasPrefix(n, ".packing-") || strings.HasPrefix(n, ".toolbox-")
}
func scan(ctx context.Context, c Config) ([]string, error) {
	a := []string{}
	e := filepath.WalkDir(c.Root, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if p == c.Root {
			return nil
		}
		if unsafeEntry(p, d) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if excluded(d.Name()) || (c.Output != "" && within(c.Output, p)) {
				return filepath.SkipDir
			}
			if filter, ok := ctx.Value(remainingScanKey{}).(remainingScanFilter); ok && IndependentFileTask(c.Tool) {
				if !filter.directories[sourceKey(p)] || !c.Recursive {
					return filepath.SkipDir
				}
				return nil
			}
			a = append(a, p)
			if !c.Recursive {
				return filepath.SkipDir
			}
		} else if !strings.EqualFold(p, c.Output) {
			if filter, ok := ctx.Value(remainingScanKey{}).(remainingScanFilter); ok && IndependentFileTask(c.Tool) && !filter.sources[sourceKey(p)] {
				return nil
			}
			a = append(a, p)
		}
		return nil
	})
	return a, e
}
func buildPlan(ctx context.Context, c Config, log func(string)) (*Plan, error) {
	c, e := Normalize(c)
	if e != nil {
		return nil, e
	}
	if c.Tool == "extract" {
		return buildExtractPlan(ctx, c, log)
	}
	if c.Tool == "archive" {
		return buildArchivePlan(ctx, c, log)
	}
	if c.Tool == "number" {
		return buildNumberPlan(ctx, c, log)
	}
	if c.Tool == "thumbnail" {
		return buildThumbnailPlan(ctx, c, log)
	}
	paths, e := scan(ctx, c)
	if e != nil {
		return nil, e
	}
	p := &Plan{Config: c, Created: time.Now(), Items: []Item{}}
	imageFormats := map[string]bool{}
	if c.Tool == "image" {
		for _, ext := range c.ImageFormats {
			imageFormats[ext] = true
		}
		log("本次仅处理图片格式：" + strings.Join(c.ImageFormats, ", "))
		if imageFormats[".png"] {
			switch c.PngMode {
			case "webp-png":
				log(fmt.Sprintf("PNG 方案：WebP 编码、保留原 .png 文件名，画质 %d，尺寸不变", c.PngQuality))
			case "webp":
				log(fmt.Sprintf("PNG 方案：转为 .webp，画质 %d，尺寸不变", c.PngQuality))
			default:
				log("PNG 方案：原格式无损优化")
			}
		}
	}
	var renameRule renameRule
	if c.Tool == "rename" {
		renameRule, e = compileRenameRule(c)
		if e != nil {
			return nil, e
		}
	}
	var stats map[string]directoryStats
	if c.Tool == "format" {
		var cleanup []Item
		stats, cleanup, e = measureDirectoryTreeForFormat(ctx, c.Root, c.FormatCleanOther, c.FormatDeleteMode)
		if e != nil {
			return nil, e
		}
		p.Items = append(p.Items, cleanup...)
		p.formatStats = stats
		if c.FormatCleanOther {
			log(fmt.Sprintf("清理预览：%d 个非媒体文件；执行前请核对文件清单", len(cleanup)))
		}
	}
	for _, src := range paths {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		s, e := os.Stat(src)
		if e != nil {
			return nil, e
		}
		name := s.Name()
		dst := ""
		note := ""
		sourceVideoNote := ""
		switch c.Tool {
		case "rename":
			if s.IsDir() && !c.Dirs || !s.IsDir() && !c.Files {
				continue
			}
			if c.Find != "" {
				if renameRule.pattern != nil {
					name = renameRule.pattern.ReplaceAllString(name, renameRule.replacement)
				} else {
					name = strings.ReplaceAll(name, c.Find, c.Replace)
				}
			}
			ext := ""
			if !s.IsDir() {
				ext = filepath.Ext(name)
				name = strings.TrimSuffix(name, ext)
			}
			name = c.Prefix + name + c.Suffix + ext
			dst = filepath.Join(filepath.Dir(src), name)
			note = "重命名"
		case "format":
			if !s.IsDir() {
				continue
			}
			count, size := stats[src].Count, stats[src].Size
			parts := []string{}
			for i, k := range []string{"P", "V", "A"} {
				if count[i] > 0 {
					parts = append(parts, fmt.Sprintf("%d%s", count[i], k))
				}
			}
			parts = append(parts, Human(size))
			base := stripDirectoryStatsSuffix(name)
			if base != "" {
				base += " "
			}
			dst = filepath.Join(filepath.Dir(src), base+"["+strings.Join(parts, "-")+"]")
			note = "媒体数量 / 总大小"
		case "describe":
			if s.IsDir() || !has(src, Videos+" "+Audios) {
				continue
			}
			m, e := probe(ctx, c.FFprobe, src)
			if e != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				p.Items = append(p.Items, Item{Source: src, Conflict: "媒体信息读取失败：" + e.Error(), Note: "跳过无法读取的媒体", Size: s.Size(), Modified: s.ModTime()})
				log("无法读取媒体，预览中已标记：" + src)
				continue
			}
			n := int64(m.Duration)
			res := ""
			if m.Width > 0 {
				res = fmt.Sprintf("%dx%d,", m.Width, m.Height)
			}
			duration := fmt.Sprintf("%02d分%02d秒", n/60, n%60)
			if n >= 3600 {
				duration = fmt.Sprintf("%d小时%02d分%02d秒", n/3600, n/60%60, n%60)
			}
			ext := filepath.Ext(name)
			base := regexp.MustCompile("「[^「」]*」$").ReplaceAllString(strings.TrimSuffix(name, ext), "")
			dst = filepath.Join(filepath.Dir(src), fmt.Sprintf("%s「%s%s,%s」%s", base, res, duration, Human(s.Size()), ext))
			note = "分辨率 / 时长 / 大小"
		case "image", "exif", "video":
			if s.IsDir() {
				continue
			}
			if c.Tool == "video" {
				if !has(src, Videos) || s.Size() < int64(c.MinMB)*1024*1024 {
					continue
				}
				if _, err := videoOutputPath(c, src); err != nil {
					p.Items = append(p.Items, Item{Source: src, Conflict: err.Error(), Note: "保留原视频", Size: s.Size(), Modified: s.ModTime()})
					continue
				}
				v, e := probeCompressionVideo(ctx, c.FFprobe, src)
				if e != nil {
					if ctx.Err() != nil {
						return nil, ctx.Err()
					}
					p.Items = append(p.Items, Item{Source: src, Conflict: "媒体信息读取失败：" + e.Error(), Note: "跳过无法读取的媒体", Size: s.Size(), Modified: s.ModTime()})
					log("无法读取媒体，预览中已标记：" + src)
					continue
				}
				if v.Bitrate < int64(c.MinKbps)*1000 {
					continue
				}
				_, ve := effectiveHDRConfig(c, v)
				sourceVideoNote = hdrPlanNote(c, v)
				if ve != nil {
					if ctx.Err() != nil {
						return nil, ctx.Err()
					}
					p.Items = append(p.Items, Item{Source: src, Conflict: ve.Error(), Note: "保留原视频", Size: s.Size(), Modified: s.ModTime()})
					continue
				}
			} else if c.Tool == "exif" {
				isVideo, isImage := has(src, metadataVideos), has(src, Images)
				if (!isVideo && !isImage) || (isVideo && c.MetadataTypes == "images") || (isImage && c.MetadataTypes == "videos") {
					continue
				}
			} else if c.Tool == "image" && !imageFormats[strings.ToLower(filepath.Ext(src))] {
				continue
			} else if !has(src, Images) {
				continue
			}
			if c.Tool == "image" {
				reason, e := imageCompressionSkipReason(src)
				if e != nil {
					reason = "图片读取失败：" + e.Error()
				}
				if reason != "" {
					p.Items = append(p.Items, Item{Source: src, Conflict: reason, Size: s.Size(), Modified: s.ModTime()})
					continue
				}
			}
			rel, _ := filepath.Rel(c.Root, src)
			dst = filepath.Join(c.Output, rel)
			if c.Tool == "image" && strings.EqualFold(filepath.Ext(src), ".png") && c.PngMode == "webp" {
				dst = strings.TrimSuffix(dst, filepath.Ext(dst)) + ".webp"
			}
			if c.Tool == "video" {
				dst, _ = videoOutputPath(c, dst)
			}
			note = "保留原件，输出到新目录"
			if c.Tool == "exif" {
				if has(src, metadataVideos) {
					if has(src, metadataExifVideos) {
						note = "ExifTool 清理视频标签，不重新编码；原件保留"
					} else {
						note = "FFmpeg 流复制清理，不重新编码；原件保留"
					}
				} else {
					note = "图片元数据清理；原图保留"
				}
			}
			if c.Tool == "video" {
				note = sourceVideoNote
			}
			if c.Tool == "image" && strings.EqualFold(filepath.Ext(src), ".png") {
				if c.PngMode == "webp-png" {
					note = fmt.Sprintf("WebP 编码，保留原 .png 文件名，质量 %d", c.PngQuality)
				} else if c.PngMode == "webp" {
					note = fmt.Sprintf("转为 .webp，质量 %d", c.PngQuality)
				} else {
					note = "PNG 原格式无损优化"
				}
			}
		case "pdf":
			if !s.IsDir() || filepath.Dir(src) != c.Root {
				continue
			}
			dst = c.Output
			note = "目录封面页（首张可解码图片）"
		}
		if dst != "" && dst != src {
			it := Item{Source: src, Target: dst, Note: note, Size: s.Size(), Modified: s.ModTime()}
			if (c.Tool == "rename" || c.Tool == "format") && (!validName(nameForTarget(dst)) || c.Tool == "rename" && !validName(name)) {
				it.Conflict = "目标名称不合法"
				if c.Tool == "rename" && strings.TrimSpace(name) == "" {
					it.Conflict = "替换后名称为空，请填写替换内容或保留资源描述"
				}
			}
			p.Items = append(p.Items, it)
		}
	}
	if c.Tool == "rename" || c.Tool == "format" {
		sort.SliceStable(p.Items, func(i, j int) bool {
			if isFormatCleanup(p.Items[i]) != isFormatCleanup(p.Items[j]) {
				return isFormatCleanup(p.Items[i])
			}
			return len(p.Items[i].Source) > len(p.Items[j].Source)
		})
	}
	checkPlanConflicts(p)
	log(fmt.Sprintf("扫描完成：%d 项", len(p.Items)))
	return p, nil
}

func checkPlanConflicts(p *Plan) {
	seen := map[string]int{}
	for i := range p.Items {
		it := &p.Items[i]
		if it.Conflict != "" {
			continue
		}
		if isFormatCleanup(*it) {
			continue
		}
		key := strings.ToLower(filepath.Clean(it.Target))
		if previous, exists := seen[key]; p.Config.Tool != "pdf" && exists {
			it.Conflict = "多个输入映射到同一目标"
			p.Items[previous].Conflict = "多个输入映射到同一目标"
		}
		if _, e := os.Lstat(it.Target); e == nil {
			it.Conflict = "目标已存在"
		} else if !os.IsNotExist(e) {
			it.Conflict = e.Error()
		}
		seen[key] = i
	}
}
func nameForTarget(p string) string { return filepath.Base(p) }

type MediaInfo struct {
	Duration      float64
	Width, Height int
	Bitrate       int64
}

func probe(ctx context.Context, exe, src string) (MediaInfo, error) {
	m := MediaInfo{}
	out, e := run(ctx, exe, "-v", "error", "-show_entries", "format=duration,bit_rate:stream=width,height,bit_rate", "-of", "json", src)
	if e != nil {
		return m, e
	}
	var raw map[string]json.RawMessage
	if e = json.Unmarshal(out, &raw); e != nil {
		return m, e
	}
	var f map[string]string
	json.Unmarshal(raw["format"], &f)
	m.Duration, _ = strconv.ParseFloat(f["duration"], 64)
	m.Bitrate, _ = strconv.ParseInt(f["bit_rate"], 10, 64)
	var streams []map[string]interface{}
	json.Unmarshal(raw["streams"], &streams)
	for _, s := range streams {
		if w, ok := s["width"].(float64); ok && w > 0 {
			m.Width = int(w)
			if h, ok := s["height"].(float64); ok {
				m.Height = int(h)
			}
			if b, ok := s["bit_rate"].(string); ok {
				m.Bitrate, _ = strconv.ParseInt(b, 10, 64)
			}
			break
		}
	}
	return m, nil
}
func run(ctx context.Context, exe string, args ...string) ([]byte, error) {
	return runInDirectory(ctx, "", exe, args...)
}

func runInDirectory(ctx context.Context, dir, exe string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = dir
	hideWindow(cmd)
	out, e := cmd.Output()
	if e != nil {
		var ee *exec.ExitError
		if errors.As(e, &ee) {
			return nil, fmt.Errorf("%s: %s", filepath.Base(exe), strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, e
	}
	return out, nil
}
