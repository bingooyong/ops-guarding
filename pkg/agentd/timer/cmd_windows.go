//go:build windows
// +build windows

package timer

import (
	"os/exec"
)

func CmdStart(cmd *exec.Cmd) error {
	return cmd.Start()
}

func CmdKill(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return errNoProcess
	}
	return cmd.Process.Kill()
}
