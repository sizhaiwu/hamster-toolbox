//go:build !windows

package toolbox

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
