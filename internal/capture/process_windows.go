//go:build windows

package capture

import (
	"os/exec"
	"strconv"
)

func configureBrowserProcess(cmd *exec.Cmd) {}

func terminateBrowserProcess(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return exec.Command("taskkill", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F").Run()
}
