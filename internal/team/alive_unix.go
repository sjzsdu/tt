//go:build !windows

package team

import (
	"errors"
	"syscall"
)

// processAlive reports whether a process with the given pid exists.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}
