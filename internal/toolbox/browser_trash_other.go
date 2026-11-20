//go:build !windows

package toolbox

import (
	"errors"
	"os/exec"
	"runtime"
)

func browserRecycle(paths []string) error {
	if runtime.GOOS == "darwin" {
		args := []string{"-e", `on run argv
tell application "Finder"
repeat with p in argv
delete (POSIX file (contents of p) as alias)
end repeat
end tell
end run`, "--"}
		return exec.Command("osascript", append(args, paths...)...).Run()
	}
	if program, e := exec.LookPath("gio"); e == nil {
		return exec.Command(program, append([]string{"trash", "--"}, paths...)...).Run()
	}
	if program, e := exec.LookPath("trash-put"); e == nil {
		return exec.Command(program, append([]string{"--"}, paths...)...).Run()
	}
	return errors.New("未找到系统废纸篓程序（gio 或 trash-put），原文件已保留；不会自动永久删除")
}
