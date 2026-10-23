//go:build !windows
// +build !windows

package timer

import (
	"os/exec"
	"syscall"
)

func CmdStart(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd.Start()
}

func CmdKill(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return errNoProcess
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
