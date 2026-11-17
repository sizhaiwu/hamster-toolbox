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
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

type Config struct {
	ThumbnailColumns, ThumbnailRows, ThumbnailWidth, ThumbnailGap, ThumbnailQuality            int
	ThumbnailFormat, ThumbnailBackground                                                       string
	ThumbnailInfo, ThumbnailTime                                                               bool
	PngMode                                                                                    string
	PngQuality                                                                                 int
	ImageFormats                                                                               []string
	Tool, Root, Output, Find, Replace, Prefix, Suffix, FFmpeg, FFprobe, Magick, Exiftool, Font string
	Recursive, Files, Dirs, Regex                                                              bool
	Quality, CRF, MinMB, MinKbps                                                               int
	NumberName, NumberStyle, NumberOrder, NumberFilter                                         string
	NumberTemplate                                                                             string
	StartNumber, NumberDigits                                                                  int
	NumberKeepOriginal                                                                         bool
	RenameMode                                                                                 string
	ExampleNumber, ExampleAtStart                                                              bool
}
type Item struct {
	Source, Target, Note, Conflict string
	Size                           int64
	Modified                       time.Time
}
type Plan struct {
	Config  Config
	Items   []Item
	Created time.Time
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
	return Config{ThumbnailColumns: 5, ThumbnailRows: 5, ThumbnailWidth: 1024, ThumbnailGap: 5, ThumbnailQuality: 90, ThumbnailFormat: "jpg", ThumbnailBackground: "#ffffff", ThumbnailInfo: true, ThumbnailTime: true, PngMode: "lossless", PngQuality: 90, ImageFormats: defaultImageFormats(), ExampleNumber: true, ExampleAtStart: true, Recursive: true, Files: true, Dirs: true, Quality: 60, CRF: 23, MinMB: 100, MinKbps: 1000, StartNumber: 1, NumberDigits: 3, NumberStyle: "dash", NumberOrder: "name", NumberKeepOriginal: true, Font: detectFont(), FFmpeg: detectProgram("ffmpeg", "ffmpeg"), FFprobe: detectProgram("ffprobe", "ffmpeg"), Magick: detectProgram("magick", "imagemagick"), Exiftool: detectProgram("exiftool", "exiftool")}
}
func Normalize(c Config) (Config, error) {
	c = ResolveDependencyPaths(ScopeConfig(c))
	if strings.TrimSpace(c.Root) == "" {
		return c, errors.New("请选择输入目录")
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
	case "format", "rename", "number", "describe", "exif", "pdf", "image", "video", "thumbnail":
	default:
		return c, errors.New("未知工具")
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
	for _, d := range RequiredDependencies(c) {
		if _, e = dependencyLocation(d); e != nil {
			return c, fmt.Errorf("%s：%s。请展开“本工具需要的程序”，选择文件或查看下载步骤", d.Name, e)
		}
	}
	if c.Tool == "image" || c.Tool == "video" || c.Tool == "exif" || c.Tool == "pdf" || c.Tool == "thumbnail" {
		if strings.TrimSpace(c.Output) == "" {
			c.Output = filepath.Join(c.Root, "toolbox-output", c.Tool)
			if c.Tool == "pdf" {
				c.Output = filepath.Join(c.Root, "toolbox-output", "目录索引.pdf")
			}
			if c.Tool == "thumbnail" {
				c.Output = filepath.Join(c.Root, "视频预览图")
			}
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
	return c, nil
}
func excluded(n string) bool {
	return n == "toolbox-output" || n == "compress_output" || n == "exif_clear_output" || n == ".build_index_tmp" || n == ".git"
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
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			if excluded(d.Name()) || (c.Output != "" && within(c.Output, p)) {
				return filepath.SkipDir
			}
			a = append(a, p)
			if !c.Recursive {
				return filepath.SkipDir
			}
		} else if !strings.EqualFold(p, c.Output) {
			a = append(a, p)
		}
		return nil
	})
	return a, e
}
func BuildPlan(ctx context.Context, c Config, log func(string)) (*Plan, error) {
	c, e := Normalize(c)
	if e != nil {
		return nil, e
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
		stats, e = measureDirectoryTree(ctx, c.Root)
		if e != nil {
			return nil, e
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
			base := regexp.MustCompile("\\s*\\[(?:\\d+(?:P|V|A)-)*(?:\\d+(?:\\.\\d+)?(?:B|KB|MB|GB|TB|PB))\\]$").ReplaceAllString(name, "")
			dst = filepath.Join(filepath.Dir(src), strings.TrimSpace(base)+" ["+strings.Join(parts, "-")+"]")
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
				m, e := probe(ctx, c.FFprobe, src)
				if e != nil {
					if ctx.Err() != nil {
						return nil, ctx.Err()
					}
					p.Items = append(p.Items, Item{Source: src, Conflict: "媒体信息读取失败：" + e.Error(), Note: "跳过无法读取的媒体", Size: s.Size(), Modified: s.ModTime()})
					log("无法读取媒体，预览中已标记：" + src)
					continue
				}
				if m.Bitrate < int64(c.MinKbps)*1000 {
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
				dst = strings.TrimSuffix(dst, filepath.Ext(dst)) + ".mp4"
			}
			note = "保留原件，输出到新目录"
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
		sort.SliceStable(p.Items, func(i, j int) bool { return len(p.Items[i].Source) > len(p.Items[j].Source) })
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
