package beads

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// DefaultTimeout is the default per-command timeout when none is configured.
const DefaultTimeout = 30 * time.Second

// Runner invokes the bd CLI with context-aware timeouts, workspace selection,
// executable discovery, and stderr preservation.
type Runner struct {
	// Bin is the resolved path to the bd executable.
	// When empty, it is discovered via exec.LookPath("bd").
	Bin string
	// Workspace is the directory containing .beads/.
	// When empty, it is auto-detected from the current working directory.
	Workspace string
	// Timeout is the per-command timeout. Zero uses DefaultTimeout.
	Timeout time.Duration
}

// NewRunner creates a Runner with the given workspace. If workspace is empty,
// it is auto-detected. The bd executable is discovered via PATH.
func NewRunner(workspace string) (*Runner, error) {
	bin, err := exec.LookPath("bd")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	if workspace == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("beads: getting working directory: %w", err)
		}
		workspace, err = findWorkspace(wd)
		if err != nil {
			return nil, err
		}
	} else {
		// Validate that the workspace contains .beads/.
		if !isBeadsWorkspace(workspace) {
			return nil, fmt.Errorf("%w: %s does not contain .beads/", ErrWorkspaceNotFound, workspace)
		}
	}
	return &Runner{Bin: bin, Workspace: workspace}, nil
}

// NewRunnerWithBin creates a Runner with an explicit binary path (useful for testing).
func NewRunnerWithBin(bin, workspace string) (*Runner, error) {
	if bin == "" {
		return nil, fmt.Errorf("%w: empty binary path", ErrNotFound)
	}
	return &Runner{Bin: bin, Workspace: workspace}, nil
}

// runResult holds the captured output of a bd command.
type runResult struct {
	Stdout   []byte
	Stderr   string
	ExitCode int
	Duration time.Duration
}

// run executes a bd subcommand with the given arguments.
// It applies the configured timeout and captures stdout/stderr separately.
// The command is run in its own process group so that the entire process tree
// can be killed on timeout or cancellation.
func (r *Runner) run(ctx context.Context, subcmd string, args ...string) (runResult, error) {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	fullArgs := append([]string{subcmd}, args...)
	cmd := exec.CommandContext(runCtx, r.Bin, fullArgs...)
	cmd.Dir = r.Workspace
	cmd.Env = append(os.Environ(), "BD_JSON_ENVELOPE=0")
	// Run in a new process group so we can kill the whole tree on timeout.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	started := time.Now()

	// Start the command and watch for context done to kill the process group.
	if err := cmd.Start(); err != nil {
		return runResult{}, err
	}

	// Monitor context cancellation to kill the entire process group.
	done := make(chan struct{})
	go func() {
		select {
		case <-runCtx.Done():
			// Kill the entire process group (negative pid).
			if cmd.Process != nil {
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
		case <-done:
		}
	}()

	runErr := cmd.Wait()
	close(done)
	duration := time.Since(started)

	result := runResult{
		Stdout:   stdout.Bytes(),
		Stderr:   strings.TrimSpace(stderr.String()),
		Duration: duration,
	}

	if runErr != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			return result, fmt.Errorf("%w: %s after %s", ErrTimeout, subcmd, timeout)
		}
		if ctx.Err() == context.Canceled {
			return result, fmt.Errorf("beads: %s cancelled: %w", subcmd, ctx.Err())
		}
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
			return result, &ExitError{
				Command:  subcmd,
				Stderr:   result.Stderr,
				ExitCode: exitErr.ExitCode(),
			}
		}
		return result, runErr
	}
	return result, nil
}

// runJSON executes a bd subcommand and decodes the JSON output into dst.
func (r *Runner) runJSON(ctx context.Context, dst any, subcmd string, args ...string) error {
	result, err := r.run(ctx, subcmd, args...)
	if err != nil {
		return err
	}
	if len(result.Stdout) == 0 {
		return nil
	}
	if err := json.Unmarshal(result.Stdout, dst); err != nil {
		return &DecodeError{Command: subcmd, Err: err}
	}
	return nil
}

// findWorkspace walks up from dir looking for a directory containing .beads/.
func findWorkspace(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("beads: resolving path: %w", err)
	}
	for {
		if isBeadsWorkspace(dir) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("%w: no .beads/ directory found in ancestors of %s", ErrWorkspaceNotFound, dir)
		}
		dir = parent
	}
}

// isBeadsWorkspace reports whether dir contains a .beads/ directory.
func isBeadsWorkspace(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".beads"))
	return err == nil && info.IsDir()
}
