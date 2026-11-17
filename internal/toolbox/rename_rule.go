package toolbox

import (
	"fmt"
	"regexp"
	"strings"
)

type renameRule struct {
	pattern     *regexp.Regexp
	replacement string
}

var exampleDigits = regexp.MustCompile(`[0-9]+`)
var exampleSpaces = regexp.MustCompile(`[\s\x{3000}]+`)

func quoteExample(text string) string {
	var b strings.Builder
	last := 0
	for _, span := range exampleSpaces.FindAllStringIndex(text, -1) {
		b.WriteString(regexp.QuoteMeta(text[last:span[0]]))
		b.WriteString(`[\s\x{3000}]+`)
		last = span[1]
	}
	b.WriteString(regexp.QuoteMeta(text[last:]))
	return b.String()
}

func compileRenameRule(c Config) (renameRule, error) {
	if c.RenameMode == "example" {
		if strings.TrimSpace(c.Find) == "" {
			return renameRule{}, fmt.Errorf("请填写一个要删除或替换的名称示例，例如 test 第001期")
		}
		pattern := quoteExample(c.Find)
		hasNumber := false
		if c.ExampleNumber {
			matches := exampleDigits.FindAllStringIndex(c.Find, -1)
			if len(matches) > 0 {
				span := matches[len(matches)-1]
				pattern = quoteExample(c.Find[:span[0]]) + `(?P<number>[0-9]+)` + quoteExample(c.Find[span[1]:])
				hasNumber = true
			}
		}
		if c.ExampleAtStart {
			pattern = "^" + pattern
			if c.Replace == "" {
				pattern += `[\s\x{3000}]*`
			}
		}
		replacement := strings.ReplaceAll(c.Replace, "$", "$$")
		if strings.Contains(c.Replace, "{编号}") {
			if !hasNumber {
				return renameRule{}, fmt.Errorf("替换中使用了 {编号}，请启用编号可变，并在示例中填写数字")
			}
			replacement = strings.ReplaceAll(replacement, "{编号}", "${number}")
		}
		re, e := regexp.Compile(pattern)
		if e != nil {
			return renameRule{}, e
		}
		return renameRule{re, replacement}, nil
	}
	if c.RenameMode != "regex" && !c.Regex {
		return renameRule{}, nil
	}
	if c.Find == "" && c.Prefix == "" && c.Suffix == "" {
		return renameRule{}, fmt.Errorf("请填写正则表达式；不熟悉正则时，请选择“按示例（编号可变）”")
	}
	re, e := regexp.Compile(c.Find)
	if e != nil {
		return renameRule{}, fmt.Errorf("正则表达式无效：%w。请直接填写表达式，不要添加 /.../g；不支持前后断言", e)
	}
	return renameRule{re, c.Replace}, nil
}
