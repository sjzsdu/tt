package executor

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sjzsdu/tt/internal/formula"
)

func TestExecutorSkipsInitialCompletedResults(t *testing.T) {
	recipe := &formula.Recipe{
		Name: "resume-demo",
		Steps: []formula.RecipeStep{
			{ID: "resume-demo", Title: "Root", IsRoot: true},
			{ID: "resume-demo.first", Title: "First", OutputKey: "first_out"},
			{ID: "resume-demo.second", Title: "Second", InputCtx: []string{"first_out"}},
		},
		Deps: []formula.RecipeDep{
			{StepID: "resume-demo.first", DependsOnID: "resume-demo", Type: "parent-child"},
			{StepID: "resume-demo.second", DependsOnID: "resume-demo", Type: "parent-child"},
			{StepID: "resume-demo.second", DependsOnID: "resume-demo.first", Type: "blocks"},
		},
	}
	exec := New(recipe, RunOptions{
		InitialContext: map[string]string{"first_out": "saved output"},
		InitialResults: []StepResult{{StepID: "resume-demo.first", Title: "First", Status: StatusCompleted, Output: "saved output"}},
	})
	called := []string{}
	result, err := exec.Run(context.Background(), func(ctx context.Context, step *formula.RecipeStep, prompt string) (string, error) {
		called = append(called, step.ID)
		if step.ID == "resume-demo.first" {
			t.Fatalf("completed step should not rerun")
		}
		if step.ID == "resume-demo.second" && !strings.Contains(prompt, "saved output") {
			t.Fatalf("resume context missing from prompt: %s", prompt)
		}
		return "second output", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(called) != 1 || called[0] != "resume-demo.second" {
		t.Fatalf("called steps = %v", called)
	}
	if result.Completed != 3 {
		t.Fatalf("completed = %d, want 3", result.Completed)
	}
}

func TestExecutorRunsScriptStepAndCapturesJSON(t *testing.T) {
	recipe := &formula.Recipe{
		Name: "script-demo",
		Steps: []formula.RecipeStep{
			{ID: "script-demo", Title: "Root", IsRoot: true},
			{ID: "script-demo.fetch", Title: "Fetch", Execution: "script", OutputKey: "data", Script: &formula.ScriptSpec{Command: []string{"printf", `{"ok":true}`}, Format: "json"}},
		},
		Deps: []formula.RecipeDep{{StepID: "script-demo.fetch", DependsOnID: "script-demo", Type: "parent-child"}},
	}
	result, err := New(recipe, RunOptions{AllowScripts: true}).Run(context.Background(), func(ctx context.Context, step *formula.RecipeStep, prompt string) (string, error) {
		t.Fatalf("agent runner should not be called for script steps")
		return "", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Completed != 2 {
		t.Fatalf("completed = %d, want 2", result.Completed)
	}
	var captured struct {
		ExitCode int             `json:"exit_code"`
		Stdout   string          `json:"stdout"`
		JSON     json.RawMessage `json:"json"`
	}
	if err := json.Unmarshal([]byte(result.FinalOutput), &captured); err != nil {
		t.Fatalf("script output is not json envelope: %v\n%s", err, result.FinalOutput)
	}
	if captured.ExitCode != 0 || !strings.Contains(captured.Stdout, `"ok":true`) || len(captured.JSON) == 0 {
		t.Fatalf("captured = %+v", captured)
	}
}

func TestExecutorInfersRepoHintFromGitRemote(t *testing.T) {
	tmp := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = tmp
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	runGit("init")
	runGit("remote", "add", "origin", "git@github.com:flexcompute/flex.git")
	subdir := filepath.Join(tmp, "frontend", "flow360-ui-next")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.Chdir(subdir); err != nil {
		t.Fatal(err)
	}

	recipe := &formula.Recipe{Vars: map[string]*formula.VarDef{"repo_hint": {Default: stringPtr("")}}}
	exec := New(recipe, RunOptions{})
	if got := exec.Context()["repo_hint"]; got != "flexcompute/flex" {
		t.Fatalf("repo_hint = %q, want flexcompute/flex", got)
	}
}

func TestParseGitRemoteRepo(t *testing.T) {
	cases := map[string]string{
		"git@github.com:flexcompute/flex.git":        "flexcompute/flex",
		"git@github_fc:cmsflexc/flexcompute.com.git": "cmsflexc/flexcompute.com",
		"https://github.com/flexcompute/flex.git":    "flexcompute/flex",
		"ssh://git@github.com/flexcompute/flex.git":  "flexcompute/flex",
		"/Users/me/src/flexcompute/flex.git":         "flexcompute/flex",
		"":                                           "",
	}
	for input, want := range cases {
		if got := parseGitRemoteRepo(input); got != want {
			t.Fatalf("parseGitRemoteRepo(%q) = %q, want %q", input, got, want)
		}
	}
}

func stringPtr(s string) *string { return &s }

func TestExecutorDeniesDangerousScriptCommand(t *testing.T) {
	recipe := &formula.Recipe{Name: "script-deny", Steps: []formula.RecipeStep{
		{ID: "script-deny", Title: "Root", IsRoot: true},
		{ID: "script-deny.rm", Title: "Remove", Execution: "script", Script: &formula.ScriptSpec{Command: []string{"rm", "-rf", "/tmp/nope"}}},
	}, Deps: []formula.RecipeDep{{StepID: "script-deny.rm", DependsOnID: "script-deny", Type: "parent-child"}}}
	_, err := New(recipe, RunOptions{AllowScripts: true}).Run(context.Background(), func(ctx context.Context, step *formula.RecipeStep, prompt string) (string, error) { return "", nil })
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("err = %v, want denied error", err)
	}
}

func TestExecutorCanDisableScriptSteps(t *testing.T) {
	recipe := &formula.Recipe{Name: "script-disabled", Steps: []formula.RecipeStep{
		{ID: "script-disabled", Title: "Root", IsRoot: true},
		{ID: "script-disabled.echo", Title: "Echo", Execution: "script", Script: &formula.ScriptSpec{Command: []string{"printf", "ok"}}},
	}, Deps: []formula.RecipeDep{{StepID: "script-disabled.echo", DependsOnID: "script-disabled", Type: "parent-child"}}}
	_, err := New(recipe, RunOptions{AllowScripts: false}).Run(context.Background(), func(ctx context.Context, step *formula.RecipeStep, prompt string) (string, error) { return "", nil })
	if err == nil || !strings.Contains(err.Error(), "uses script execution") {
		t.Fatalf("err = %v, want disabled script error", err)
	}
}

func TestExecutorDisablesShellScriptByDefault(t *testing.T) {
	recipe := &formula.Recipe{Name: "script-shell", Steps: []formula.RecipeStep{
		{ID: "script-shell", Title: "Root", IsRoot: true},
		{ID: "script-shell.echo", Title: "Echo", Execution: "script", Script: &formula.ScriptSpec{Shell: "sh", Command: []string{"printf ok"}}},
	}, Deps: []formula.RecipeDep{{StepID: "script-shell.echo", DependsOnID: "script-shell", Type: "parent-child"}}}
	_, err := New(recipe, RunOptions{AllowScripts: true}).Run(context.Background(), func(ctx context.Context, step *formula.RecipeStep, prompt string) (string, error) { return "", nil })
	if err == nil || !strings.Contains(err.Error(), "shell mode is disabled") {
		t.Fatalf("err = %v, want shell disabled error", err)
	}
}

func TestEvaluateConditionSupportsJSONPath(t *testing.T) {
	ctx := map[string]string{"decision": `{"approved":true,"score": 9, "kind":"frontend"}`}
	if !EvaluateCondition("decision.approved == true", ctx) {
		t.Fatalf("expected boolean JSON path condition to pass")
	}
	if !EvaluateCondition("decision.kind == frontend", ctx) {
		t.Fatalf("expected string JSON path condition to pass")
	}
	if !EvaluateCondition("decision.score == 9", ctx) {
		t.Fatalf("expected numeric JSON path condition to pass")
	}
}

func TestExecutorRuntimeUntilLoopUsesAgentOutput(t *testing.T) {
	recipe := &formula.Recipe{
		Name: "runtime-loop",
		Steps: []formula.RecipeStep{
			{ID: "runtime-loop", Title: "Root", IsRoot: true},
			{
				ID:    "runtime-loop.improve",
				Title: "Improve until approved",
				Loop:  &formula.LoopSpec{Until: "review.approved == true", Max: 3, Body: []*formula.Step{{ID: "review", Title: "Review {{iteration}}", OutputKey: "review"}}},
			},
		},
		Deps: []formula.RecipeDep{{StepID: "runtime-loop.improve", DependsOnID: "runtime-loop", Type: "parent-child"}},
	}
	calls := 0
	result, err := New(recipe, RunOptions{}).Run(context.Background(), func(ctx context.Context, step *formula.RecipeStep, prompt string) (string, error) {
		calls++
		if calls == 1 {
			return `{"approved":false}`, nil
		}
		return `{"approved":true}`, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	if result.Completed != 4 { // root + loop + two loop-body iterations
		t.Fatalf("completed = %d, want 4; result=%+v", result.Completed, result)
	}
}

func TestExecutorRuntimeUntilLoopEmitsParentCompletionUpdate(t *testing.T) {
	recipe := &formula.Recipe{
		Name: "runtime-loop",
		Steps: []formula.RecipeStep{
			{ID: "runtime-loop", Title: "Root", IsRoot: true},
			{
				ID:    "runtime-loop.improve",
				Title: "Improve until approved",
				Loop:  &formula.LoopSpec{Until: "review.approved == true", Max: 2, Body: []*formula.Step{{ID: "review", Title: "Review", OutputKey: "review"}}},
			},
		},
		Deps: []formula.RecipeDep{{StepID: "runtime-loop.improve", DependsOnID: "runtime-loop", Type: "parent-child"}},
	}
	var updates []StepResult
	_, err := New(recipe, RunOptions{OnStepUpdate: func(result StepResult) {
		updates = append(updates, result)
	}}).Run(context.Background(), func(ctx context.Context, step *formula.RecipeStep, prompt string) (string, error) {
		return `{"approved":true}`, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 1 {
		t.Fatalf("updates = %+v, want one parent completion update", updates)
	}
	if updates[0].StepID != "runtime-loop.improve" || updates[0].Status != StatusCompleted || updates[0].Output == "" {
		t.Fatalf("update = %+v, want completed loop parent with output", updates[0])
	}
}

func TestResolveAgentDefaultsToPicoclawMain(t *testing.T) {
	exec := New(&formula.Recipe{}, RunOptions{})
	agent := exec.resolveAgent(&formula.RecipeStep{ID: "step"})
	if agent.Name != defaultAgentID {
		t.Fatalf("default agent = %q, want %q", agent.Name, defaultAgentID)
	}
}

func TestResolveAgentPreservesExplicitStepAgent(t *testing.T) {
	exec := New(&formula.Recipe{}, RunOptions{Agent: defaultAgentID})
	agent := exec.resolveAgent(&formula.RecipeStep{ID: "step", Agent: &formula.AgentConfig{Name: "planner", Model: "custom-model"}})
	if agent.Name != "planner" || agent.Model != "custom-model" {
		t.Fatalf("agent = %+v, want explicit step agent", agent)
	}
}
