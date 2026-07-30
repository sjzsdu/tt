package beads

import (
	"errors"
	"fmt"
)

// ErrNotFound is returned when the bd executable cannot be found on PATH.
var ErrNotFound = errors.New("beads: bd executable not found")

// ErrWorkspaceNotFound is returned when no .beads workspace directory is found.
var ErrWorkspaceNotFound = errors.New("beads: workspace not found")

// ErrUnsupported is returned when the installed bd version does not support a command.
var ErrUnsupported = errors.New("beads: command not supported by installed bd")

// ErrTimeout is returned when a bd command exceeds the configured timeout.
var ErrTimeout = errors.New("beads: command timed out")

// ExitError records a non-zero exit from bd along with captured stderr.
type ExitError struct {
	Command string // the bd subcommand (e.g. "list", "show")
	Stderr  string // captured stderr output
	ExitCode int   // process exit code
}

func (e *ExitError) Error() string {
	msg := fmt.Sprintf("beads: %s exited with code %d", e.Command, e.ExitCode)
	if e.Stderr != "" {
		msg += ": " + e.Stderr
	}
	return msg
}

// Unwrap returns nil; ExitError is a leaf error but implements the interface
// so callers can use errors.As to inspect it.

// DecodeError records a JSON decoding failure from bd output.
type DecodeError struct {
	Command string // the bd subcommand that produced the output
	Err     error  // underlying decoding error
}

func (e *DecodeError) Error() string {
	return fmt.Sprintf("beads: decoding %s output: %v", e.Command, e.Err)
}

func (e *DecodeError) Unwrap() error {
	return e.Err
}
