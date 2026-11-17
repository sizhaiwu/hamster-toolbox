package toolbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExampleRenameWithoutRegex(t *testing.T) {
	for _, tc := range []struct{ original, find, replacement, want string }{
		{"test 第001期  泡汤啦 [18P-3V-269.13MB]", "test 第001期", "", "泡汤啦 [18P-3V-269.13MB]"},
		{"test 第099期 泡汤啦", "test 第001期", "新名称", "新名称 泡汤啦"},
		{"test 第1234期 泡汤啦", "test 第001期", "新名称_{编号}", "新名称_1234 泡汤啦"},
		{"2026合集 第099期 资源", "2026合集 第001期", "新合集_{编号}", "新合集_099 资源"},
		{"test (第099期) 资源", "test (第001期)", "", "资源"},
		{"test 第099期 资源", "test 第001期", "价格$1", "价格$1 资源"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			root := t.TempDir()
			file(t, filepath.Join(root, tc.original, "image.jpg"), []byte("untouched"))
			c := DefaultConfig()
			c.Root = root
			c.Tool = "rename"
			c.Files = false
			c.RenameMode = "example"
			c.Find = tc.find
			c.Replace = tc.replacement
			p, e := BuildPlan(context.Background(), c, quiet)
			if e != nil || len(p.Items) != 1 || filepath.Base(p.Items[0].Target) != tc.want || p.Items[0].Conflict != "" {
				t.Fatal(p, e)
			}
			r := Execute(context.Background(), p, quiet, progress)
			if r.Success != 1 || r.Failed != 0 {
				t.Fatal(r)
			}
			b, e := os.ReadFile(filepath.Join(root, tc.want, "image.jpg"))
			if e != nil || string(b) != "untouched" {
				t.Fatal(e)
			}
		})
	}
}

func TestExampleRenameScopeAndNumberSwitch(t *testing.T) {
	root := t.TempDir()
	file(t, filepath.Join(root, "test 第002期 资源", "image.jpg"), []byte("x"))
	file(t, filepath.Join(root, "其他 test 第003期 资源", "image.jpg"), []byte("x"))
	c := DefaultConfig()
	c.Root = root
	c.Tool = "rename"
	c.Files = false
	c.RenameMode = "example"
	c.Find = "test 第001期"
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil || len(p.Items) != 1 {
		t.Fatal(p, e)
	}
	c.ExampleNumber = false
	p, e = BuildPlan(context.Background(), c, quiet)
	if e != nil || len(p.Items) != 0 {
		t.Fatal(p, e)
	}
	c.ExampleNumber = true
	c.ExampleAtStart = false
	p, e = BuildPlan(context.Background(), c, quiet)
	if e != nil || len(p.Items) != 2 {
		t.Fatal(p, e)
	}
	c.Find = "test"
	c.Replace = "{编号}"
	if _, e = BuildPlan(context.Background(), c, quiet); e == nil || !strings.Contains(e.Error(), "没有可变编号") && !strings.Contains(e.Error(), "启用编号可变") {
		t.Fatal(e)
	}
}
