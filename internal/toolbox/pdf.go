package toolbox

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// PDF pages are standard JPEG XObjects. Titles are rendered by ImageMagick
// with a configured CJK font, avoiding a bundled 60 MB font dependency.
func makePDF(ctx context.Context, p *Plan, log func(string), progress func(int, int)) error {
	c := p.Config
	if e := CheckReady(p); e != nil {
		return e
	}
	if e := os.MkdirAll(filepath.Dir(c.Output), 0755); e != nil {
		return e
	}
	tmp, e := os.MkdirTemp(filepath.Dir(c.Output), ".toolbox-pdf-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(tmp)
	pages := []string{}
	for i, it := range p.Items {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		title := CleanText(filepath.Base(it.Source))
		runes := []rune(title)
		lines := []string{}
		for len(runes) > 0 {
			n := 28
			if len(runes) < n {
				n = len(runes)
			}
			lines = append(lines, string(runes[:n]))
			runes = runes[n:]
		}
		title = strings.Join(lines, "\n")
		titleFile := filepath.Join(tmp, "title.txt")
		if e = os.WriteFile(titleFile, []byte(title), 0644); e != nil {
			return e
		}
		// Try candidate images in sorted WalkDir order; unsupported images are logged.
		cover := ""
		e = filepath.WalkDir(it.Source, func(f string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if d.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if d.IsDir() {
				if f != it.Source && excluded(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if !has(f, Images) {
				return nil
			}
			candidate := filepath.Join(tmp, "cover.jpg")
			_, ce := run(ctx, c.Magick, f+"[0]", "-auto-orient", "-thumbnail", "1120x1320>", "-background", "white", "-alpha", "remove", "-gravity", "center", "-extent", "1240x1754", candidate)
			if ce != nil {
				log("封面图片跳过：" + f)
				return nil
			}
			cover = candidate
			return fs.SkipAll
		})
		if e != nil {
			return e
		}
		args := []string{}
		if cover != "" {
			args = append(args, cover)
		} else {
			args = append(args, "-size", "1240x1754", "xc:white")
			log("目录无可解码图片，生成标题页：" + it.Source)
		}
		page := filepath.Join(tmp, fmt.Sprintf("page-%04d.jpg", i))
		args = append(args, "-fill", "white", "-draw", "rectangle 0,0 1240,260", "-font", filepath.ToSlash(c.Font), "-encoding", "UTF-8", "-pointsize", "32", "-fill", "#172c36", "-gravity", "northwest", "-annotate", "+48+40", "@"+titleFile, "-colorspace", "sRGB", "-type", "TrueColor", "-quality", "86", page)
		if _, e = runWithFont(ctx, c.Magick, args...); e != nil {
			return e
		}
		pages = append(pages, page)
		progress(i+1, len(p.Items))
	}
	pdfFile := filepath.Join(tmp, "index.pdf")
	if e = writePDF(pdfFile, pages); e != nil {
		return e
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return publishContext(ctx, pdfFile, c.Output)
}

type pdfWriter struct {
	writer io.Writer
	n      int
	err    error
}

func (b *pdfWriter) Write(p []byte) (int, error) {
	if b.err != nil {
		return 0, b.err
	}
	n, e := b.writer.Write(p)
	b.n += n
	b.err = e
	return n, e
}
func (b *pdfWriter) WriteString(s string) (int, error) { return b.Write([]byte(s)) }
func (b *pdfWriter) Len() int                          { return b.n }

func writePDF(path string, pages []string) error {
	out, e := os.Create(path)
	if e != nil {
		return e
	}
	defer out.Close()
	b := pdfWriter{writer: out}
	b.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	offsets := []int{0}
	obj := func(n int, data []byte) {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n", n)
		b.Write(data)
		b.WriteString("\nendobj\n")
	}
	obj(1, []byte("<< /Type /Catalog /Pages 2 0 R >>"))
	kids := []string{}
	for i := range pages {
		kids = append(kids, fmt.Sprintf("%d 0 R", 3+i*3))
	}
	obj(2, []byte(fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", len(pages), strings.Join(kids, " "))))
	for i, p := range pages {
		data, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		cfg, _, e := image.DecodeConfig(bytes.NewReader(data))
		if e != nil {
			return e
		}
		id := 3 + i*3
		obj(id, []byte(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595.28 841.89] /Resources << /XObject << /Im0 %d 0 R >> >> /Contents %d 0 R >>", id+1, id+2)))
		stream := fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n", cfg.Width, cfg.Height, len(data))
		obj(id+1, append(append([]byte(stream), data...), []byte("\nendstream")...))
		content := "q 595.28 0 0 841.89 0 0 cm /Im0 Do Q"
		obj(id+2, []byte("<< /Length "+strconv.Itoa(len(content))+" >>\nstream\n"+content+"\nendstream"))
	}
	start := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, o := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), start)
	if b.err != nil {
		return b.err
	}
	return out.Close()
}
