//go:build windows

package process

import (
	"os/exec"
	"strconv"
)

func configureCommand(cmd *exec.Cmd) {}
func terminateOwned(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return terminateExternal(cmd.Process.Pid)
}
func terminateExternal(pid int) error {
	return exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
}
