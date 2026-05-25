package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sjzsdu/tt/internal/formula"
	formularuntime "github.com/sjzsdu/tt/internal/formula/runtime"
	"github.com/sjzsdu/tt/internal/formula/steps"
	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
)

type fakeFormulaDirectProcessor struct {
	opt pcwrap.RunOptions
}

func (f *fakeFormulaDirectProcessor) ProcessDirect(opt pcwrap.RunOptions) (string, error) {
	f.opt = opt
	return " agent response ", nil
}

func TestFormulaRuntimeAgentRunnerUsesDefaults(t *testing.T) {
	fake := &fakeFormulaDirectProcessor{}
	runner := formulaRuntimeAgentRunner{processor: fake, defaultAgent: "main", defaultModel: "model-a", session: "session-a", workspace: "/tmp/ws", quiet: true}
	value, err := runner.RunAgent(context.Background(), steps.AgentRequest{Prompt: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if fake.opt.Agent != "main" || fake.opt.Model != "model-a" || fake.opt.Session != "session-a" || fake.opt.Message != "hello" {
		t.Fatalf("unexpected options: %+v", fake.opt)
	}
	var got string
	if err := json.Unmarshal(value.Raw, &got); err != nil {
		t.Fatal(err)
	}
	if got != "agent response" {
		t.Fatalf("response = %q", got)
	}
}

func TestNewFormulaRuntimeExecutorBuildsWorkflowExecutor(t *testing.T) {
	recipe := &formula.Recipe{Name: "demo", Vars: map[string]*formula.VarDef{}, Steps: []formula.RecipeStep{{ID: "demo", IsRoot: true}, {ID: "demo.start", Execution: "noop"}}}
	exec, err := newFormulaRuntimeExecutor(formulaRuntimeRunOptions{Recipe: recipe, DryRun: true, AllowScripts: true})
	if err != nil {
		t.Fatal(err)
	}
	if exec.Workflow == nil || exec.Workflow.ID != "demo" {
		t.Fatalf("bad workflow: %+v", exec.Workflow)
	}
	if exec.Capabilities.Agents == nil || exec.Capabilities.Scripts == nil {
		t.Fatalf("missing dry-run capabilities: %+v", exec.Capabilities)
	}
}

func TestExecuteFormulaRecipeRuntimeDryRun(t *testing.T) {
	recipe := &formula.Recipe{Name: "demo", Vars: map[string]*formula.VarDef{}, Steps: []formula.RecipeStep{{ID: "demo", IsRoot: true}, {ID: "demo.start", Execution: "noop"}}}
	var out bytes.Buffer
	err := executeFormulaRecipeRuntime(context.Background(), executeFormulaRuntimeOptions{Recipe: recipe, DryRun: true, AllowScripts: true, Out: &out})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Runtime status: completed") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestFormulaRuntimeDashboardEventSinkUpdatesDashboard(t *testing.T) {
	recipe := &formula.Recipe{Name: "demo", Vars: map[string]*formula.VarDef{}, Steps: []formula.RecipeStep{{ID: "demo.work", Title: "Work"}}}
	dashboard := newFormulaDashboardServer(recipe)
	workflow := formula.WorkflowFromRecipe(recipe)
	sink := formulaRuntimeDashboardEventSink{dashboard: dashboard, workflow: workflow}
	sink.Emit(formularuntime.Event{Type: "step.started", NodeID: "demo.work"})
	if dashboard.state.Steps[0].Status != "running" {
		t.Fatalf("status = %s", dashboard.state.Steps[0].Status)
	}
	raw, _ := json.Marshal("done")
	sink.Emit(formularuntime.Event{Type: "step.completed", NodeID: "demo.work", Payload: &steps.RunResult{Status: steps.StatusCompleted, Output: steps.Value{Type: "json", Raw: raw}}})
	if dashboard.state.Steps[0].Status != "completed" || dashboard.state.Steps[0].Output != "done" {
		t.Fatalf("step = %+v", dashboard.state.Steps[0])
	}
}
