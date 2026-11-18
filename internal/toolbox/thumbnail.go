package toolbox

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func normalizeThumbnailOptions(c *Config) error {
	// Missing numeric fields are from an older client with no thumbnail settings.
	if c.ThumbnailColumns == 0 {
		c.ThumbnailColumns = 5
	}
	if c.ThumbnailRows == 0 {
		c.ThumbnailRows = 5
	}
	if c.ThumbnailWidth == 0 {
		c.ThumbnailWidth = 1024
	}
	if c.ThumbnailQuality == 0 {
		c.ThumbnailQuality = 90
	}
	if c.ThumbnailFormat == "" {
		c.ThumbnailFormat = "jpg"
	}
	if c.ThumbnailBackground == "" {
		c.ThumbnailBackground = "#ffffff"
	}
	if c.ThumbnailColumns < 1 || c.ThumbnailColumns > 10 || c.ThumbnailRows < 1 || c.ThumbnailRows > 10 {
		return fmt.Errorf("缩略图行列数须为 1–10，每个视频最多 100 张画面")
	}
	if c.ThumbnailWidth < 320 || c.ThumbnailWidth > 4096 || c.ThumbnailGap < 0 || c.ThumbnailGap > 40 {
		return fmt.Errorf("拼图总宽度须为 320–4096 像素，间距须为 0–40 像素")
	}
	if (c.ThumbnailWidth-(c.ThumbnailColumns+1)*c.ThumbnailGap)/c.ThumbnailColumns < 80 {
		return fmt.Errorf("每格画面至少需要 80 像素宽，请增加总宽度或减少列数、间距")
	}
	if c.ThumbnailQuality < 1 || c.ThumbnailQuality > 100 {
		return fmt.Errorf("缩略图 JPEG 质量须为 1–100")
	}
	if c.ThumbnailFormat != "jpg" && c.ThumbnailFormat != "png" {
		return fmt.Errorf("缩略图请选择 JPEG 或 PNG")
	}
	if _, e := thumbnailColor(c.ThumbnailBackground); e != nil {
		return e
	}
	return nil
}

func thumbnailColor(s string) (color.RGBA, error) {
	if len(s) != 7 || s[0] != '#' {
		return color.RGBA{}, fmt.Errorf("背景颜色须为 #RRGGBB")
	}
	v, e := strconv.ParseUint(s[1:], 16, 24)
	if e != nil {
		return color.RGBA{}, fmt.Errorf("背景颜色须为 #RRGGBB")
	}
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}, nil
}

type thumbnailVideo struct {
	Width, Height         int
	Duration, Aspect, FPS float64
	Codec                 string
}

func thumbnailRatio(value string, separator string) float64 {
	parts := strings.Split(value, separator)
	if len(parts) != 2 {
		return 0
	}
	n, _ := strconv.ParseFloat(parts[0], 64)
	d, _ := strconv.ParseFloat(parts[1], 64)
	if n <= 0 || d <= 0 || math.IsInf(n, 0) || math.IsInf(d, 0) || math.IsNaN(n) || math.IsNaN(d) {
		return 0
	}
	return n / d
}

func probeThumbnail(ctx context.Context, exe, source string) (thumbnailVideo, error) {
	var raw struct {
		Format  struct{ Duration string }
		Streams []struct {
			Width, Height int
			Duration      string
			Codec         string `json:"codec_name"`
			Rate          string `json:"avg_frame_rate"`
			SAR           string `json:"sample_aspect_ratio"`
			Tags          struct{ Rotate string }
			SideData      []struct{ Rotation float64 } `json:"side_data_list"`
		}
	}
	// Full stream output also works with older FFprobe builds whose section names differ.
	out, e := run(ctx, exe, "-v", "error", "-select_streams", "V:0", "-show_streams", "-show_format", "-of", "json", source)
	if e != nil {
		return thumbnailVideo{}, e
	}
	if e = json.Unmarshal(out, &raw); e != nil {
		return thumbnailVideo{}, e
	}
	if len(raw.Streams) != 1 {
		return thumbnailVideo{}, fmt.Errorf("没有可用的视频画面")
	}
	s := raw.Streams[0]
	duration, _ := strconv.ParseFloat(s.Duration, 64)
	if duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		duration, _ = strconv.ParseFloat(raw.Format.Duration, 64)
	}
	if s.Width <= 0 || s.Height <= 0 || duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		return thumbnailVideo{}, fmt.Errorf("无法读取有效的视频时长或分辨率")
	}
	aspect := float64(s.Width) / float64(s.Height)
	if sar := thumbnailRatio(s.SAR, ":"); sar > 0 {
		aspect *= sar
	}
	rotation, _ := strconv.ParseFloat(s.Tags.Rotate, 64)
	if len(s.SideData) > 0 {
		rotation = s.SideData[0].Rotation
	}
	// FFmpeg applies display rotation while decoding. Match the resulting aspect.
	if math.Abs(math.Mod(rotation, 180)) > 45 && math.Abs(math.Mod(rotation, 180)) < 135 {
		aspect = 1 / aspect
	}
	return thumbnailVideo{s.Width, s.Height, duration, aspect, thumbnailRatio(s.Rate, "/"), s.Codec}, nil
}

type thumbnailLayout struct {
	CellWidth, CellHeight, HeaderHeight, Height, PointSize int
	Header                                                 string
}

func thumbnailWrap(text string, columns int) []string {
	lines := []string{}
	var line strings.Builder
	width := 0
	for _, r := range text {
		n := 1
		if r > 127 {
			n = 2
		}
		if width+n > columns && width > 0 {
			lines = append(lines, line.String())
			line.Reset()
			width = 0
		}
		line.WriteRune(r)
		width += n
	}
	if line.Len() > 0 {
		lines = append(lines, line.String())
	}
	return lines
}

func thumbnailTimestamp(seconds float64) string {
	n := int64(math.Round(seconds * 1000))
	return fmt.Sprintf("%02d:%02d:%02d", n/3600000, n/60000%60, n/1000%60)
}

func thumbnailDimensions(c Config, m thumbnailVideo, name string, size int64) (thumbnailLayout, error) {
	l := thumbnailLayout{CellWidth: (c.ThumbnailWidth - (c.ThumbnailColumns+1)*c.ThumbnailGap) / c.ThumbnailColumns, PointSize: max(12, min(24, c.ThumbnailWidth/64))}
	if m.Aspect <= 0 || math.IsNaN(m.Aspect) || math.IsInf(m.Aspect, 0) {
		return l, fmt.Errorf("视频宽高比无效")
	}
	height := float64(l.CellWidth) / m.Aspect
	if height < 24 || height > 4096 {
		return l, fmt.Errorf("画面比例过宽或过高，请调整拼图列数或总宽度")
	}
	l.CellHeight = int(math.Round(height))
	if c.ThumbnailInfo {
		columns := max(12, (c.ThumbnailWidth-20)*2/l.PointSize)
		lines := thumbnailWrap("文件："+CleanText(name), columns)
		info := fmt.Sprintf("时长：%s   分辨率：%d×%d   大小：%s   视频：%s", thumbnailTimestamp(m.Duration), m.Width, m.Height, Human(size), m.Codec)
		if m.FPS > 0 {
			info += fmt.Sprintf("   帧率：%.2f fps", m.FPS)
		}
		lines = append(lines, thumbnailWrap(info, columns)...)
		l.Header = strings.Join(lines, "\n")
		l.HeaderHeight = 20 + len(lines)*(l.PointSize+7)
	}
	l.Height = l.HeaderHeight + c.ThumbnailRows*l.CellHeight + (c.ThumbnailRows+1)*c.ThumbnailGap
	if int64(l.Height)*int64(c.ThumbnailWidth) > 40_000_000 {
		return l, fmt.Errorf("拼图过大，请减少行数或总宽度（最多 4000 万像素）")
	}
	return l, nil
}

func buildThumbnailPlan(ctx context.Context, c Config, log func(string)) (*Plan, error) {
	paths, e := scan(ctx, c)
	if e != nil {
		return nil, e
	}
	p := &Plan{Config: c, Created: time.Now(), Items: []Item{}}
	log(fmt.Sprintf("缩略图布局：%d 列 × %d 行，总宽 %d，输出 %s；均匀截取，避开首尾", c.ThumbnailColumns, c.ThumbnailRows, c.ThumbnailWidth, strings.ToUpper(c.ThumbnailFormat)))
	for _, source := range paths {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !has(source, Videos) {
			continue
		}
		st, e := os.Stat(source)
		if e != nil {
			return nil, e
		}
		if !st.Mode().IsRegular() {
			continue
		}
		rel, e := filepath.Rel(c.Root, source)
		if e != nil {
			return nil, e
		}
		it := Item{Source: source, Target: filepath.Join(c.Output, rel+"."+c.ThumbnailFormat), Size: st.Size(), Modified: st.ModTime()}
		if !validName(filepath.Base(it.Target)) {
			it.Conflict = "输出文件名过长或不合法，请先缩短视频文件名"
		} else {
			m, pe := probeThumbnail(ctx, c.FFprobe, source)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if pe != nil {
				it.Conflict = "视频读取失败：" + pe.Error()
			} else if l, le := thumbnailDimensions(c, m, filepath.Base(source), st.Size()); le != nil {
				it.Conflict = le.Error()
			} else {
				it.Note = fmt.Sprintf("%d 张画面，%d×%d，时长 %s", c.ThumbnailColumns*c.ThumbnailRows, c.ThumbnailWidth, l.Height, thumbnailTimestamp(m.Duration))
			}
		}
		p.Items = append(p.Items, it)
	}
	checkPlanConflicts(p)
	log(fmt.Sprintf("扫描完成：%d 个视频，已有目标与无法读取的文件将在执行时跳过", len(p.Items)))
	return p, nil
}

func thumbnailSeek(m thumbnailVideo, index, count int) float64 {
	seek := m.Duration * float64(index+1) / float64(count+1)
	// A one-frame clip has no decodable frame after its first timestamp.
	if m.FPS > 0 {
		seek = min(seek, max(0, m.Duration-1/m.FPS))
	}
	return seek
}

func makeThumbnail(ctx context.Context, c Config, it Item, target string) error {
	m, e := probeThumbnail(ctx, c.FFprobe, it.Source)
	if e != nil {
		return e
	}
	l, e := thumbnailDimensions(c, m, filepath.Base(it.Source), it.Size)
	if e != nil {
		return e
	}
	bg, e := thumbnailColor(c.ThumbnailBackground)
	if e != nil {
		return e
	}
	canvas := image.NewRGBA(image.Rect(0, 0, c.ThumbnailWidth, l.Height))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)
	count := c.ThumbnailColumns * c.ThumbnailRows
	for i := 0; i < count; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		frame := filepath.Join(filepath.Dir(target), "frame.png")
		seek := thumbnailSeek(m, i, count)
		filter := fmt.Sprintf("scale=%d:%d:flags=lanczos,setsar=1", l.CellWidth, l.CellHeight)
		// Input seeking decodes only a nearby GOP, rather than the whole video.
		_, e = run(ctx, c.FFmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-ss", strconv.FormatFloat(seek, 'f', 6, 64), "-i", it.Source, "-map", "0:V:0", "-an", "-sn", "-frames:v", "1", "-vf", filter, "-threads", "1", "-f", "image2", "-update", "1", frame)
		if e != nil {
			return fmt.Errorf("第 %d/%d 张画面截取失败：%w", i+1, count, e)
		}
		f, e := os.Open(frame)
		if e != nil {
			return fmt.Errorf("%s 处未能截取画面：%w", thumbnailTimestamp(seek), e)
		}
		img, de := png.Decode(f)
		f.Close()
		if de != nil {
			return de
		}
		if img.Bounds().Dx() != l.CellWidth || img.Bounds().Dy() != l.CellHeight {
			return fmt.Errorf("截帧尺寸与拼图设置不一致")
		}
		x := c.ThumbnailGap + (i%c.ThumbnailColumns)*(l.CellWidth+c.ThumbnailGap)
		y := l.HeaderHeight + c.ThumbnailGap + (i/c.ThumbnailColumns)*(l.CellHeight+c.ThumbnailGap)
		draw.Draw(canvas, image.Rect(x, y, x+l.CellWidth, y+l.CellHeight), img, img.Bounds().Min, draw.Src)
		// A failed later seek must never reuse an earlier frame.
		if e = os.Remove(frame); e != nil {
			return e
		}
	}
	base := filepath.Join(filepath.Dir(target), "sheet.png")
	f, e := os.Create(base)
	if e != nil {
		return e
	}
	e = png.Encode(f, canvas)
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	args := []string{base}
	if c.ThumbnailInfo || c.ThumbnailTime {
		// ImageMagick interprets backslashes in font paths as escape characters.
		args = append(args, "-font", filepath.ToSlash(c.Font), "-encoding", "UTF-8")
	}
	if c.ThumbnailInfo {
		header := filepath.Join(filepath.Dir(target), "info.txt")
		// Text comes from a file, so filenames cannot become command arguments.
		if e = os.WriteFile(header, []byte(strings.ReplaceAll(l.Header, "%", "%%")), 0600); e != nil {
			return e
		}
		ink := "#172c36"
		if (int(bg.R)*299+int(bg.G)*587+int(bg.B)*114)/1000 < 120 {
			ink = "white"
		}
		args = append(args, "-fill", ink, "-pointsize", strconv.Itoa(l.PointSize), "-interline-spacing", "7", "-gravity", "northwest", "-annotate", "+10+8", "@"+header)
	}
	if c.ThumbnailTime {
		point := min(18, max(10, l.CellWidth/12))
		for i := 0; i < count; i++ {
			x := c.ThumbnailGap + (i%c.ThumbnailColumns)*(l.CellWidth+c.ThumbnailGap) + 2
			top := l.HeaderHeight + c.ThumbnailGap + (i/c.ThumbnailColumns)*(l.CellHeight+c.ThumbnailGap)
			bottom := l.Height - (top + l.CellHeight) + 2
			// Trim to the actual glyph bounds and add equal padding on both
			// sides. Font bearings and baselines no longer offset the label.
			args = append(args, "(")
			args = append(args, thumbnailTimeLabelArgs(thumbnailTimestamp(thumbnailSeek(m, i, count)), point)...)
			args = append(args, ")", "-gravity", "southwest", "-geometry", fmt.Sprintf("+%d+%d", x, bottom), "-compose", "over", "-composite")
		}
	}
	args = append(args, "-strip", "-colorspace", "sRGB", "-quality", strconv.Itoa(c.ThumbnailQuality), target)
	if _, e = runWithFont(ctx, c.Magick, args...); e != nil {
		return fmt.Errorf("拼图保存失败：%w", e)
	}
	return nil
}

func thumbnailTimeLabelArgs(text string, point int) []string {
	return []string{"-background", "black", "-fill", "white", "-stroke", "none", "-interline-spacing", "0", "-pointsize", strconv.Itoa(point), "label:" + text, "-trim", "+repage", "-bordercolor", "black", "-border", "4x3"}
}
