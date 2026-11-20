package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"reflect"
	"runtime"
	"strings"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

type cliCommands struct{ Preview, Execute, Notice string }

func shellLiteral(v string) string { return "'" + strings.ReplaceAll(v, "'", "''") + "'" }

func buildCLICommands(c toolbox.Config, exe string) (cliCommands, error) {
	if !toolbox.ToolExists(c.Tool) {
		return cliCommands{}, errors.New("未知工具")
	}
	c = toolbox.ScopeConfig(c)
	if c.Tool != "dedupe" && strings.TrimSpace(c.Root) == "" {
		return cliCommands{}, errors.New("请先填写输入目录")
	}
	secretEnv := ""
	if c.ArchivePassword != "" {
		secretEnv = "TOOLBOX_ARCHIVE_PASSWORD"
	}
	if c.ExtractPassword != "" {
		secretEnv = "TOOLBOX_EXTRACT_PASSWORD"
	}
	c.ArchivePassword = ""
	c.ExtractPassword = ""
	base := toolbox.DefaultConfig()
	base.Tool = c.Tool
	base = toolbox.ScopeConfig(base)
	v, b := reflect.ValueOf(c), reflect.ValueOf(base)
	typ := v.Type()
	config := map[string]any{}
	for i := 0; i < v.NumField(); i++ {
		if !v.Field(i).IsZero() || !reflect.DeepEqual(v.Field(i).Interface(), b.Field(i).Interface()) {
			config[typ.Field(i).Name] = v.Field(i).Interface()
		}
	}
	data, e := json.MarshalIndent(config, "", "  ")
	if e != nil {
		return cliCommands{}, e
	}
	command := "& " + shellLiteral(exe) + " cli " + c.Tool + " --config -"
	if secretEnv != "" {
		command += " --password-env " + secretEnv
	}
	makeCommand := func(execute bool) string {
		run := command
		if execute {
			run += " --execute"
		}
		return "$toolboxEncoding = $OutputEncoding\ntry {\n  $OutputEncoding = [System.Text.UTF8Encoding]::new($false)\n@'\n" + string(data) + "\n'@ | " + run + "\n} finally { $OutputEncoding = $toolboxEncoding }"
	}
	notice := "PowerShell 命令。预览只扫描；执行命令会重新扫描，并按当前参数处理整个目录。保留工具箱的校验、回退与安全防护。"
	if runtime.GOOS != "windows" {
		command = posixLiteral(exe) + " cli " + c.Tool + " --config -"
		if secretEnv != "" {
			command += " --password-env " + secretEnv
		}
		makeCommand = func(execute bool) string {
			run := command
			if execute {
				run += " --execute"
			}
			return "printf '%s\\n' " + posixLiteral(string(data)) + " | " + run
		}
		notice = "终端命令（sh/bash/zsh）。预览只扫描；执行会重新扫描并处理整个目录，保留工具箱校验与安全防护。"
	}
	if secretEnv != "" {
		notice += "命令不包含密码，请先在终端设置环境变量 " + secretEnv + "。"
	}
	if c.Tool == "dedupe" {
		notice = "仅检测或打开外部去重软件，不会自动扫描或删除重复文件。"
	}
	return cliCommands{makeCommand(false), makeCommand(true), notice}, nil
}
func posixLiteral(v string) string { return "'" + strings.ReplaceAll(v, "'", `'"'"'`) + "'" }

func (s *server) cliCommandsAPI(w http.ResponseWriter, r *http.Request) {
	var c toolbox.Config
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	if e := json.NewDecoder(r.Body).Decode(&c); e != nil {
		fail(w, errors.New("命令参数无效"))
		return
	}
	exe, e := os.Executable()
	if e != nil {
		fail(w, e)
		return
	}
	commands, e := buildCLICommands(c, exe)
	if e != nil {
		fail(w, e)
		return
	}
	send(w, commands)
}
