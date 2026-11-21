package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"sort"
	"strconv"
	"strings"
)

// Never resolve embedded images or links from a confidential document. Even a
// benign-looking tracking image can disclose the user's IP or document data.
type notesSafeRenderer struct{}

func (notesSafeRenderer) RegisterFuncs(r renderer.NodeRendererFuncRegisterer) {
	r.Register(ast.KindFencedCodeBlock, func(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			w.WriteString("<pre")
			if line, ok := node.AttributeString("data-note-line"); ok {
				fmt.Fprintf(w, ` data-note-line="%s"`, line)
			}
			w.WriteString("><code>")
			for i := 0; i < node.Lines().Len(); i++ {
				segment := node.Lines().At(i)
				w.Write(util.EscapeHTML(segment.Value(source)))
			}
			w.WriteString("</code></pre>\n")
		}
		return ast.WalkSkipChildren, nil
	})

	r.Register(ast.KindImage, func(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			image := node.(*ast.Image)
			id := strings.TrimPrefix(string(image.Destination), "attachment:")
			if string(image.Destination) == "attachment:"+id && validNotesID(id) {
				w.WriteString(`<img data-note-attachment="` + id + `" alt="`)
				w.Write(util.EscapeHTML(image.Text(source)))
				w.WriteString(`">`)
			} else {
				w.WriteString(`<span class="notes-hidden-image">[图片已隐藏]</span>`)
			}
		}
		return ast.WalkSkipChildren, nil
	})
	r.Register(ast.KindLink, func(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			w.WriteString(`<span class="notes-prose-link">`)
		} else {
			w.WriteString("</span>")
		}
		return ast.WalkContinue, nil
	})
	r.Register(ast.KindAutoLink, func(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			w.WriteString(`<span class="notes-prose-link">`)
			w.Write(util.EscapeHTML(n.(*ast.AutoLink).Label(source)))
			w.WriteString("</span>")
		}
		return ast.WalkSkipChildren, nil
	})
}

var privateMarkdown = goldmark.New(goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.TaskList), goldmark.WithRendererOptions(renderer.WithNodeRenderers(util.Prioritized(notesSafeRenderer{}, 100))))

// One fixed, CSP-hashed stylesheet covers every permitted size. Selecting a
// size only changes the HTML class, never relaxes the sandbox or allows CSS.
var notesPreviewStyle = func() string {
	prose, e := assets.ReadFile("web/notes-prose.css")
	if e != nil {
		panic(e)
	}
	style := "html{container:notes-document / inline-size;font-size:14px;background:#fff;color-scheme:light;scrollbar-gutter:stable}body{margin-block:0}img{display:none}" + strings.ReplaceAll(strings.ReplaceAll(string(prose), "\r\n", "\n"), "\r", "\n")
	for size := 10; size <= 40; size++ {
		style += fmt.Sprintf("html.notes-font-%d{font-size:%dpx}", size, size)
	}
	return style
}()

var notesPreviewScript = func() string {
	syntax, err := assets.ReadFile("web/notes-syntax.js")
	if err != nil {
		panic(err)
	}
	bridge, err := assets.ReadFile("web/notes-preview.js")
	if err != nil {
		panic(err)
	}
	return strings.ReplaceAll(strings.ReplaceAll(string(syntax)+"\n"+string(bridge), "\r\n", "\n"), "\r", "\n")
}()

func notesPreviewScriptHash() string {
	h := sha256.Sum256([]byte(notesPreviewScript))
	return base64.StdEncoding.EncodeToString(h[:])
}

func notesMarkdown(source string) (string, error) {
	return notesMarkdownSized(source, 14)
}
func notesMarkdownSized(source string, size int) (string, error) {
	if !validNotesFontSize(size) {
		return "", errors.New("字号请选择 10–40px")
	}
	if len(source) > notesMaxText {
		return "", errors.New("正文最多 1 MB")
	}
	var b bytes.Buffer
	raw := []byte(source)
	doc := privateMarkdown.Parser().Parse(text.NewReader(raw))
	starts := []int{0}
	for at, c := range raw {
		if c == '\n' {
			starts = append(starts, at+1)
		}
	}
	ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && node.Type() == ast.TypeBlock && node.Lines().Len() > 0 {
			offset := node.Lines().At(0).Start
			line := sort.Search(len(starts), func(i int) bool { return starts[i] > offset }) - 1
			node.SetAttributeString("data-note-line", []byte(strconv.Itoa(line)))
		}
		return ast.WalkContinue, nil
	})
	if e := privateMarkdown.Renderer().Render(&b, raw, doc); e != nil {
		return "", e
	}
	// Only the fixed hash-authorized scroll bridge can run; no same-origin privileges or network access.
	return fmt.Sprintf(`<!doctype html><html lang="zh-CN" class="notes-font-%d"><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'sha256-%s'; img-src data:; style-src 'unsafe-inline'; form-action 'none'; base-uri 'none'"><style>`, size, notesPreviewScriptHash()) + notesPreviewStyle + `</style><body class="notes-prose">` + b.String() + `<script>` + notesPreviewScript + `</script></body></html>`, nil
}
