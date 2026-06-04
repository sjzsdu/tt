package formulacmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sjzsdu/tt/internal/formula"
)

type formulaPreflightFailure struct {
	Check   string
	Message string
	Err     error
}

type formulaPreflightError struct {
	Failures []formulaPreflightFailure
}

func (e formulaPreflightError) Error() string {
	if len(e.Failures) == 0 {
		return "formula preflight failed"
	}
	var b strings.Builder
	b.WriteString("formula preflight failed:")
	for _, f := range e.Failures {
		b.WriteString("\n  - ")
		if f.Check != "" {
			b.WriteString(f.Check)
			b.WriteString(": ")
		}
		if strings.TrimSpace(f.Message) != "" {
			b.WriteString(strings.TrimSpace(f.Message))
		} else if f.Err != nil {
			b.WriteString(f.Err.Error())
		} else {
			b.WriteString("failed")
		}
		if f.Err != nil && strings.TrimSpace(f.Message) != "" {
			b.WriteString(" (")
			b.WriteString(f.Err.Error())
			b.WriteString(")")
		}
	}
	return b.String()
}

func runFormulaPreflight(ctx context.Context, f *formula.Formula, workspace string, vars map[string]string) error {
	if f == nil || f.Preflight == nil || len(f.Preflight.Checks) == 0 {
		return nil
	}
	if workspace == "" {
		workspace, _ = os.Getwd()
	}
	var failures []formulaPreflightFailure
	conditionVars := preflightConditionVars(f, vars)
	for i, check := range f.Preflight.Checks {
		if check == nil {
			continue
		}
		include, err := formula.EvaluateStepCondition(check.Condition, conditionVars)
		if err != nil {
			failures = append(failures, formulaPreflightFailure{Check: preflightCheckLabel(check, i), Message: check.Message, Err: fmt.Errorf("invalid preflight condition %q: %w", check.Condition, err)})
			continue
		}
		if !include {
			continue
		}
		if err := runFormulaPreflightCheck(ctx, check, workspace); err != nil {
			failures = append(failures, formulaPreflightFailure{Check: preflightCheckLabel(check, i), Message: check.Message, Err: err})
		}
	}
	if len(failures) > 0 {
		return formulaPreflightError{Failures: failures}
	}
	return nil
}

func preflightConditionVars(f *formula.Formula, overrides map[string]string) map[string]string {
	vars := make(map[string]string)
	if f != nil {
		for name, def := range f.Vars {
			if def != nil && def.Default != nil {
				vars[name] = *def.Default
			}
		}
	}
	for k, v := range overrides {
		vars[k] = v
	}
	return vars
}

func runFormulaPreflightCheck(ctx context.Context, check *formula.PreflightCheck, workspace string) error {
	switch strings.ToLower(strings.TrimSpace(check.Type)) {
	case "command":
		cmd := strings.TrimSpace(check.Command)
		if cmd == "" {
			return fmt.Errorf("command check requires command")
		}
		if _, err := exec.LookPath(cmd); err != nil {
			return fmt.Errorf("command %q not found in PATH", cmd)
		}
		return nil
	case "exec":
		cmd := strings.TrimSpace(check.Command)
		if cmd == "" {
			return fmt.Errorf("exec check requires command")
		}
		return runPreflightExec(ctx, workspace, cmd, check.Args)
	case "git":
		return runPreflightGit(ctx, workspace, check)
	case "env":
		name := strings.TrimSpace(check.Env)
		if name == "" {
			name = strings.TrimSpace(check.Name)
		}
		if name == "" {
			return fmt.Errorf("env check requires env or name")
		}
		if strings.TrimSpace(os.Getenv(name)) == "" {
			return fmt.Errorf("environment variable %q is not set", name)
		}
		return nil
	case "path":
		p := strings.TrimSpace(check.Path)
		if p == "" {
			return fmt.Errorf("path check requires path")
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(workspace, p)
		}
		if _, err := os.Stat(p); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unsupported preflight check type %q", check.Type)
	}
}

func runPreflightExec(ctx context.Context, workspace, command string, args []string) error {
	checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	if len(args) > 0 {
		cmd = exec.CommandContext(checkCtx, command, args...)
	} else {
		cmd = exec.CommandContext(checkCtx, "bash", "-lc", command)
	}
	cmd.Dir = workspace
	out, err := cmd.CombinedOutput()
	if errors.Is(checkCtx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("command timed out after 30s: %s", command)
	}
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("command failed: %s: %s", command, msg)
	}
	return nil
}

func runPreflightGit(ctx context.Context, workspace string, check *formula.PreflightCheck) error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("command %q not found in PATH", "git")
	}
	if check.RequireRepo {
		if err := runPreflightExec(ctx, workspace, "git rev-parse --is-inside-work-tree >/dev/null", nil); err != nil {
			return fmt.Errorf("current directory is not a git repository: %w", err)
		}
	}
	if check.RequireRemote {
		if err := runPreflightExec(ctx, workspace, "git remote get-url origin >/dev/null", nil); err != nil {
			return fmt.Errorf("git remote origin is not configured: %w", err)
		}
	}
	return nil
}

func preflightCheckLabel(check *formula.PreflightCheck, index int) string {
	if strings.TrimSpace(check.Name) != "" {
		return strings.TrimSpace(check.Name)
	}
	if strings.TrimSpace(check.Type) != "" {
		return fmt.Sprintf("%s[%d]", strings.TrimSpace(check.Type), index)
	}
	return fmt.Sprintf("check[%d]", index)
}
