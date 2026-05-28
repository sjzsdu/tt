package runtime

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

const EnvironmentContextKey = "env"

type EnvironmentContext struct {
	CWD           string         `json:"cwd"`
	InvocationCWD string         `json:"invocation_cwd,omitempty"`
	WorkspaceCWD  string         `json:"workspace_cwd,omitempty"`
	FormulaRunDir string         `json:"formula_run_dir,omitempty"`
	OS            EnvironmentOS  `json:"os"`
	Git           EnvironmentGit `json:"git"`
}

type EnvironmentOS struct {
	Name string `json:"name"`
	Arch string `json:"arch"`
}

type EnvironmentGit struct {
	IsRepo    bool   `json:"is_repo"`
	Root      string `json:"root,omitempty"`
	Repo      string `json:"repo,omitempty"`
	Branch    string `json:"branch,omitempty"`
	Commit    string `json:"commit,omitempty"`
	RemoteURL string `json:"remote_url,omitempty"`
}

func BuildEnvironmentContext(workspace string) EnvironmentContext {
	cwd := strings.TrimSpace(workspace)
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	}
	env := EnvironmentContext{
		CWD: cwd,
		OS:  EnvironmentOS{Name: goruntime.GOOS, Arch: goruntime.GOARCH},
	}
	env.Git = detectGitEnvironment(cwd)
	return env
}

func EnvironmentValue(workspace string) steps.Value {
	raw, _ := json.Marshal(BuildEnvironmentContext(workspace))
	return steps.Value{Type: "json", Raw: raw}
}

func (e *Executor) SeedEnvironment(workspace string) {
	e.seedEnvironment(workspace, "", "")
}

func (e *Executor) SeedWorkspaceEnvironment(workspace, invocationCWD, formulaRunDir string) {
	e.seedEnvironment(workspace, invocationCWD, formulaRunDir)
}

func (e *Executor) seedEnvironment(workspace, invocationCWD, formulaRunDir string) {
	if e == nil {
		return
	}
	if e.Context == nil {
		e.Context = NewContextStore()
	}
	env := BuildEnvironmentContext(workspace)
	if invocationCWD != "" {
		env.InvocationCWD = invocationCWD
	}
	if formulaRunDir != "" {
		env.FormulaRunDir = formulaRunDir
	}
	if env.InvocationCWD != "" && env.CWD != env.InvocationCWD {
		env.WorkspaceCWD = env.CWD
	}
	raw, _ := json.Marshal(env)
	_ = e.Context.Set(EnvironmentContextKey, steps.Value{Type: "json", Raw: raw})
}

func (e *Executor) SeedVars(vars map[string]string) {
	if e == nil || len(vars) == 0 {
		return
	}
	if e.Context == nil {
		e.Context = NewContextStore()
	}
	for name, value := range vars {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		raw, _ := json.Marshal(value)
		_ = e.Context.Set(name, steps.Value{Type: "json", Raw: raw})
	}
}

func (e *Executor) SeedWorkflowVars(workflow *ir.Workflow) {
	if e == nil || workflow == nil || len(workflow.Vars) == 0 {
		return
	}
	defaults := make(map[string]string, len(workflow.Vars))
	for name, schema := range workflow.Vars {
		if schema.Default != nil {
			defaults[name] = *schema.Default
		}
	}
	e.SeedVars(defaults)
}

func detectGitEnvironment(cwd string) EnvironmentGit {
	root := gitOutput(cwd, "rev-parse", "--show-toplevel")
	if root == "" {
		return EnvironmentGit{IsRepo: false}
	}
	branch := gitOutput(cwd, "branch", "--show-current")
	if branch == "" {
		branch = gitOutput(cwd, "rev-parse", "--abbrev-ref", "HEAD")
		if branch == "HEAD" {
			branch = ""
		}
	}
	return EnvironmentGit{
		IsRepo:    true,
		Root:      root,
		Repo:      filepath.Base(root),
		Branch:    branch,
		Commit:    gitOutput(cwd, "rev-parse", "--short", "HEAD"),
		RemoteURL: gitOutput(cwd, "config", "--get", "remote.origin.url"),
	}
}

func gitOutput(cwd string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
