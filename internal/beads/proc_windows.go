//go:build windows

package beads

import (
	"os/exec"
	"strconv"
	"syscall"
)

// setProcessGroup puts the command in its own process group.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// killProcessTree terminates the command and everything it spawned; taskkill
// with /T walks the child process tree the way a Unix negative-pid kill does.
func killProcessTree(pid int) {
	_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
}
