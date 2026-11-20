package toolbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type extractMember struct {
	Name string
	Size int64
	Dir  bool
}
type extractJob struct {
	volumeIDs   map[string]fileIdentity
	VolumeCount int
	Volumes     []archiveEntry
	Members     []extractMember
}

var errExtractSkipped = errors.New("所有文件均已跳过")

func normalizeExtractOptions(c *Config) error {
	if c.ExtractDeleteMode == "" {
		c.ExtractDeleteMode = "recycle"
	}
	if c.ExtractDeleteMode != "recycle" && c.ExtractDeleteMode != "permanent" {
		if c.ExtractDeleteSource {
			return errors.New("请选择移入回收站或永久删除")
		}
		c.ExtractDeleteMode = "recycle"
	}
	if c.ExtractConflict == "" {
		c.ExtractConflict = "rename"
	}
	if c.ExtractLayout == "" {
		c.ExtractLayout = "separate"
	}
	if c.ExtractConflict != "rename" && c.ExtractConflict != "skip" && c.ExtractConflict != "overwrite" {
		return errors.New("请选择有效的重名文件处理规则")
	}
	if c.ExtractLayout != "separate" && c.ExtractLayout != "merge" {
		return errors.New("请选择有效的解压目录规则")
	}
	if strings.ContainsAny(c.ExtractPassword, "\x00\r\n") {
		return errors.New("密码不能包含换行或空字符")
	}
	return nil
}

var extractSplitPattern = regexp.MustCompile(`(?i)\.(7z|zip|rar)\.(\d+)$`)
var extractRARPartPattern = regexp.MustCompile(`(?i)^(.*)\.part(\d+)\.rar$`)

func extractArchiveName(path string) (string, bool) {
	name := filepath.Base(path)
	if m := extractSplitPattern.FindStringSubmatch(name); m != nil {
		n, _ := strconv.Atoi(m[2])
		if n != 1 {
			return "", false
		}
		base := name[:len(name)-len(m[2])-1]
		return strings.TrimSuffix(base, filepath.Ext(base)), true
	}
	if m := extractRARPartPattern.FindStringSubmatch(name); m != nil {
		n, _ := strconv.Atoi(m[2])
		return m[1], n == 1
	}
	if m := extractNumericSplitPattern.FindStringSubmatch(name); m != nil {
		n, _ := strconv.Atoi(m[1])
		return strings.TrimSuffix(name, m[0]), n == 1
	}
	if !has(name, ".7z .zip .rar") {
		return "", false
	}
	return strings.TrimSuffix(name, filepath.Ext(name)), true
}

func extractVolumes(source string) ([]archiveEntry, error) {
	name := filepath.Base(source)
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	entries, e := os.ReadDir(filepath.Dir(source))
	if e != nil {
		return nil, e
	}
	match := func(n string) bool { return strings.EqualFold(n, name) }
	if m := extractSplitPattern.FindStringSubmatch(name); m != nil {
		base := name[:len(name)-len(m[2])]
		re := regexp.MustCompile(`(?i)^` + regexp.QuoteMeta(base) + `\d+$`)
		match = re.MatchString
	} else if m := extractNumericSplitPattern.FindStringSubmatch(name); m != nil {
		base := name[:len(name)-len(m[1])]
		re := regexp.MustCompile(`(?i)^` + regexp.QuoteMeta(base) + `\d{3,}$`)
		match = re.MatchString
	} else if m := extractRARPartPattern.FindStringSubmatch(name); m != nil {
		re := regexp.MustCompile(`(?i)^` + regexp.QuoteMeta(m[1]) + `\.part\d+\.rar$`)
		match = re.MatchString
	} else if has(name, ".rar") {
		re := regexp.MustCompile(`(?i)^` + regexp.QuoteMeta(stem) + `\.[r-z]\d{2}$`)
		match = func(n string) bool { return strings.EqualFold(n, name) || re.MatchString(n) }
	} else if has(name, ".zip") {
		re := regexp.MustCompile(`(?i)^` + regexp.QuoteMeta(stem) + `\.z\d+$`)
		match = func(n string) bool { return strings.EqualFold(n, name) || re.MatchString(n) }
	}
	var result []archiveEntry
	for _, d := range entries {
		if !match(d.Name()) {
			continue
		}
		s, e := d.Info()
		if e != nil {
			return nil, e
		}
		if d.Type()&os.ModeSymlink != 0 || !s.Mode().IsRegular() {
			return nil, errors.New("压缩分卷必须为普通文件，不能为链接")
		}
		result = append(result, archiveEntry{Source: filepath.Join(filepath.Dir(source), d.Name()), Size: s.Size(), Modified: s.ModTime()})
	}
	sort.Slice(result, func(i, j int) bool { return naturalLess(result[i].Source, result[j].Source) })
	return result, nil
}

func safeExtractName(name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(name, "/") || strings.Contains(name, ":") {
		return "", errors.New("压缩包含绝对路径或特殊路径，已阻止解压")
	}
	var parts []string
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." {
			continue
		}
		if part == ".." || !validName(part) {
			return "", errors.New("压缩包含不安全或无效的文件路径，已阻止解压")
		}
		parts = append(parts, part)
	}
	return filepath.Join(parts...), nil
}

func readExtractMembers(ctx context.Context, c Config, source string) ([]extractMember, int, error) {
	if has(strings.TrimSuffix(source, ".001"), ".rar") && strings.EqualFold(filepath.Base(c.SevenZip), "7za.exe") {
		return nil, 0, errors.New("RAR 需要完整版 7-Zip，请选择 7z.exe，并保留同目录的 7z.dll")
	}
	raw, e := runExtractCommand(ctx, c, source, "l", "-slt", "-sccUTF-8", "-p"+c.ExtractPassword, "-y", "-bd", "-spd", "--", source)
	if e != nil {
		if ctx.Err() != nil {
			return nil, 0, ctx.Err()
		}
		return nil, 0, e
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	at := strings.Index(text, "\n----------\n")
	if at < 0 {
		return nil, 0, errors.New("无法识别压缩包文件清单")
	}
	volumeCount := extractVolumeCount(text[:at])
	text = text[at+len("\n----------\n"):]
	var result []extractMember
	for _, block := range strings.Split(text, "\n\n") {
		f := map[string]string{}
		for _, line := range strings.Split(block, "\n") {
			if k, v, ok := strings.Cut(line, " = "); ok {
				f[k] = v
			}
		}
		name, ok := f["Path"]
		if !ok {
			continue
		}
		if c.ExtractPassword == "" && f["Encrypted"] == "+" {
			return nil, 0, errors.New("压缩包已加密，请填写解压密码后重新扫描")
		}
		if f["Symbolic Link"] != "" || f["Hard Link"] != "" || strings.Contains(f["Attributes"], "l") || f["Reparse Point"] == "+" {
			return nil, 0, errors.New("压缩包含链接，已阻止解压以避免写入目录外的文件")
		}
		name, e := safeExtractName(name)
		if e != nil {
			return nil, 0, e
		}
		if name == "" {
			continue
		}
		size, e := strconv.ParseInt(f["Size"], 10, 64)
		if e != nil || size < 0 {
			return nil, 0, errors.New("压缩包文件大小无效")
		}
		dir := f["Folder"] == "+" || strings.HasPrefix(f["Attributes"], "D") || strings.HasPrefix(f["Attributes"], "d")
		result = append(result, extractMember{name, size, dir})
	}
	return result, volumeCount, nil
}

func buildExtractPlan(ctx context.Context, c Config, log func(string)) (*Plan, error) {
	paths, e := scan(ctx, c)
	if e != nil {
		return nil, e
	}
	sort.Slice(paths, func(i, j int) bool { return naturalLess(paths[i], paths[j]) })
	p := &Plan{Config: c, Created: time.Now(), Items: []Item{}}
	for _, source := range paths {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		st, e := os.Lstat(source)
		if e != nil {
			return nil, e
		}
		if !st.Mode().IsRegular() {
			continue
		}
		name, ok := extractArchiveName(source)
		if !ok {
			continue
		}
		if unnamedExtractSplit(source) {
			format, err := extractSplitType(source)
			if err != nil {
				return nil, err
			}
			if format == "" {
				continue
			}
		}
		target := c.Output
		if c.ExtractLayout == "separate" {
			if !validName(name) {
				name = filepath.Base(source)
			}
			rel, _ := filepath.Rel(c.Root, filepath.Dir(source))
			target = filepath.Join(c.Output, rel, name)
		}
		it := Item{Source: source, Target: target, Modified: st.ModTime(), Note: "7-Zip 解压"}
		labels := map[string]string{"rename": "重名文件自动改名", "skip": "跳过重名文件", "overwrite": "覆盖同名文件"}
		it.Note += " · " + labels[c.ExtractConflict]
		if c.ExtractDeleteSource {
			if c.ExtractDeleteMode == "permanent" {
				it.Note += " · 完整解压且无跳过后永久删除原包及分卷"
			} else {
				it.Note += " · 完整解压且无跳过后将原包及分卷移入回收站"
			}
		} else {
			it.Note += " · 保留原压缩包"
		}
		job := extractJob{volumeIDs: map[string]fileIdentity{}}
		job.Volumes, e = extractVolumes(source)
		if e != nil {
			it.Conflict = e.Error()
		} else {
			for _, volume := range job.Volumes {
				var id fileIdentity
				id, e = snapshotFileIdentity(volume.Source)
				if e != nil {
					break
				}
				job.volumeIDs[volume.Source] = id
			}
			if e == nil {
				job.Members, job.VolumeCount, e = readExtractMembers(ctx, c, source)
			}
			if e != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				it.Conflict = e.Error()
			}
		}
		for _, v := range job.Volumes {
			it.Size += v.Size
		}
		files := 0
		var total int64
		for _, m := range job.Members {
			if !m.Dir {
				files++
				total += m.Size
			}
		}
		it.Note += fmt.Sprintf(" · %d 个文件 / %s", files, Human(total))
		p.Items = append(p.Items, it)
		p.extracts = append(p.extracts, job)
	}
	log(fmt.Sprintf("扫描完成：%d 个压缩包；分卷仅从首卷处理一次", len(p.Items)))
	return p, nil
}

func checkExtractVolumes(ctx context.Context, it Item, job extractJob) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	now, e := extractVolumes(it.Source)
	if e != nil {
		return e
	}
	if !sameArchiveEntries(now, job.Volumes) {
		return errors.New("压缩包或分卷已变化，请重新扫描预览")
	}
	for _, volume := range job.Volumes {
		if _, e := safePath(volume.Source); e != nil {
			return e
		}
		id, e := snapshotFileIdentity(volume.Source)
		old, ok := job.volumeIDs[volume.Source]
		if e != nil || !ok || !sameFileIdentity(id, old) {
			return errors.New("压缩包或分卷已被替换，请重新扫描预览")
		}
	}
	return nil
}

func extractCandidate(path string, n int, dir bool) string {
	if n == 0 {
		return path
	}
	ext := ""
	if !dir {
		ext = filepath.Ext(path)
	}
	return strings.TrimSuffix(path, ext) + fmt.Sprintf("_%d", n) + ext
}

func resolveExtractDir(parent, name, rule string) (string, bool, bool, error) {
	original := filepath.Join(parent, name)
	for n := 0; n < 100000; n++ {
		target := extractCandidate(original, n, true)
		if _, e := safePath(target); e != nil {
			return "", false, false, e
		}
		e := os.Mkdir(target, 0755)
		if e == nil {
			return target, true, false, nil
		}
		if !os.IsExist(e) {
			return "", false, false, e
		}
		st, e := os.Lstat(target)
		if e != nil {
			return "", false, false, e
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return "", false, false, errors.New("输出目录含链接，请更换输出位置")
		}
		if st.IsDir() {
			return target, false, false, nil
		}
		if rule == "skip" {
			return "", false, true, nil
		}
		if rule != "rename" {
			return "", false, false, errors.New("目录与已有文件重名，请改用自动重命名或更换输出位置")
		}
	}
	return "", false, false, errors.New("同名目录过多")
}

func extractRoot(output, target, rule string) (string, bool, error) {
	if _, e := safePath(target); e != nil {
		return "", false, e
	}
	if !within(output, target) {
		return "", false, errors.New("解压目标超出输出目录")
	}
	st, e := os.Lstat(output)
	if os.IsNotExist(e) {
		e = os.MkdirAll(output, 0755)
	} else if e == nil && (!st.IsDir() || st.Mode()&os.ModeSymlink != 0) {
		return "", false, errors.New("输出位置必须为普通目录")
	}
	if e != nil {
		return "", false, e
	}
	rel, _ := filepath.Rel(output, target)
	dir := output
	if rel != "." {
		for _, part := range strings.Split(rel, string(filepath.Separator)) {
			var skip bool
			dir, _, skip, e = resolveExtractDir(dir, part, rule)
			if e != nil || skip {
				return "", skip, e
			}
		}
	}
	return dir, false, nil
}

func replaceExtractFile(ctx context.Context, source, target string) (err error) {
	if _, e := safePath(target); e != nil {
		return e
	}
	in, e := os.Open(source)
	if e != nil {
		return e
	}
	defer in.Close()
	out, e := os.CreateTemp(filepath.Dir(target), ".extract-write-*")
	if e != nil {
		return e
	}
	temp := out.Name()
	defer os.Remove(temp)
	_, e = io.Copy(out, contextReader{ctx, in})
	ce := out.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	st, e := os.Stat(source)
	if e != nil {
		return e
	}
	if e = os.Chtimes(temp, st.ModTime(), st.ModTime()); e != nil {
		return e
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	// Never replace directories or links, even with overwrite explicitly selected.
	if st, e = os.Lstat(target); e == nil && (!st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0) {
		return errors.New("覆盖目标不是普通文件")
	} else if e != nil && !os.IsNotExist(e) {
		return e
	}
	return os.Rename(temp, target)
}

func publishExtractFile(ctx context.Context, source, target, rule string) (renamed, skipped bool, err error) {
	for n := 0; n < 100000; n++ {
		candidate := extractCandidate(target, n, false)
		if _, e := safePath(candidate); e != nil {
			return false, false, e
		}
		st, e := os.Lstat(candidate)
		if e == nil {
			if st.Mode()&os.ModeSymlink != 0 {
				return false, false, errors.New("输出文件位置含链接，请更换输出目录")
			}
			if rule == "skip" {
				return false, true, nil
			}
			if rule == "overwrite" {
				return false, false, replaceExtractFile(ctx, source, candidate)
			}
			continue
		}
		if !os.IsNotExist(e) {
			return false, false, e
		}
		if archiveCanLink(source) {
			e = publishArchiveVolume(ctx, source, candidate)
		} else {
			e = publishContext(ctx, source, candidate)
		}
		if os.IsExist(e) {
			if rule == "rename" {
				continue
			}
			n--
			continue
		}
		if e != nil {
			return false, false, e
		}
		st, e = os.Stat(source)
		if e == nil {
			e = os.Chtimes(candidate, st.ModTime(), st.ModTime())
		}
		if e == nil {
			e = copyArchiveAttributes(source, candidate)
		}
		return n > 0, false, e
	}
	return false, false, errors.New("同名文件过多")
}

func extractOne(ctx context.Context, c Config, it Item, job extractJob, log func(string)) (err error) {
	live := newExtractProgress(ctx, it.Source, log)
	if e := checkExtractVolumes(ctx, it, job); e != nil {
		return e
	}
	if e := os.MkdirAll(c.Output, 0755); e != nil {
		return e
	}
	stage, e := os.MkdirTemp(c.Output, ".extract-")
	if e != nil {
		return e
	}
	defer func() {
		if within(c.Output, stage) && filepath.Dir(stage) == c.Output {
			os.RemoveAll(stage)
		}
	}()
	payload := filepath.Join(stage, "payload")
	if e = os.Mkdir(payload, 0755); e != nil {
		return e
	}
	option := map[string]string{"rename": "-aou", "skip": "-aos", "overwrite": "-aoa"}[c.ExtractConflict]
	live.phase("extract", 0, 0)
	if _, e = runExtractCommandProgress(ctx, c, it.Source, live.percent, "x", "-y", "-bso0", "-bsp1", "-bse2", "-sccUTF-8", "-spd", option, "-p"+c.ExtractPassword, "-o"+payload, "--", it.Source); e != nil {
		return e
	}
	stagedFiles := 0
	var stagedBytes int64
	live.phase("validate", 0, 0)
	// Check the entire staged tree before publishing any file.
	e = filepath.WalkDir(payload, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("解压结果含链接，已阻止写入输出目录")
		}
		s, e := d.Info()
		if e != nil {
			return e
		}
		if s.Mode().IsRegular() {
			stagedFiles++
			stagedBytes = addExtractBytes(stagedBytes, s.Size())
			live.value.FilesDone = stagedFiles
			live.emit(false)
		}
		if !s.IsDir() && !s.Mode().IsRegular() {
			return errors.New("解压结果含特殊文件")
		}
		rel, _ := filepath.Rel(payload, path)
		if rel != "." {
			_, e = safeExtractName(rel)
		}
		return e
	})
	if e != nil {
		return e
	}
	if e = checkExtractVolumes(ctx, it, job); e != nil {
		return e
	}
	root, skip, e := extractRoot(c.Output, it.Target, c.ExtractConflict)
	if e != nil {
		return e
	}
	if skip {
		return errExtractSkipped
	}
	dirs := map[string]string{".": root}
	written, renamed, skipped := 0, 0, 0
	live.phase("publish", stagedFiles, stagedBytes)
	copyCtx := context.WithValue(ctx, copyProgressKey{}, live.copyBytes)
	defer func() {
		if err != nil && written > 0 && !errors.Is(err, errExtractDeleteSource) {
			log(fmt.Sprintf("当前压缩包已写出 %d 个文件，已完成的文件保留", written))
		}
	}()
	err = filepath.WalkDir(payload, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if path == payload {
			return nil
		}
		rel, _ := filepath.Rel(payload, path)
		parent := dirs[filepath.Dir(rel)]
		if d.IsDir() {
			target, _, skip, e := resolveExtractDir(parent, d.Name(), c.ExtractConflict)
			if e != nil {
				return e
			}
			if skip {
				skipped++
				return filepath.SkipDir
			}
			dirs[rel] = target
			return nil
		}
		before := live.value.BytesDone
		info, e := d.Info()
		if e != nil {
			return e
		}
		rename, skip, e := publishExtractFile(copyCtx, path, filepath.Join(parent, d.Name()), c.ExtractConflict)
		if e != nil {
			return e
		}
		if skip {
			skipped++
		} else {
			written++
			if rename {
				renamed++
			}
		}
		live.value.FilesDone++
		live.value.BytesDone = addExtractBytes(before, info.Size())
		live.writingPercent()
		live.emit(false)
		return nil
	})
	if err == nil {
		if written == 0 && skipped > 0 {
			return errExtractSkipped
		}
		log(fmt.Sprintf("解压完成：%s（写出 %d，自动改名 %d，跳过 %d）", root, written, renamed, skipped))
		if c.ExtractDeleteSource {
			expectedFiles := 0
			for _, member := range job.Members {
				if !member.Dir {
					expectedFiles++
				}
			}
			if skipped > 0 || stagedFiles != expectedFiles || written != stagedFiles {
				log("原压缩包已保留：存在跳过或未全部独立写出的内容")
			} else {
				live.phase("cleanup", 0, 0)
				if e := deleteExtractSources(ctx, c, it, job, log); e != nil {
					return e
				}
			}
		}
		live.value.Phase = "complete"
		live.value.Percent = 100
		live.value.PercentKnown = true
		live.emit(true)
	}
	return
}

func executeExtract(ctx context.Context, p *Plan, log func(string), progress func(int, int)) Result {
	result := Result{}
	if len(p.Items) != len(p.extracts) {
		log("解压计划已失效，请重新扫描预览")
		result.Failed = len(p.Items)
		return result
	}
	for i, it := range p.Items {
		if ctx.Err() != nil {
			log("解压已取消，已完成的文件保留")
			break
		}
		if it.Conflict != "" {
			result.Skipped++
			log("跳过：" + it.Source + " · " + it.Conflict)
			progress(i+1, len(p.Items))
			continue
		}
		log("正在使用 7-Zip 解压：" + it.Source)
		if e := checkItemSafety(p, it); e != nil {
			log("安全检查未通过：" + e.Error())
			result.Failed++
			break
		}
		itemCtx := context.WithValue(ctx, extractPositionKey{}, extractPosition{i + 1, len(p.Items)})
		if e := extractOne(itemCtx, p.Config, it, p.extracts[i], log); e != nil {
			if ctx.Err() != nil {
				log("解压已取消，已完成的文件保留")
				break
			}
			if errors.Is(e, errExtractSkipped) {
				result.Skipped++
				log("跳过：" + it.Source + " · 重名内容均已按规则跳过")
				progress(i+1, len(p.Items))
				continue
			}
			if errors.Is(e, errExtractDeleteSource) {
				result.Failed++
				log("原包清理未完成（解压结果已保留）：" + it.Source + " · " + e.Error())
				progress(i+1, len(p.Items))
				continue
			}
			result.Failed++
			message := e.Error()
			if p.Config.ExtractPassword != "" {
				message = strings.ReplaceAll(message, p.Config.ExtractPassword, "[密码已隐藏]")
			}
			log("解压失败：" + it.Source + " · " + message)
		} else {
			result.Success++
		}
		progress(i+1, len(p.Items))
	}
	return result
}
