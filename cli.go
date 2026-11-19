package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"reflect"
	"regexp"
	"strings"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

func cliMain(args []string) int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	return runCLI(ctx, args, os.Stdin, os.Stdout, os.Stderr)
}

var cliCapital = regexp.MustCompile(`([a-z0-9])([A-Z])`)

func cliFlagName(name string) string {
	return strings.ToLower(cliCapital.ReplaceAllString(name, "$1-$2"))
}

func cliFields(tool string) map[string]bool {
	fields := map[string]bool{}
	base := toolbox.Config{Tool: tool}
	if tool == "exif" {
		// Video metadata cleanup also accepts custom FFmpeg/FFprobe locations.
		base.MetadataTypes = "all"
	}
	probe := base
	v := reflect.ValueOf(&probe).Elem()
	typ := v.Type()
	for i := 0; i < v.NumField(); i++ {
		if typ.Field(i).Name == "Tool" {
			continue
		}
		v.Set(reflect.ValueOf(base))
		f := v.Field(i)
		switch f.Kind() {
		case reflect.String:
			f.SetString("probe")
		case reflect.Bool:
			f.SetBool(true)
		case reflect.Int:
			f.SetInt(1)
		case reflect.Slice:
			f.Set(reflect.ValueOf([]string{"probe"}))
		}
		if !reflect.ValueOf(toolbox.ScopeConfig(probe)).Field(i).IsZero() {
			fields[typ.Field(i).Name] = true
		}
	}
	return fields
}

func runCLI(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "用法：hamster-toolbox.exe cli <工具> --root <目录> [选项] [--execute]\n默认仅扫描，--execute 才执行；Ctrl+C 取消。\n工具：format rename number describe exif pdf image video thumbnail archive extract dedupe\n参数：--config <JSON文件或->、--password-env <环境变量名>；<工具> --help 查看全部参数。\n示例：hamster-toolbox.exe cli format --root E:\\media\n      hamster-toolbox.exe cli video --root E:\\videos --video-mode efficient --video-acceleration cpu --execute")
		return 0
	}
	tool := args[0]
	if !toolbox.ToolExists(tool) {
		fmt.Fprintln(stderr, "未知工具：", tool)
		return 2
	}
	c := toolbox.DefaultConfig()
	c.Tool = tool
	// Load configuration before parsing flags, so explicit flags always win.
	configPath := ""
	for i := 1; i < len(args); i++ {
		v := strings.TrimLeft(args[i], "-")
		if strings.HasPrefix(v, "config=") {
			configPath = strings.TrimPrefix(v, "config=")
		}
		if v == "config" && i+1 < len(args) {
			configPath = args[i+1]
			i++
		}
	}
	if configPath != "" {
		reader := stdin
		if configPath != "-" {
			f, e := os.Open(configPath)
			if e != nil {
				fmt.Fprintln(stderr, "无法读取配置文件")
				return 2
			}
			defer f.Close()
			reader = f
		}
		data, e := io.ReadAll(io.LimitReader(reader, 1024*1024+1))
		if e != nil || len(data) > 1024*1024 {
			fmt.Fprintln(stderr, "配置读取失败或超过 1 MB")
			return 2
		}
		decoder := json.NewDecoder(strings.NewReader(strings.TrimPrefix(string(data), "\ufeff")))
		decoder.DisallowUnknownFields()
		if e = decoder.Decode(&c); e != nil {
			fmt.Fprintln(stderr, "JSON 配置无效，请检查字段名称与类型")
			return 2
		}
		var tail any
		if decoder.Decode(&tail) != io.EOF {
			fmt.Fprintln(stderr, "配置只能包含一个 JSON 对象")
			return 2
		}
		if c.Tool != "" && c.Tool != tool {
			fmt.Fprintln(stderr, "配置中的 Tool 与命令工具不一致")
			return 2
		}
		c.Tool = tool
	}
	flags := flag.NewFlagSet("cli "+tool, flag.ContinueOnError)
	flags.SetOutput(stderr)
	execute := flags.Bool("execute", false, "执行本次扫描结果（默认仅预览）")
	passwordEnv := flags.String("password-env", "", "从指定环境变量读取压缩包密码，避免密码出现在命令中")
	flags.StringVar(&configPath, "config", configPath, "JSON 配置文件；- 表示从标准输入读取")
	v := reflect.ValueOf(&c).Elem()
	typ := v.Type()
	allowed := cliFields(tool)
	for i := 0; i < v.NumField(); i++ {
		name := typ.Field(i).Name
		if !allowed[name] || name == "ArchivePassword" || name == "ExtractPassword" {
			continue
		}
		f := v.Field(i)
		help := cliFieldHelp[name]
		switch f.Kind() {
		case reflect.String:
			flags.StringVar(f.Addr().Interface().(*string), cliFlagName(name), f.String(), help)
		case reflect.Bool:
			flags.BoolVar(f.Addr().Interface().(*bool), cliFlagName(name), f.Bool(), help+"（关闭用 =false）")
		case reflect.Int:
			flags.IntVar(f.Addr().Interface().(*int), cliFlagName(name), int(f.Int()), help)
		case reflect.Slice:
			flags.Func(cliFlagName(name), help, func(value string) error { c.ImageFormats = strings.Split(value, ","); return nil })
		}
	}
	if e := flags.Parse(args[1:]); e != nil {
		if errors.Is(e, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "存在多余参数；目录含空格时请加引号")
		return 2
	}
	if *passwordEnv != "" {
		password, ok := os.LookupEnv(*passwordEnv)
		if !ok || password == "" {
			fmt.Fprintln(stderr, "指定的密码环境变量未设置或为空")
			return 2
		}
		switch tool {
		case "archive":
			c.ArchivePassword = password
		case "extract":
			c.ExtractPassword = password
		default:
			fmt.Fprintln(stderr, "密码选项仅用于打包和解压")
			return 2
		}
	}
	log := func(message string) {
		for _, secret := range []string{c.ArchivePassword, c.ExtractPassword} {
			if secret != "" {
				message = strings.ReplaceAll(message, secret, "[密码已隐藏]")
			}
		}
		fmt.Fprintln(stderr, message)
	}
	if tool == "dedupe" {
		info := toolbox.InspectDedupeProgram(c.DedupeProgram)
		if !*execute {
			json.NewEncoder(stdout).Encode(info)
			return 0
		}
		cmd, e := toolbox.DedupeLaunchCommand(info.Path)
		if e == nil {
			e = cmd.Start()
		}
		if e != nil {
			log(e.Error())
			return 1
		}
		go cmd.Wait()
		fmt.Fprintln(stdout, `{"Opened":true}`)
		return 0
	}
	p, e := toolbox.BuildPlan(ctx, c, log)
	if e != nil {
		log(e.Error())
		if ctx.Err() != nil {
			return 130
		}
		return 1
	}
	if !*execute {
		p.Config.ArchivePassword = ""
		p.Config.ExtractPassword = ""
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if encoder.Encode(p) != nil {
			return 1
		}
		return 0
	}
	if len(p.Items) == 0 {
		json.NewEncoder(stdout).Encode(toolbox.Result{})
		return 0
	}
	if e = toolbox.CheckReady(p); e != nil {
		log(e.Error())
		return 1
	}
	r := toolbox.Execute(ctx, p, log, func(int, int) {})
	json.NewEncoder(stdout).Encode(r)
	if ctx.Err() != nil {
		return 130
	}
	if r.Failed > 0 {
		return 1
	}
	return 0
}
