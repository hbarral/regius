//go:build !windows

package cli

import (
	"os/exec"
	"syscall"
)

// setSysProcAttr puts the child in its own process group so the whole tree
// (the app plus anything it spawns) can be signaled together, and so the
// terminal's Ctrl+C reaches `regius dev` first, which then shuts the app
// down gracefully.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// terminateProcess sends SIGTERM to the child's process group.
func terminateProcess(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
}

// killProcess sends SIGKILL to the child's process group.
func killProcess(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
