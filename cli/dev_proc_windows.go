//go:build windows

package cli

import (
	"os/exec"
	"strconv"
)

// setSysProcAttr is a no-op on Windows; taskkill /T handles process trees.
func setSysProcAttr(cmd *exec.Cmd) {}

// terminateProcess asks the child's process tree to shut down via taskkill
// (graceful: no /F flag).
func terminateProcess(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return exec.Command("taskkill", "/PID", strconv.Itoa(cmd.Process.Pid), "/T").Run()
}

// killProcess force-kills the child's process tree via taskkill /F.
func killProcess(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return exec.Command("taskkill", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F").Run()
}
