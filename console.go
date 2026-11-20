package main

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
)

type startupConsole struct {
	out   io.Writer
	color bool
}

func (c startupConsole) paint(code, text string) string {
	if c.color {
		return "\x1b[" + code + "m" + text + "\x1b[0m"
	}
	return text
}

// Paths and operating-system errors must not become terminal control sequences.
func consoleText(text string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || r >= 0x80 && r <= 0x9f {
			return ' '
		}
		return r
	}, text)
}

func (c startupConsole) header() {
	fmt.Fprintln(c.out)
	fmt.Fprintln(c.out, "  "+c.paint("1;36", "仓鼠工具箱")+"  "+consoleText(appVersion))
	fmt.Fprintf(c.out, "  Hamster Toolbox · %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintln(c.out, "  "+c.paint("90", strings.Repeat("─", 56)))
}

func (c startupConsole) status(code, label, message string) {
	fmt.Fprintf(c.out, "  %s %s\n", c.paint(code, "["+label+"]"), consoleText(message))
}

func (c startupConsole) ready(address, dataDir, browserMessage string, browserErr, sharingErr error, b *fileBrowser) {
	c.header()
	c.status("32", "就绪", "本机服务已启动")
	if browserErr != nil {
		c.status("33", "浏览器", "未能自动打开，请复制下方链接到浏览器")
		fmt.Fprintln(c.out, "           "+consoleText(browserErr.Error()))
	} else {
		c.status("36", "浏览器", browserMessage)
	}
	if sharingErr != nil {
		c.status("33", "共享", "未开启 · "+sharingErr.Error())
	} else if b != nil {
		b.mu.Lock()
		running, port, roots, unavailable := b.server != nil, b.config.Port, len(b.scopes), len(b.unavailable)
		b.mu.Unlock()
		if running {
			c.status("32", "共享", fmt.Sprintf("已开启 · 端口 %d · %d 个可用目录", port, roots))
			if unavailable > 0 {
				c.status("33", "提示", fmt.Sprintf("%d 个共享目录暂不可用，请在页面中检查", unavailable))
			}
		} else {
			c.status("90", "共享", "未开启 · 可在资源管理器中设置")
		}
	}
	fmt.Fprintln(c.out)
	fmt.Fprintln(c.out, "  "+c.paint("1", "访问工具箱"))
	// Keep the complete session URL on its own line for terminal link detection.
	fmt.Fprintln(c.out, "  "+address)
	fmt.Fprintln(c.out, "  仅供本机访问；其他设备请使用内网共享地址。")
	fmt.Fprintln(c.out)
	fmt.Fprintln(c.out, "  "+c.paint("1", "设置与任务记录"))
	fmt.Fprintln(c.out, "  "+consoleText(dataDir))
	fmt.Fprintln(c.out)
	fmt.Fprintln(c.out, "  "+c.paint("90", strings.Repeat("─", 56)))
	fmt.Fprintln(c.out, "  保持此窗口运行，关闭浏览器页面不会退出。")
	fmt.Fprintln(c.out, "  按 Ctrl+C 或关闭此窗口退出，未完成的任务会中断。")
	fmt.Fprintln(c.out)
}

func consoleColorAllowed() bool {
	_, disabled := os.LookupEnv("NO_COLOR")
	return !disabled && os.Getenv("TERM") != "dumb"
}

func (c startupConsole) listenFailure(port int, err error) {
	c.header()
	message := "无法开启本机服务"
	switch {
	case port < 0 || port > 65535:
		message = "端口必须在 0–65535 之间，0 表示自动选择"
	case consoleAddressInUse(err):
		message = fmt.Sprintf("端口 %d 已被占用，请检查是否已经启动了工具箱", port)
	case os.IsPermission(err):
		message = fmt.Sprintf("系统不允许使用端口 %d，请尝试其他端口", port)
	}
	c.status("31", "启动失败", message)
	command := "./hamster-toolbox -port 8790"
	if runtime.GOOS == "windows" {
		command = "start-toolbox.cmd -port 8790"
	}
	fmt.Fprintln(c.out, "  可使用 "+command+" 指定其他端口。")
	fmt.Fprintln(c.out, "  详细信息："+consoleText(err.Error()))
	fmt.Fprintln(c.out)
}
