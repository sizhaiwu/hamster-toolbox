package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

type scheduleLibraryField struct {
	Key, Label, Type, Placeholder, Default string
	Required                               bool
	Options                                []string
}
type scheduleLibraryTemplate struct {
	ID, Name, Category, Description, Effect string
	Fields                                  []scheduleLibraryField
}

func scheduleLibraryCatalog() []scheduleLibraryTemplate {
	root := scheduleLibraryField{Key: "root", Label: "资源目录（绝对路径）", Type: "text", Required: true, Placeholder: "粘贴要处理的文件夹完整路径"}
	output := scheduleLibraryField{Key: "output", Label: "输出目录（与资源目录分开）", Type: "text", Required: true, Placeholder: "粘贴已创建的输出文件夹完整路径"}
	mode := scheduleLibraryField{Key: "mode", Label: "处理方式", Type: "select", Default: "preview", Options: []string{"preview", "execute"}}
	return []scheduleLibraryTemplate{
		{ID: "system", Name: "查看系统信息", Category: "系统检查", Description: "输出当前时间、操作系统和工作目录，适合第一次测试计划任务。", Effect: "只读，不修改文件", Fields: []scheduleLibraryField{}},
		{ID: "disk", Name: "检查磁盘空间", Category: "系统检查", Description: "查看本机各磁盘的使用量与可用空间。", Effect: "只读，不清理磁盘", Fields: []scheduleLibraryField{}},
		{ID: "inventory", Name: "列出目录文件", Category: "文件与资源", Description: "递归列出指定目录里的文件，方便定期查看资源。输出超过日志上限时只保留前 16 KB。", Effect: "只读，不改名、不删除", Fields: []scheduleLibraryField{root}},
		{ID: "large", Name: "查找大文件", Category: "文件与资源", Description: "列出超过指定大小的文件，仅生成报告，由你决定后续处理。", Effect: "只读，不删除文件", Fields: []scheduleLibraryField{root, {Key: "minMB", Label: "大小超过（MB）", Type: "number", Default: "500", Required: true}}},
		{ID: "old", Name: "查找长期未修改的文件", Category: "文件与资源", Description: "列出指定天数前修改的文件，不自动清理。不同系统的日期边界可能相差一天。", Effect: "只读，不删除文件", Fields: []scheduleLibraryField{root, {Key: "days", Label: "超过多少天未修改", Type: "number", Default: "30", Required: true}}},
		{ID: "hash", Name: "计算文件 SHA256", Category: "文件与资源", Description: "计算指定文件的校验值，用来核对文件是否变化。", Effect: "只读，不修改文件", Fields: []scheduleLibraryField{{Key: "file", Label: "文件完整路径", Type: "text", Required: true}}},
		{ID: "backup", Name: "备份本地文件夹", Category: "文件与资源", Description: "在目标目录内创建带时间编号的新副本，不覆盖源文件，也不自动删除旧备份；请预留磁盘空间。", Effect: "会新增备份文件，不删除原件", Fields: []scheduleLibraryField{root, output}},
		{ID: "get", Name: "定时访问链接（GET）", Category: "网络请求", Description: "定期检查自己的服务或访问更新接口，HTTP 2xx 为成功。", Effect: "会向填写的地址发送请求", Fields: []scheduleLibraryField{{Key: "url", Label: "请求地址", Type: "url", Placeholder: "https://example.com/health", Required: true}}},
		{ID: "post", Name: "发送 JSON / Webhook（POST）", Category: "网络请求", Description: "向自己的接口发送 JSON。对方接口可能产生通知或其他操作，请先核对地址和内容。", Effect: "会发送 POST 请求，可能触发远端操作", Fields: []scheduleLibraryField{{Key: "url", Label: "请求地址", Type: "url", Placeholder: "https://example.com/api", Required: true}, {Key: "body", Label: "JSON 请求体", Type: "textarea", Default: `{"source":"hamster-toolbox","action":"check"}`, Required: true}}},
		{ID: "format", Name: "定时更新目录统计名称", Category: "工具箱自动化", Description: "调用工具箱 CLI 统计资源数量和大小。默认仅扫描预览，改为执行后会直接更新子目录名称。", Effect: "预览只读；执行会修改目录名", Fields: []scheduleLibraryField{root, mode}},
		{ID: "image", Name: "定时压缩 JPG 图片", Category: "工具箱自动化", Description: "调用图片压缩工具，只处理 JPG / JPEG 类图片，质量 75，写到独立输出目录。", Effect: "预览只读；执行会写入压缩结果，保留源图", Fields: []scheduleLibraryField{root, output, mode}},
		{ID: "video", Name: "定时压缩视频", Category: "工具箱自动化", Description: "使用省空间 H.265 模式和自动加速，默认只处理 100 MB、1000 kbps 以上的视频；可在生成的脚本中调整参数。", Effect: "预览只读；执行占用编码资源，结果写到独立目录", Fields: []scheduleLibraryField{root, output, mode}},
		{ID: "thumbnail", Name: "定时生成视频预览图", Category: "工具箱自动化", Description: "为视频生成 5×5 拼图，宽度 1024，写到独立输出目录。", Effect: "预览只读；执行会新增预览图，保留视频", Fields: []scheduleLibraryField{root, output, mode}},
	}
}
func libraryExistingPath(value string, directory bool) (string, error) {
	if strings.ContainsRune(value, 0) || strings.ContainsAny(value, "\r\n") || len(value) > 4096 || !filepath.IsAbs(value) {
		return "", errors.New("请填写有效的绝对路径，不支持换行")
	}
	p, e := filepath.EvalSymlinks(filepath.Clean(value))
	if e != nil {
		return "", errors.New("路径不存在或无法访问")
	}
	st, e := os.Stat(p)
	if e != nil || directory && !st.IsDir() || !directory && !st.Mode().IsRegular() {
		return "", errors.New("请选择现有目录或普通文件")
	}
	return p, nil
}
func libraryPathsOverlap(a, b string) bool {
	if runtime.GOOS == "windows" {
		a = strings.ToLower(a)
		b = strings.ToLower(b)
	}
	inside := func(root, path string) bool {
		rel, e := filepath.Rel(root, path)
		return e == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
	}
	return inside(a, b) || inside(b, a)
}
func libraryNumber(p map[string]string, key string, min, max int) (int, error) {
	n, e := strconv.Atoi(p[key])
	if e != nil || n < min || n > max {
		return 0, fmt.Errorf("数值需为 %d～%d", min, max)
	}
	return n, nil
}
func scheduleCLIExecutable() (string, error) {
	current, e := os.Executable()
	if e != nil {
		return "", e
	}
	name := "hamster-toolbox"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	candidate := filepath.Join(filepath.Dir(current), name)
	if st, e := os.Stat(candidate); e == nil && !st.IsDir() {
		return candidate, nil
	}
	if !strings.Contains(strings.ToLower(filepath.Base(current)), "desktop") {
		return current, nil
	}
	return "", errors.New("找不到同目录的命令行程序 hamster-toolbox，请使用完整工具箱目录")
}
func buildLibraryJob(id string, params map[string]string, platform, exe string) (scheduledJob, error) {
	var spec *scheduleLibraryTemplate
	for _, t := range scheduleLibraryCatalog() {
		if t.ID == id {
			copy := t
			spec = &copy
			break
		}
	}
	if spec == nil {
		return scheduledJob{}, errors.New("模板不存在")
	}
	if len(params) > 16 {
		return scheduledJob{}, errors.New("模板参数过多")
	}
	known := map[string]bool{}
	for _, field := range spec.Fields {
		known[field.Key] = true
	}
	for key := range params {
		if !known[key] {
			return scheduledJob{}, errors.New("模板包含不支持的参数")
		}
	}
	p := map[string]string{}
	for _, field := range spec.Fields {
		v := params[field.Key]
		if v == "" {
			v = field.Default
		}
		if len(v) > 65536 {
			return scheduledJob{}, errors.New("模板参数过长")
		}
		if field.Required && strings.TrimSpace(v) == "" {
			return scheduledJob{}, fmt.Errorf("请填写%s", field.Label)
		}
		p[field.Key] = v
	}
	j := scheduledJob{Name: spec.Name, Kind: "script", Shell: "sh", Period: "day", AtHour: 9, Timeout: 600, Enabled: false}
	windows := platform == "windows"
	quote := posixLiteral
	if windows {
		quote = shellLiteral
		j.Shell = "powershell"
	}
	root, output, file := "", "", ""
	var e error
	if p["root"] != "" {
		root, e = libraryExistingPath(p["root"], true)
		if e != nil {
			return j, fmt.Errorf("资源目录：%w", e)
		}
	}
	if p["output"] != "" {
		output, e = libraryExistingPath(p["output"], true)
		if e != nil {
			return j, fmt.Errorf("输出目录：%w", e)
		}
		if libraryPathsOverlap(root, output) {
			return j, errors.New("资源目录与输出目录不能相同或互相包含，请选择分开的目录")
		}
	}
	if p["file"] != "" {
		file, e = libraryExistingPath(p["file"], false)
		if e != nil {
			return j, e
		}
	}
	switch id {
	case "system":
		j.Script = "date\nuname -s\npwd"
		if windows {
			j.Script = "Get-Date\n[System.Environment]::OSVersion.VersionString\nGet-Location"
		}
	case "disk":
		j.Script = "df -h"
		if windows {
			j.Script = "Get-PSDrive -PSProvider FileSystem | Select-Object Name, Used, Free"
		}
	case "inventory":
		j.Script = "find " + quote(root) + " -type f -print"
		if windows {
			j.Script = "Get-ChildItem -LiteralPath " + quote(root) + " -File -Recurse -ErrorAction Stop | Select-Object -ExpandProperty FullName"
		}
	case "large":
		n, e := libraryNumber(p, "minMB", 1, 1048576)
		if e != nil {
			return j, e
		}
		j.Script = "find " + quote(root) + " -type f -size +" + strconv.FormatInt(int64(n)*1024*1024, 10) + "c -print"
		if windows {
			j.Script = "Get-ChildItem -LiteralPath " + quote(root) + " -File -Recurse -ErrorAction Stop | Where-Object { $_.Length -gt " + strconv.FormatInt(int64(n)*1024*1024, 10) + " } | Sort-Object Length -Descending | Select-Object -First 100 FullName, Length"
		}
	case "old":
		n, e := libraryNumber(p, "days", 1, 36500)
		if e != nil {
			return j, e
		}
		j.Script = "find " + quote(root) + " -type f -mtime +" + strconv.Itoa(n) + " -print"
		if windows {
			j.Script = "$cutoff = (Get-Date).AddDays(-" + strconv.Itoa(n) + ")\nGet-ChildItem -LiteralPath " + quote(root) + " -File -Recurse -ErrorAction Stop | Where-Object { $_.LastWriteTime -lt $cutoff } | Select-Object -First 100 FullName, LastWriteTime"
		}
	case "hash":
		j.Script = "sha256sum " + quote(file)
		if platform == "darwin" {
			j.Script = "shasum -a 256 " + quote(file)
		}
		if windows {
			j.Script = "$stream = [System.IO.File]::OpenRead(" + quote(file) + ")\ntry {\n  $sha = [System.Security.Cryptography.SHA256]::Create()\n  try { [System.BitConverter]::ToString($sha.ComputeHash($stream)).Replace('-','').ToLowerInvariant() } finally { $sha.Dispose() }\n} finally { $stream.Dispose() }"
		}
	case "backup":
		j.ReadPaths = []string{root}
		j.WritePaths = []string{output}
		j.Timeout = 86400
		j.AtHour = 2
		j.Script = "set -eu\nsource=" + quote(root) + "\ndestination=" + quote(output) + "\nbackup=\"$destination/backup-$(date +%Y%m%d-%H%M%S)-$$\"\nif [ -e \"$backup\" ]; then printf '%s\\n' 'Backup already exists' >&2; exit 1; fi\ncp -R \"$source\" \"$backup\"\nprintf '%s\\n' \"$backup\""
		if windows {
			j.Script = "$source = " + quote(root) + "\n$destination = " + quote(output) + "\n$backup = Join-Path $destination ('backup-' + (Get-Date -Format 'yyyyMMdd-HHmmss') + '-' + [guid]::NewGuid().ToString('N'))\nif (Test-Path -LiteralPath $backup) { throw '备份目录已存在' }\nCopy-Item -LiteralPath $source -Destination $backup -Recurse -ErrorAction Stop\nWrite-Output $backup"
		}
	case "get", "post":
		j.Kind = "http"
		j.Shell = ""
		j.URL = p["url"]
		j.Method = "GET"
		if id == "post" {
			if !json.Valid([]byte(p["body"])) {
				return j, errors.New("请求体必须是合法 JSON")
			}
			j.Method = "POST"
			j.Body = p["body"]
			j.Headers = map[string]string{"Content-Type": "application/json"}
		}
	case "format", "image", "video", "thumbnail":
		if exe == "" {
			return j, errors.New("找不到工具箱命令行程序")
		}
		if p["mode"] != "preview" && p["mode"] != "execute" {
			return j, errors.New("请选择预览或执行")
		}
		c := toolbox.DefaultConfig()
		c.Tool = id
		c.Root = root
		c.Output = output
		c.Workers = 1
		if id == "image" {
			c.ImageFormats = []string{"jpg", "jpeg", "jpe", "jfif"}
			c.Quality = 75
		}
		if id == "video" {
			c.VideoMode = "efficient"
			c.VideoAcceleration = "auto"
		}
		commands, e := buildCLICommands(c, exe)
		if e != nil {
			return j, e
		}
		j.Script = commands.Preview
		if p["mode"] == "execute" {
			j.Script = commands.Execute
		}
		if windows {
			j.Script += "\nif ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }"
		} else {
			j.Script += "\nexit $?"
		}
		j.Timeout = 86400
		j.AtHour = 3
	}
	return j, nil
}
func (s *server) scheduleLibraryAPI(w http.ResponseWriter, action, id string, params map[string]string) {
	if action == "library" {
		send(w, map[string]any{"Templates": scheduleLibraryCatalog(), "Platform": runtime.GOOS})
		return
	}
	exe, e := scheduleCLIExecutable()
	if e != nil && (id == "format" || id == "image" || id == "video" || id == "thumbnail") {
		fail(w, e)
		return
	}
	j, e := buildLibraryJob(id, params, runtime.GOOS, exe)
	if e == nil {
		e = validateScheduledJob(&j)
	}
	if e != nil {
		fail(w, e)
		return
	}
	send(w, map[string]any{"Job": j})
}
