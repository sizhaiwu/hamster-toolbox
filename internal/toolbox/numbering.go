package toolbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var numberedDirectory = regexp.MustCompile("(?i)^(.+?)NO[. _-]*(\\d+)(?:[^0-9]|$)")
var rootStatistics = regexp.MustCompile("\\s*\\[[^\\[\\]]*\\]$")
var rootIndex = regexp.MustCompile("^\\d+[-_ ]+")

type NumberInfo struct {
	Name, Style                                      string
	MaxNumber, NextNumber, Digits, Existing, Pending int
}
type numberEntry struct {
	path, name string
	info       os.FileInfo
}

func parseNumbered(name string) (prefix string, number, digits int, style string, ok bool) {
	m := numberedDirectory.FindStringSubmatch(name)
	if m == nil {
		return
	}
	// NO without a dot needs a separator, so an ordinary name such as piano001
	// is not mistaken for a resource sequence.
	before := m[1]
	if strings.TrimRight(before, " _-\t") == before && !strings.HasPrefix(strings.ToUpper(name[len(before):]), "NO.") {
		return "", 0, 0, "", false
	}
	number, e := strconv.Atoi(m[2])
	if e != nil {
		return "", 0, 0, "", false
	}
	prefix = strings.TrimRight(strings.TrimSpace(m[1]), " _-")
	if prefix == "" {
		return "", 0, 0, "", false
	}
	style = "space"
	if strings.HasSuffix(strings.TrimSpace(m[1]), "_") {
		style = "underscore"
	} else if strings.HasSuffix(strings.TrimSpace(m[1]), "-") {
		style = "dash"
	}
	return prefix, number, len(m[2]), style, true
}

type numberParser func(string) (string, int, int, string, bool)

// Custom templates use literal separators, not user-provided regular expressions.
// The same template both formats new names and identifies existing numbered names.
func numberingUsesName(c Config) bool {
	return c.NumberStyle != "number" && (c.NumberStyle != "custom" || strings.Contains(c.NumberTemplate, "{name}"))
}
func numberingName(c Config) string {
	if !numberingUsesName(c) {
		return ""
	}
	return strings.TrimSpace(c.NumberName)
}

var unnamedNO = regexp.MustCompile(`(?i)^NO[. _-]*([0-9]+)(?:[^0-9]|$)`)
var plainNumber = regexp.MustCompile(`^([0-9]+)(?:\s+-\s+.*)?$`)

func numberingParser(c Config) (numberParser, error) {
	fallback := func(s string) (string, int, int, string, bool) {
		if p, n, d, style, ok := parseNumbered(s); ok {
			return p, n, d, style, true
		}
		m := unnamedNO.FindStringSubmatch(s)
		if c.NumberStyle == "number" && m == nil {
			m = plainNumber.FindStringSubmatch(s)
		}
		if m != nil {
			n, e := strconv.Atoi(m[1])
			if e == nil {
				style := c.NumberStyle
				if style == "" || style == "custom" {
					style = "dash"
				}
				return "", n, len(m[1]), style, true
			}
		}
		return "", 0, 0, "", false
	}
	if c.NumberStyle != "custom" {
		return fallback, nil
	}
	template := c.NumberTemplate
	nameCount := strings.Count(template, "{name}")
	if nameCount > 1 || strings.Count(template, "{number}") != 1 {
		return nil, fmt.Errorf("自定义模板必须包含一次 {number}，{name} 可不填或使用一次")
	}
	literal := strings.NewReplacer("{name}", "", "{number}", "").Replace(template)
	if strings.ContainsAny(literal, "{}") || strings.Contains(template, "{name}{number}") || strings.Contains(template, "{number}{name}") {
		return nil, fmt.Errorf("不要使用其他占位符；同时使用名称与编号时，请用文字、空格或符号分隔")
	}
	sample := strings.TrimSpace(strings.NewReplacer("{name}", "资源名称", "{number}", "099").Replace(template))
	if c.NumberKeepOriginal {
		sample += " 原目录名"
	}
	if !validName(sample) {
		return nil, fmt.Errorf("自定义模板不能包含路径符号或其他非法文件名字符")
	}
	templates := []string{strings.TrimSpace(template)}
	if nameCount == 1 {
		unnamed := strings.TrimSpace(strings.ReplaceAll(template, "{name}", ""))
		if strings.TrimSpace(c.NumberName) == "" {
			// An omitted name must not capture the original directory description
			// as the resource name in a template such as "{number} - {name}".
			templates = append([]string{unnamed}, templates...)
		} else {
			templates = append(templates, unnamed)
		}
	}
	var parsers []*regexp.Regexp
	for _, t := range templates {
		pattern := regexp.QuoteMeta(t)
		capture := `(?P<name>.+?)`
		if name := strings.TrimSpace(c.NumberName); name != "" {
			capture = "(?P<name>" + regexp.QuoteMeta(name) + ")"
		}
		pattern = strings.ReplaceAll(pattern, regexp.QuoteMeta("{name}"), capture)
		pattern = strings.ReplaceAll(pattern, regexp.QuoteMeta("{number}"), `(?P<number>[0-9]+)`)
		re, e := regexp.Compile("(?i)^" + pattern + `(?:\s.*)?$`)
		if e != nil {
			return nil, e
		}
		parsers = append(parsers, re)
	}
	return func(s string) (string, int, int, string, bool) {
		for _, re := range parsers {
			m := re.FindStringSubmatch(s)
			if m == nil {
				continue
			}
			prefix := ""
			if at := re.SubexpIndex("name"); at >= 0 {
				prefix = m[at]
			}
			digits := m[re.SubexpIndex("number")]
			n, e := strconv.Atoi(digits)
			if e == nil {
				return prefix, n, len(digits), "custom", true
			}
		}
		return fallback(s)
	}, nil
}

func formatNumberedName(c Config, n int) string {
	number := fmt.Sprintf("%0*d", c.NumberDigits, n)
	if c.NumberStyle == "number" {
		if c.NumberKeepOriginal {
			return number + " -"
		}
		return number
	}
	if c.NumberStyle == "custom" {
		return strings.TrimSpace(strings.NewReplacer("{name}", strings.TrimSpace(c.NumberName), "{number}", number).Replace(c.NumberTemplate))
	}
	name := strings.TrimSpace(c.NumberName)
	if name == "" {
		return "NO." + number
	}
	separator := " - "
	switch c.NumberStyle {
	case "underscore":
		separator = "_"
	case "space":
		separator = " "
	}
	return name + separator + "NO." + number
}

func numberEntries(ctx context.Context, root string) ([]numberEntry, error) {
	if _, e := safePath(root); e != nil {
		return nil, e
	}
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("请先选择输入目录")
	}
	root, e := filepath.Abs(root)
	if e != nil {
		return nil, e
	}
	entries, e := os.ReadDir(root)
	if e != nil {
		return nil, fmt.Errorf("无法读取输入目录：%w", e)
	}
	result := []numberEntry{}
	for _, d := range entries {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !d.IsDir() || unsafeEntry(filepath.Join(root, d.Name()), d) || excluded(d.Name()) {
			continue
		}
		s, e := d.Info()
		if e != nil {
			return nil, e
		}
		result = append(result, numberEntry{filepath.Join(root, d.Name()), d.Name(), s})
	}
	return result, nil
}

func InspectNumbering(ctx context.Context, c Config) (NumberInfo, error) {
	parse, e := numberingParser(c)
	if e != nil {
		return NumberInfo{}, e
	}
	entries, e := numberEntries(ctx, c.Root)
	if e != nil {
		return NumberInfo{}, e
	}
	type tally struct {
		count int
		style string
	}
	prefixes := map[string]tally{}
	for _, d := range entries {
		prefix, _, _, style, ok := parse(d.name)
		if ok {
			v := prefixes[prefix]
			v.count++
			v.style = style
			prefixes[prefix] = v
		}
	}
	name := numberingName(c)
	if name == "" && numberingUsesName(c) {
		keys := make([]string, 0, len(prefixes))
		for k := range prefixes {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		best := 0
		chosen := false
		for _, k := range keys {
			if prefixes[k].count > best {
				name = k
				chosen = true
				best = prefixes[k].count
			}
		}
		if !chosen {
			root := filepath.Clean(c.Root)
			name = rootIndex.ReplaceAllString(rootStatistics.ReplaceAllString(filepath.Base(root), ""), "")
		}
	}
	result := NumberInfo{Name: name, Style: "dash", Digits: 3, NextNumber: 1}
	if c.NumberStyle == "number" {
		result.Style = "number"
	}
	for _, d := range entries {
		prefix, n, width, style, ok := parse(d.name)
		if ok {
			if strings.EqualFold(prefix, name) {
				result.Existing++
				if n >= result.MaxNumber {
					result.MaxNumber = n
					result.Style = style
				}
				if width > result.Digits {
					result.Digits = width
				}
			}
			continue
		}
		if c.NumberFilter == "" || strings.Contains(strings.ToLower(d.name), strings.ToLower(c.NumberFilter)) {
			result.Pending++
		}
	}
	if result.Digits > 9 {
		result.Digits = 9
	}
	if result.MaxNumber >= 999999999 {
		return result, fmt.Errorf("已有编号已达上限，请更换名称")
	}
	result.NextNumber = result.MaxNumber + 1
	if c.NumberStyle == "custom" {
		result.Style = "custom"
	}
	return result, nil
}

func validateNumberConfig(c Config) error {
	if name := numberingName(c); name != "" && !validName(name) {
		return fmt.Errorf("资源名称包含不支持的字符，或名称过长")
	}
	if c.StartNumber < 0 || c.StartNumber > 999999999 {
		return fmt.Errorf("起始编号须为 0–999999999")
	}
	if c.NumberDigits < 1 || c.NumberDigits > 9 {
		return fmt.Errorf("编号位数须为 1–9")
	}
	if c.NumberStyle != "dash" && c.NumberStyle != "underscore" && c.NumberStyle != "space" && c.NumberStyle != "custom" && c.NumberStyle != "number" {
		return fmt.Errorf("请选择有效的命名样式")
	}
	if _, e := numberingParser(c); e != nil {
		return e
	}
	if c.NumberOrder != "name" && c.NumberOrder != "modified" {
		return fmt.Errorf("请选择有效的编号顺序")
	}
	return nil
}

func buildNumberPlan(ctx context.Context, c Config, log func(string)) (*Plan, error) {
	parse, e := numberingParser(c)
	if e != nil {
		return nil, e
	}
	entries, e := numberEntries(ctx, c.Root)
	if e != nil {
		return nil, e
	}
	occupied := map[int]string{}
	pending := []numberEntry{}
	skipped := 0
	for _, d := range entries {
		prefix, n, _, _, ok := parse(d.name)
		if ok {
			skipped++
			if strings.EqualFold(prefix, numberingName(c)) {
				occupied[n] = d.name
			}
			continue
		}
		if c.NumberFilter != "" && !strings.Contains(strings.ToLower(d.name), strings.ToLower(c.NumberFilter)) {
			continue
		}
		pending = append(pending, d)
	}
	sort.Slice(pending, func(i, j int) bool {
		if c.NumberOrder == "modified" && !pending[i].info.ModTime().Equal(pending[j].info.ModTime()) {
			return pending[i].info.ModTime().Before(pending[j].info.ModTime())
		}
		return naturalLess(pending[i].name, pending[j].name)
	})
	if int64(c.StartNumber)+int64(len(pending))-1 > 999999999 {
		return nil, fmt.Errorf("本次资源数量超出编号上限，请降低起始编号")
	}
	p := &Plan{Config: c, Created: time.Now(), Items: []Item{}}
	for i, d := range pending {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		n := c.StartNumber + i
		name := formatNumberedName(c, n)
		if c.NumberKeepOriginal {
			name += " " + d.name
		}
		it := Item{Source: d.path, Target: filepath.Join(c.Root, name), Note: fmt.Sprintf("编号 %0*d", c.NumberDigits, n), Size: d.info.Size(), Modified: d.info.ModTime()}
		if !validName(name) {
			it.Conflict = "编号后名称不合法或过长"
		}
		if existing, ok := occupied[n]; ok {
			it.Conflict = fmt.Sprintf("编号 %d 已被 %s 使用，请调整起始编号", n, existing)
		}
		p.Items = append(p.Items, it)
	}
	checkPlanConflicts(p)
	log(fmt.Sprintf("扫描完成：待编号 %d 个目录，已跳过 %d 个有编号目录；仅处理直接子目录", len(p.Items), skipped))
	return p, nil
}

// Compare numeric chunks numerically: resource2 comes before resource10.
func naturalLess(a, b string) bool {
	aa, bb := []rune(strings.ToLower(a)), []rune(strings.ToLower(b))
	i, j := 0, 0
	digit := func(r rune) bool { return r >= '0' && r <= '9' }
	for i < len(aa) && j < len(bb) {
		if digit(aa[i]) && digit(bb[j]) {
			ie, je := i, j
			for ie < len(aa) && digit(aa[ie]) {
				ie++
			}
			for je < len(bb) && digit(bb[je]) {
				je++
			}
			x, y := strings.TrimLeft(string(aa[i:ie]), "0"), strings.TrimLeft(string(bb[j:je]), "0")
			if len(x) != len(y) {
				return len(x) < len(y)
			}
			if x != y {
				return x < y
			}
			i, j = ie, je
			continue
		}
		if aa[i] != bb[j] {
			return aa[i] < bb[j]
		}
		i++
		j++
	}
	if i == len(aa) && j != len(bb) {
		return true
	}
	if j == len(bb) && i != len(aa) {
		return false
	}
	return a < b
}
