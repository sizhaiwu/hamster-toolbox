package toolbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNumberHundredResourcesFrom099(t *testing.T) {
	root := t.TempDir()
	for i := 1; i <= 100; i++ {
		file(t, filepath.Join(root, fmt.Sprintf("资源%d [1P-1B]", i), "child", "image.jpg"), []byte("x"))
	}
	file(t, filepath.Join(root, "Sample作者 - NO.001 已有资源 [1P-1B]", "old.jpg"), []byte("old"))
	file(t, filepath.Join(root, "普通文件.txt"), []byte("not a folder"))
	c := DefaultConfig()
	c.Tool = "number"
	c.Root = root
	c.NumberName = "Sample作者"
	c.StartNumber = 99
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Items) != 100 {
		t.Fatal(len(p.Items))
	}
	if !strings.HasSuffix(p.Items[0].Target, "Sample作者 - NO.099 资源1 [1P-1B]") || !strings.HasSuffix(p.Items[99].Target, "Sample作者 - NO.198 资源100 [1P-1B]") {
		t.Fatal(p.Items[0], p.Items[99])
	}
	if !strings.Contains(p.Items[1].Target, "NO.100 资源2 ") {
		t.Fatal("not natural order", p.Items[1])
	}
	for _, it := range p.Items {
		if it.Conflict != "" {
			t.Fatal(it)
		}
		if _, e := os.Stat(it.Source); e != nil {
			t.Fatal("preview changed directory", e)
		}
	}
	r := Execute(context.Background(), p, quiet, progress)
	if r.Success != 100 || r.Failed != 0 {
		t.Fatal(r)
	}
	if _, e := os.Stat(filepath.Join(p.Items[0].Target, "child", "image.jpg")); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(root, "Sample作者 - NO.001 已有资源 [1P-1B]", "old.jpg")); e != nil {
		t.Fatal(e)
	}
	p, e = BuildPlan(context.Background(), c, quiet)
	if e != nil || len(p.Items) != 0 {
		t.Fatal("rerun must skip numbered dirs", e, p)
	}
	info, e := InspectNumbering(context.Background(), Config{Root: root})
	if e != nil {
		t.Fatal(e)
	}
	if info.Name != "Sample作者" || info.NextNumber != 199 || info.Existing != 101 || info.Pending != 0 {
		t.Fatal(info)
	}
}

func TestNumberCollisionBlockedEvenWithDifferentDescriptions(t *testing.T) {
	root := t.TempDir()
	file(t, filepath.Join(root, "作者_NO.099 原资源", "a.jpg"), []byte("x"))
	file(t, filepath.Join(root, "新增资源", "b.jpg"), []byte("x"))
	c := DefaultConfig()
	c.Tool = "number"
	c.Root = root
	c.NumberName = "作者"
	c.StartNumber = 99
	c.NumberStyle = "underscore"
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Items) != 1 || !strings.Contains(p.Items[0].Conflict, "编号 99") {
		t.Fatal(p)
	}
	if CheckReady(p) == nil {
		t.Fatal("occupied number allowed")
	}
	info, e := InspectNumbering(context.Background(), Config{Root: root})
	if e != nil || info.NextNumber != 100 || info.Style != "underscore" {
		t.Fatal(info, e)
	}
	c.StartNumber = info.NextNumber
	c.NumberKeepOriginal = false
	p, e = BuildPlan(context.Background(), c, quiet)
	if e != nil || filepath.Base(p.Items[0].Target) != "作者_NO.100" || p.Items[0].Conflict != "" {
		t.Fatal(p, e)
	}
}

func TestNumberFilterAndModifiedOrder(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"update10", "update2", "ignore"} {
		os.Mkdir(filepath.Join(root, n), 0755)
	}
	earlier := time.Now().Add(-time.Hour)
	os.Chtimes(filepath.Join(root, "update10"), earlier, earlier)
	c := DefaultConfig()
	c.Tool = "number"
	c.Root = root
	c.NumberName = "作者"
	c.NumberFilter = "UPDATE"
	c.NumberOrder = "modified"
	c.StartNumber = 0
	c.NumberDigits = 2
	p, e := BuildPlan(context.Background(), c, quiet)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Items) != 2 || filepath.Base(p.Items[0].Source) != "update10" || !strings.Contains(p.Items[0].Target, "NO.00 ") {
		t.Fatal(p)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = BuildPlan(ctx, c, quiet); e == nil {
		t.Fatal("cancel ignored")
	}
}

func TestNumberValidationAndRootNameGuess(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "001-Sample作者 [12P-2V-1.20GB]")
	os.Mkdir(root, 0755)
	info, e := InspectNumbering(context.Background(), Config{Root: root})
	if e != nil || info.Name != "Sample作者" || info.NextNumber != 1 {
		t.Fatal(info, e)
	}
	c := DefaultConfig()
	c.Tool = "number"
	c.Root = root
	c.NumberName = "../escape"
	if _, e := BuildPlan(context.Background(), c, quiet); e == nil {
		t.Fatal("unsafe prefix accepted")
	}
	c.NumberName = "作者"
	c.NumberDigits = 0
	if _, e := BuildPlan(context.Background(), c, quiet); e == nil {
		t.Fatal("invalid digits accepted")
	}
	c.NumberDigits = 3
	c.StartNumber = 999999999
	os.Mkdir(filepath.Join(root, "one"), 0755)
	os.Mkdir(filepath.Join(root, "two"), 0755)
	if _, e := BuildPlan(context.Background(), c, quiet); e == nil {
		t.Fatal("overflow accepted")
	}
}

func TestNumberSpaceAndMarkerVariants(t *testing.T) {
	for _, tc := range []struct{ name, style string }{
		{"作者 NO.098 标题", "space"}, {"作者_NO.098 标题", "underscore"}, {"作者 - NO.098 标题", "dash"},
		{"作者 no098 标题", "space"}, {"作者_NO_098 标题", "underscore"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			file(t, filepath.Join(root, tc.name, "image.jpg"), []byte("x"))
			file(t, filepath.Join(root, "新增资源", "image.jpg"), []byte("x"))
			info, e := InspectNumbering(context.Background(), Config{Root: root})
			if e != nil || info.Name != "作者" || info.NextNumber != 99 || info.Style != tc.style || info.Pending != 1 {
				t.Fatal(info, e)
			}
			c := DefaultConfig()
			c.Root = root
			c.Tool = "number"
			c.NumberName = "作者"
			c.StartNumber = 99
			c.NumberStyle = "space"
			p, e := BuildPlan(context.Background(), c, quiet)
			if e != nil || len(p.Items) != 1 || filepath.Base(p.Items[0].Target) != "作者 NO.099 新增资源" {
				t.Fatal(p, e)
			}
		})
	}
	if _, _, _, _, ok := parseNumbered("piano001"); ok {
		t.Fatal("ordinary numeric name mistaken for sequence")
	}
}

func TestCustomNumberingTemplateIsUsedForReadingAndWriting(t *testing.T) {
	for _, template := range []string{"{name} {number}", "{name}_{number}", "{name}-{number}", "{number} - {name}", "{name} 第{number}期", "Vol.{number} - {name}", "{name}（{number}）"} {
		t.Run(template, func(t *testing.T) {
			root := t.TempDir()
			c := DefaultConfig()
			c.Root = root
			c.Tool = "number"
			c.NumberName = "作者"
			c.NumberStyle = "custom"
			c.NumberTemplate = template
			c.StartNumber = 99
			file(t, filepath.Join(root, formatNumberedName(c, 98)+" 已有资源", "a.jpg"), []byte("x"))
			file(t, filepath.Join(root, "新资源 [1P-1B]", "a.jpg"), []byte("x"))
			info, e := InspectNumbering(context.Background(), c)
			if e != nil || info.NextNumber != 99 || info.Style != "custom" || info.Pending != 1 {
				t.Fatal(info, e)
			}
			p, e := BuildPlan(context.Background(), c, quiet)
			if e != nil || len(p.Items) != 1 || filepath.Base(p.Items[0].Target) != formatNumberedName(c, 99)+" 新资源 [1P-1B]" {
				t.Fatal(p, e)
			}
			occupied := c
			occupied.StartNumber = 98
			collision, e := BuildPlan(context.Background(), occupied, quiet)
			if e != nil || len(collision.Items) != 1 || !strings.Contains(collision.Items[0].Conflict, "编号 98") {
				t.Fatal("custom number collision not detected", collision, e)
			}
			r := Execute(context.Background(), p, quiet, progress)
			if r.Success != 1 || r.Failed != 0 {
				t.Fatal(r)
			}
			p, e = BuildPlan(context.Background(), c, quiet)
			if e != nil || len(p.Items) != 0 {
				t.Fatal("numbered directories not skipped", p, e)
			}
			info, e = InspectNumbering(context.Background(), c)
			if e != nil || info.NextNumber != 100 {
				t.Fatal(info, e)
			}
		})
	}
}

func TestInvalidCustomNumberingTemplates(t *testing.T) {
	c := DefaultConfig()
	c.Root = t.TempDir()
	c.Tool = "number"
	c.NumberName = "作者"
	c.NumberStyle = "custom"
	for _, template := range []string{"{name}", "{number}", "{name}{number}", "{name}_{number}_{other}", "../{name}_{number}", "{name}_{number}_{number}"} {
		c.NumberTemplate = template
		if _, e := BuildPlan(context.Background(), c, quiet); e == nil {
			t.Fatal("invalid template accepted", template)
		}
	}
}
