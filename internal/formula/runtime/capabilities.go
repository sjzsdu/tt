package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sjzsdu/tt/internal/formula/steps"
)

type ScriptCapability struct {
	AllowShell     bool
	DenyUnsafe     bool
	DefaultTimeout time.Duration
}

type scriptCapabilityOutput struct {
	Command    []string `json:"command"`
	Cwd        string   `json:"cwd,omitempty"`
	ExitCode   int      `json:"exit_code"`
	Stdout     string   `json:"stdout"`
	Stderr     string   `json:"stderr,omitempty"`
	DurationMS int64    `json:"duration_ms"`
}

func (c ScriptCapability) RunScript(ctx context.Context, req steps.ScriptRequest) (steps.Value, error) {
	argv := compactCommand(req.Command)
	if len(argv) == 0 {
		return steps.Value{}, fmt.Errorf("script command is required")
	}
	if c.DenyUnsafe {
		if err := ValidateScriptCommand(argv); err != nil {
			return steps.Value{}, err
		}
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = c.DefaultTimeout
	}
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, argv[0], argv[1:]...)
	if cwd := strings.TrimSpace(req.Cwd); cwd != "" {
		if !filepath.IsAbs(cwd) {
			if wd, err := os.Getwd(); err == nil {
				cwd = filepath.Join(wd, cwd)
			}
		}
		cmd.Dir = cwd
	}
	cmd.Env = os.Environ()
	for k, v := range req.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	started := time.Now()
	stdout, stderr := &strings.Builder{}, &strings.Builder{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		exitCode = 1
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		}
	}
	if runCtx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("script timed out after %s", timeout)
	}
	out := scriptCapabilityOutput{Command: argv, Cwd: cmd.Dir, ExitCode: exitCode, Stdout: stdout.String(), Stderr: stderr.String(), DurationMS: time.Since(started).Milliseconds()}
	data, marshalErr := json.Marshal(out)
	if marshalErr != nil {
		return steps.Value{}, marshalErr
	}
	return steps.Value{Type: "json", Raw: data}, err
}

func compactCommand(command []string) []string {
	out := make([]string, 0, len(command))
	for _, part := range command {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func ValidateScriptCommand(argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("script command is required")
	}
	base := filepath.Base(argv[0])
	dangerous := map[string]bool{"rm": true, "rmdir": true, "mkfs": true, "dd": true, "shutdown": true, "reboot": true, "halt": true, "poweroff": true, "sudo": true, "su": true, "chmod": true, "chown": true}
	if dangerous[base] {
		return fmt.Errorf("script command %q is denied by formula safety policy", base)
	}
	joined := strings.ToLower(strings.Join(argv, " "))
	patterns := []string{"rm -rf", ":(){", "> /dev/", "mkfs.", "curl | sh", "wget | sh"}
	for _, p := range patterns {
		if strings.Contains(joined, p) {
			return fmt.Errorf("script command contains denied pattern %q", p)
		}
	}
	return nil
}

type DryRunAgentCapability struct{}

func (DryRunAgentCapability) RunAgent(_ context.Context, req steps.AgentRequest) (steps.Value, error) {
	data, err := json.Marshal(map[string]string{"dry_run": "true", "agent": req.Agent, "model": req.Model, "prompt": req.Prompt})
	if err != nil {
		return steps.Value{}, err
	}
	return steps.Value{Type: "json", Raw: data}, nil
}

type DryRunScriptCapability struct{}

func (DryRunScriptCapability) RunScript(_ context.Context, req steps.ScriptRequest) (steps.Value, error) {
	data, err := json.Marshal(map[string]any{"dry_run": true, "command": req.Command, "cwd": req.Cwd, "env": req.Env})
	if err != nil {
		return steps.Value{}, err
	}
	return steps.Value{Type: "json", Raw: data}, nil
}
