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

type archiveEntry struct {
	Source, Name string
	Size         int64
	Modified     time.Time
	Dir          bool
}
type archiveJob struct {
	extraIDs    map[string]fileIdentity
	Entries     []archiveEntry
	SourceCount int
}

func normalizeArchiveOptions(c *Config) error {
	if !c.Files && !c.Dirs {
		return errors.New("请至少选择文件夹或文件")
	}
	if c.ArchiveMode == "" {
		c.ArchiveMode = "pure"
	}
	if c.ArchiveMode != "pure" && c.ArchiveMode != "extras" {
		return errors.New("请选择有效的打包内容")
	}
	if c.ArchiveSplit == "" {
		c.ArchiveSplit = "script"
	}
	if c.ArchiveSplit != "none" && c.ArchiveSplit != "script" && c.ArchiveSplit != "uniform" {
		return errors.New("请选择有效的分卷方式")
	}
	if c.ArchiveVolume == "" {
		c.ArchiveVolume = "3g"
	}
	if c.ArchiveFirstVolume == "" {
		c.ArchiveFirstVolume = "10k"
	}
	for _, v := range []string{c.ArchiveFirstVolume, c.ArchiveVolume} {
		if !regexp.MustCompile(`(?i)^[1-9][0-9]{0,8}[bkmg]$`).MatchString(v) {
			return errors.New("分卷大小请填写如 10k、500m、3g（不含小数）")
		}
	}
	if c.ArchiveStart < 1 || c.ArchiveStart > 999999999 || c.ArchiveDigits < 1 || c.ArchiveDigits > 9 {
		return errors.New("起始编号须为 1–999999999，编号位数为 1–9")
	}
	if c.ArchiveLevel != 0 && c.ArchiveLevel != 1 && c.ArchiveLevel != 5 && c.ArchiveLevel != 9 {
		return errors.New("请选择有效的压缩强度")
	}
	for _, prefix := range []string{c.ArchiveDirPrefix, c.ArchiveFilePrefix} {
		if !validName(prefix + "001.7z") {
			return errors.New("编号前缀含有无效文件名字符")
		}
	}
	if strings.ContainsAny(c.ArchivePassword, "\x00\r\n") {
		return errors.New("密码不能含换行或空字符")
	}
	if c.ArchiveMode == "pure" {
		c.ArchiveExtras = ""
		c.ArchiveMarker = false
	}
	return nil
}

func archiveExtraPaths(c Config) ([]string, error) {
	var paths []string
	seen := map[string]bool{}
	for _, line := range strings.Split(c.ArchiveExtras, "\n") {
		line = strings.Trim(strings.TrimSpace(line), `"`)
		if line == "" {
			continue
		}
		p, e := filepath.Abs(line)
		if e != nil {
			return nil, e
		}
		if _, _, e := readArchiveExtra(p); e != nil {
			return nil, e
		}
		key := strings.ToLower(filepath.Base(p))
		if seen[key] || key == ".packing_date" && c.ArchiveMarker {
			return nil, errors.New("附加文件重名，请调整文件名")
		}
		seen[key] = true
		paths = append(paths, p)
	}
	return paths, nil
}

// Extra files are read-only inputs. System/profile write protection is not a
// reason to reject an explicitly selected file that Windows allows us to read.
func readArchiveExtra(path string) (archiveEntry, fileIdentity, error) {
	empty := archiveEntry{}
	if _, e := resolveNonLinkPath(path); e != nil {
		return empty, fileIdentity{}, fmt.Errorf("附加文件：%w", e)
	}
	st, e := os.Lstat(path)
	if e != nil {
		return empty, fileIdentity{}, fmt.Errorf("附加文件：%w", e)
	}
	if !st.Mode().IsRegular() {
		return empty, fileIdentity{}, errors.New("附加内容只能选择普通文件，不能为文件夹或链接")
	}
	if !validName(filepath.Base(path)) {
		return empty, fileIdentity{}, errors.New("附加文件名称不合法")
	}
	f, e := os.Open(path)
	if e != nil {
		return empty, fileIdentity{}, fmt.Errorf("无法读取附加文件（请检查文件权限）：%w", e)
	}
	defer f.Close()
	st, e = f.Stat()
	if e != nil {
		return empty, fileIdentity{}, e
	}
	if !st.Mode().IsRegular() {
		return empty, fileIdentity{}, errors.New("附加文件已变化，请重新选择普通文件")
	}
	id, e := snapshotFileIdentity(path)
	if e != nil {
		return empty, fileIdentity{}, e
	}
	return archiveEntry{Source: path, Name: filepath.Base(path), Size: st.Size(), Modified: st.ModTime()}, id, nil
}

func archiveSnapshot(ctx context.Context, source string) ([]archiveEntry, error) {
	var entries []archiveEntry
	e := filepath.WalkDir(source, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if unsafeEntry(p, d) {
			return fmt.Errorf("目录含链接，请移除链接后重试：%s", p)
		}
		s, e := d.Info()
		if e != nil {
			return e
		}
		if !s.IsDir() && !s.Mode().IsRegular() {
			return fmt.Errorf("不支持的特殊文件：%s", p)
		}
		rel, e := filepath.Rel(filepath.Dir(source), p)
		if e != nil {
			return e
		}
		entries = append(entries, archiveEntry{p, rel, s.Size(), s.ModTime(), s.IsDir()})
		return nil
	})
	return entries, e
}

func archiveBase(target string) string { return strings.TrimSuffix(target, ".001") }

func archiveOutputConflict(target string) error {
	base := archiveBase(target)
	entries, e := os.ReadDir(filepath.Dir(base))
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	name := strings.ToLower(filepath.Base(base))
	for _, d := range entries {
		n := strings.ToLower(d.Name())
		if n == name || strings.HasPrefix(n, name+".") {
			return errors.New("已有同名压缩包或分卷，请更换输出目录或起始编号")
		}
	}
	return nil
}

func buildArchivePlan(ctx context.Context, c Config, log func(string)) (*Plan, error) {
	extras, e := archiveExtraPaths(c)
	if e != nil {
		return nil, e
	}
	entries, e := os.ReadDir(c.Root)
	if e != nil {
		return nil, e
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return naturalLess(entries[i].Name(), entries[j].Name())
	})
	p := &Plan{Config: c, Created: time.Now(), Items: []Item{}}
	dirN, fileN := c.ArchiveStart, c.ArchiveStart
	targets := map[string]bool{}
	for _, d := range entries {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		name := d.Name()
		source := filepath.Join(c.Root, name)
		if unsafeEntry(source, d) {
			continue
		}
		if d.Type()&os.ModeSymlink != 0 || excluded(name) || strings.EqualFold(name, "0utput_7z") || strings.HasPrefix(name, ".packing-") || within(c.Output, source) {
			continue
		}
		if d.IsDir() && !c.Dirs || !d.IsDir() && !c.Files {
			continue
		}
		// Update scripts, previous archives and manifests are not resources.
		if !d.IsDir() && (has(name, ".bat .cmd .ps1 .7z .zip .rar") || regexp.MustCompile(`(?i)\.(7z|zip|rar)\.\d+$`).MatchString(name) || strings.HasPrefix(name, "资源清单-") || name == "目录.txt" || name == ".packing_date") {
			continue
		}
		n, prefix := fileN, c.ArchiveFilePrefix
		if d.IsDir() {
			n, prefix = dirN, c.ArchiveDirPrefix
			dirN++
		} else {
			fileN++
		}
		target := filepath.Join(c.Output, prefix+fmt.Sprintf("%0*d", c.ArchiveDigits, n)+".7z")
		if c.ArchiveSplit != "none" {
			target += ".001"
		}
		it := Item{Source: source, Target: target, Note: "每项独立打包 · 完成后校验 · 保留原资源"}
		if c.ArchiveSplit == "script" {
			it.Note += " · 首卷 " + c.ArchiveFirstVolume + "，后续每卷 " + c.ArchiveVolume
		} else if c.ArchiveSplit == "uniform" {
			it.Note += " · 每卷 " + c.ArchiveVolume
		}
		if c.ArchivePassword != "" {
			it.Note += " · 加密并隐藏文件名"
		}
		job := archiveJob{}
		if within(source, c.Output) {
			it.Conflict = "输出目录位于此资源内部，请换一个输出目录"
		} else {
			job.Entries, e = archiveSnapshot(ctx, source)
			if e != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				it.Conflict = e.Error()
			}
		}
		job.SourceCount = len(job.Entries)
		seen := map[string]bool{strings.ToLower(name): true}
		for _, extra := range extras {
			key := strings.ToLower(filepath.Base(extra))
			if seen[key] {
				it.Conflict = "资源名称与附加文件重名"
				break
			}
			seen[key] = true
			a, id, err := readArchiveExtra(extra)
			if err != nil {
				return nil, err
			}
			if job.extraIDs == nil {
				job.extraIDs = map[string]fileIdentity{}
			}
			job.extraIDs[extra] = id
			job.Entries = append(job.Entries, a)
		}
		if c.ArchiveMarker && seen[".packing_date"] {
			it.Conflict = "资源名与打包标记重名"
		}
		if len(extras) > 0 {
			it.Note += fmt.Sprintf(" · 附加 %d 个文件", len(extras))
		}
		if c.ArchiveMarker {
			it.Note += " · 加入本次打包标记"
		}
		for _, a := range job.Entries {
			if !a.Dir {
				it.Size += a.Size
			}
		}
		if e := archiveOutputConflict(target); e != nil {
			it.Conflict = e.Error()
		}
		key := strings.ToLower(target)
		if targets[key] {
			it.Conflict = "文件与文件夹的输出名称重复，请设置不同前缀"
		}
		targets[key] = true
		p.Items = append(p.Items, it)
		p.archives = append(p.archives, job)
	}
	// Flag both sides of duplicate output names.
	counts := map[string]int{}
	for _, it := range p.Items {
		counts[strings.ToLower(it.Target)]++
	}
	for i := range p.Items {
		if counts[strings.ToLower(p.Items[i].Target)] > 1 {
			p.Items[i].Conflict = "输出名称重复，请设置不同前缀"
		}
	}
	log(fmt.Sprintf("扫描完成：%d 项；文件夹与文件分别编号", len(p.Items)))
	return p, nil
}

func sameArchiveEntries(a, b []archiveEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Source != b[i].Source || a[i].Dir != b[i].Dir || (!a[i].Dir && (a[i].Size != b[i].Size || !a[i].Modified.Equal(b[i].Modified))) {
			return false
		}
	}
	return true
}

func checkArchiveSources(ctx context.Context, it Item, job archiveJob) error {
	now, e := archiveSnapshot(ctx, it.Source)
	if e != nil {
		return e
	}
	if !sameArchiveEntries(now, job.Entries[:job.SourceCount]) {
		return errors.New("资源内容已变化，请重新扫描预览")
	}
	for _, a := range job.Entries[job.SourceCount:] {
		now, id, e := readArchiveExtra(a.Source)
		if e != nil {
			return e
		}
		oldID, ok := job.extraIDs[a.Source]
		if !ok || !sameFileIdentity(id, oldID) || now.Size != a.Size || !now.Modified.Equal(a.Modified) {
			return errors.New("附加文件已变化，请重新扫描预览")
		}
	}
	return nil
}

func stageArchive(ctx context.Context, payload string, job archiveJob) error {
	for _, a := range job.Entries {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		target := filepath.Join(payload, a.Name)
		if !within(payload, target) {
			return errors.New("无效的归档路径")
		}
		if a.Dir {
			if e := os.MkdirAll(target, 0755); e != nil {
				return e
			}
			continue
		}
		if e := os.MkdirAll(filepath.Dir(target), 0755); e != nil {
			return e
		}
		_, readOnlyExtra := job.extraIDs[a.Source]
		if !readOnlyExtra && archiveCanLink(a.Source) {
			if e := os.Link(a.Source, target); e == nil {
				continue
			}
		}
		if e := copyArchiveFile(ctx, a, target); e != nil {
			return e
		}
	}
	// Creating children changes directory times; restore them after staging.
	// Never change timestamps or attributes on hard-linked source files.
	for i := len(job.Entries) - 1; i >= 0; i-- {
		a := job.Entries[i]
		if !a.Dir {
			continue
		}
		target := filepath.Join(payload, a.Name)
		if e := os.Chtimes(target, a.Modified, a.Modified); e != nil {
			return e
		}
		if e := copyArchiveAttributes(a.Source, target); e != nil {
			return e
		}
	}
	return nil
}

func copyArchiveFile(ctx context.Context, a archiveEntry, target string) error {
	in, e := os.Open(a.Source)
	if e != nil {
		return e
	}
	defer in.Close()
	out, e := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = io.Copy(out, contextReader{ctx, in})
	ce := out.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	if e = os.Chtimes(target, a.Modified, a.Modified); e != nil {
		return e
	}
	return copyArchiveAttributes(a.Source, target)
}

func archiveArgs(c Config, path string, verify bool) []string {
	args := []string{"a", "-t7z", "-mx=" + strconv.Itoa(c.ArchiveLevel), "-y", "-bd", "-bso0", "-bsp0", "-sccUTF-8", "-spd"}
	if verify {
		args = []string{"t", "-y", "-bd", "-bso0", "-bsp0", "-sccUTF-8"}
	}
	if c.ArchivePassword != "" {
		args = append(args, "-p"+c.ArchivePassword)
		if !verify {
			args = append(args, "-mhe=on")
		}
	}
	if !verify && c.ArchiveSplit != "none" {
		if c.ArchiveSplit == "script" {
			args = append(args, "-v"+c.ArchiveFirstVolume)
		}
		args = append(args, "-v"+c.ArchiveVolume)
	}
	args = append(args, "--", path)
	if !verify {
		args = append(args, ".")
	}
	return args
}

func packArchive(ctx context.Context, c Config, it Item, job archiveJob, markerTime time.Time, log func(string)) (published []string, err error) {
	if err = checkArchiveSources(ctx, it, job); err != nil {
		return
	}
	if err = archiveOutputConflict(it.Target); err != nil {
		return
	}
	stage, e := os.MkdirTemp(c.Output, ".packing-")
	if e != nil {
		return nil, e
	}
	defer func() {
		if within(c.Output, stage) && filepath.Dir(stage) == c.Output {
			if c.ArchiveMarker {
				clearArchiveMarkerAttributes(filepath.Join(stage, "payload", ".packing_date"))
			}
			os.RemoveAll(stage)
		}
	}()
	payload := filepath.Join(stage, "payload")
	if err = os.Mkdir(payload, 0755); err != nil {
		return
	}
	if err = stageArchive(ctx, payload, job); err != nil {
		return
	}
	if c.ArchiveMarker {
		marker := filepath.Join(payload, ".packing_date")
		err = os.WriteFile(marker, []byte("压缩时间："+markerTime.Format("2006/01/02 15:04:05")+"\r\n"), 0600)
		if err != nil {
			return
		}
		if err = os.Chtimes(marker, markerTime, markerTime); err != nil {
			return
		}
		if err = setArchiveMarkerAttributes(marker); err != nil {
			return
		}
	}
	base := filepath.Join(stage, filepath.Base(archiveBase(it.Target)))
	if _, err = runInDirectory(ctx, payload, c.SevenZip, archiveArgs(c, base, false)...); err != nil {
		return
	}
	first := base
	if c.ArchiveSplit != "none" {
		first += ".001"
	}
	log("正在校验：" + filepath.Base(it.Target))
	if _, err = run(ctx, c.SevenZip, archiveArgs(c, first, true)...); err != nil {
		return
	}
	if err = checkArchiveSources(ctx, it, job); err != nil {
		return
	}
	if err = archiveOutputConflict(it.Target); err != nil {
		return
	}
	entries, e := os.ReadDir(stage)
	if e != nil {
		return nil, e
	}
	defer func() {
		if err != nil {
			for _, path := range published {
				os.Remove(path)
			}
			published = nil
		}
	}()
	for _, d := range entries {
		if d.IsDir() {
			continue
		}
		name := d.Name()
		if name != filepath.Base(base) && !strings.HasPrefix(name, filepath.Base(base)+".") {
			continue
		}
		target := filepath.Join(c.Output, name)
		if err = publishArchiveVolume(ctx, filepath.Join(stage, name), target); err != nil {
			return
		}
		published = append(published, target)
	}
	if len(published) == 0 {
		err = errors.New("未生成有效压缩包")
	}
	return
}

// The stage is on the output volume. Linking publishes a verified volume
// atomically without copying gigabytes again; non-linking filesystems fall back
// to the existing exclusive, cancellable publisher.
func publishArchiveVolume(ctx context.Context, source, target string) error {
	if _, e := safePath(target); e != nil {
		return e
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if e := os.Link(source, target); e == nil {
		return nil
	}
	return publishContext(ctx, source, target)
}

// Retain the update scripts' fixed manifest name and append semantics.
func openArchiveManifest(output string) (*os.File, bool, error) {
	path := filepath.Join(output, "目录.txt")
	if _, e := safePath(path); e != nil {
		return nil, false, e
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY|os.O_APPEND, 0600)
	if e == nil {
		return f, true, nil
	}
	if !os.IsExist(e) {
		return nil, false, e
	}
	s, e := os.Lstat(path)
	if e != nil {
		return nil, false, e
	}
	if !s.Mode().IsRegular() {
		return nil, false, errors.New("目录.txt 必须是普通文件，不能是目录或链接")
	}
	f, e = os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
	return f, false, e
}

func executeArchive(ctx context.Context, p *Plan, log func(string), progress func(int, int)) Result {
	r := Result{}
	c := p.Config
	if len(p.archives) != len(p.Items) {
		log("打包计划无效，请重新扫描预览")
		r.Failed = len(p.Items)
		return r
	}
	if e := os.MkdirAll(c.Output, 0755); e != nil {
		log("无法创建输出目录：" + e.Error())
		r.Failed = len(p.Items)
		return r
	}
	markerTime := time.Now()
	var manifest *os.File
	manifestCreated := false
	defer func() {
		if manifest != nil {
			manifest.Close()
			if manifestCreated && r.Success == 0 {
				os.Remove(manifest.Name())
			}
		}
	}()
	for i, it := range p.Items {
		if ctx.Err() != nil {
			log("打包已取消，保留已完成的结果")
			break
		}
		if it.Conflict != "" {
			r.Skipped++
			log("跳过：" + it.Source + " · " + it.Conflict)
			progress(i+1, len(p.Items))
			continue
		}
		var e error
		if e = checkItemSafety(p, it); e != nil {
			log("安全检查未通过：" + e.Error())
			r.Failed++
			break
		}
		if manifest == nil {
			manifest, manifestCreated, e = openArchiveManifest(c.Output)
			if e != nil {
				log("无法生成名称清单：" + e.Error())
				r.Failed++
				break
			}
		}
		log("正在打包：" + it.Source)
		published, e := packArchive(ctx, c, it, p.archives[i], markerTime, log)
		if e == nil {
			kind := "文件"
			if p.archives[i].Entries[0].Dir {
				kind = "目录"
			}
			_, e = fmt.Fprintf(manifest, "%s %s：%s\r\n", filepath.Base(archiveBase(it.Target)), kind, filepath.Base(it.Source))
			if e == nil {
				e = manifest.Sync()
			}
			if e != nil {
				for _, path := range published {
					os.Remove(path)
				}
			}
		}
		if e != nil {
			if ctx.Err() != nil {
				log("当前打包已取消，原资源保留")
				break
			}
			message := e.Error()
			if c.ArchivePassword != "" {
				message = strings.ReplaceAll(message, c.ArchivePassword, "[密码已隐藏]")
			}
			r.Failed++
			log("打包失败：" + it.Source + " · " + message)
		} else {
			r.Success++
			log(fmt.Sprintf("打包完成并通过校验：%s（%d 个分卷/文件）", it.Target, len(published)))
		}
		progress(i+1, len(p.Items))
	}
	if manifest != nil && r.Success > 0 {
		log("资源名称清单：" + manifest.Name())
	}
	return r
}
