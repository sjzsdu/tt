//go:build !windows

package beads

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts the command in its own process group so the whole tree
// can be terminated by killing the group.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessTree terminates the command and everything it spawned.
func killProcessTree(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
