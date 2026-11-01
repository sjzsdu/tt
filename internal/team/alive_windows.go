//go:build windows

package team

import "syscall"

const (
	// PROCESS_QUERY_LIMITED_INFORMATION is the minimal access right needed by
	// GetExitCodeProcess on Vista and later.
	processQueryLimitedInformation = 0x1000
	stillActive                    = 259
)

// processAlive reports whether a process with the given pid exists. Windows
// has no signal-0 probe, so open the process and check it has not exited.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)
	var code uint32
	if err := syscall.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	return code == stillActive
}
