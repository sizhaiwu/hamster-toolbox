package toolbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegexRemovesOrReplacesPeriodPrefix(t *testing.T) {
	for _, tc := range []struct{ find, replacement, want string }{
		{`^test\s*第\d+期\s*`, "", "泡汤啦 [18P-3V-269.13MB]"},
		{`^test\s*第\d+期\s*`, "新名称 ", "新名称 泡汤啦 [18P-3V-269.13MB]"},
		{`^test\s*第(\d+)期\s*`, `新名称_NO.${1} `, "新名称_NO.001 泡汤啦 [18P-3V-269.13MB]"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			root := t.TempDir()
			old := filepath.Join(root, "test 第001期 泡汤啦 [18P-3V-269.13MB]")
			file(t, filepath.Join(old, "image.jpg"), []byte("original"))
			c := DefaultConfig()
			c.Root = root
			c.Tool = "rename"
			c.Dirs = true
			c.Files = false
			c.Find = tc.find
			c.Replace = tc.replacement
			p, e := BuildPlan(context.Background(), c, quiet)
			if e != nil || len(p.Items) != 0 {
				t.Fatal("literal mode must not match regex", p, e)
			}
			c.Regex = true
			p, e = BuildPlan(context.Background(), c, quiet)
			if e != nil || len(p.Items) != 1 || filepath.Base(p.Items[0].Target) != tc.want || p.Items[0].Conflict != "" {
				t.Fatal(p, e)
			}
			r := Execute(context.Background(), p, quiet, progress)
			if r.Success != 1 || r.Failed != 0 {
				t.Fatal(r)
			}
			b, e := os.ReadFile(filepath.Join(root, tc.want, "image.jpg"))
			if e != nil || string(b) != "original" {
				t.Fatal(e, string(b))
			}
		})
	}
}

func TestRegexErrorsAndEmptyResultAreVisible(t *testing.T) {
	c := DefaultConfig()
	c.Root = t.TempDir()
	c.Tool = "rename"
	c.Regex = true
	c.Find = `(?<=test)\d+`
	if _, e := BuildPlan(context.Background(), c, quiet); e == nil || !strings.Contains(e.Error(), "正则表达式无效") {
		t.Fatal(e)
	}
	c.Find = ""
	if _, e := BuildPlan(context.Background(), c, quiet); e == nil || !strings.Contains(e.Error(), "填写正则") {
		t.Fatal(e)
	}
	c.Find = `^test\s*第\d+期\s*`
	c.Files = false
	file(t, filepath.Join(c.Root, "test 第001期", "image.jpg"), []byte("x"))
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil || len(p.Items) != 1 || !strings.Contains(p.Items[0].Conflict, "名称为空") {
		t.Fatal(p, e)
	}
}
