package formulacmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/sjzsdu/tt/internal/formula/ir"
	formularuntime "github.com/sjzsdu/tt/internal/formula/runtime"
	"github.com/sjzsdu/tt/internal/formula/steps"
	"github.com/sjzsdu/tt/internal/formulaui"
	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
)

func testFormulaWorkflow(name string, nodes ...steps.Step) *ir.Workflow {
	wf := &ir.Workflow{ID: ir.WorkflowID(name), Name: name, Graph: ir.NewGraph()}
	for _, step := range nodes {
		meta := step.Meta()
		wf.Graph.AddNode(&ir.Node{ID: ir.NodeID(meta.ID), Step: step})
	}
	return wf
}

type fakeContextFormulaDirectProcessor struct {
	ctxErr error
}

func (f *fakeContextFormulaDirectProcessor) ProcessDirect(opt pcwrap.RunOptions) (string, error) {
	return "", errors.New("context-aware processor was not used")
}

func (f *fakeContextFormulaDirectProcessor) ProcessDirectContext(ctx context.Context, opt pcwrap.RunOptions) (string, error) {
	f.ctxErr = ctx.Err()
	return "", f.ctxErr
}

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

func TestFormulaRuntimeAgentRunnerIsolatesLoopIterationSessions(t *testing.T) {
	fake := &fakeFormulaDirectProcessor{}
	runner := formulaRuntimeAgentRunner{processor: fake, defaultAgent: "main", session: "session-a"}
	_, err := runner.RunAgent(context.Background(), steps.AgentRequest{NodeID: "write-articles.iter3.draft", Prompt: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if fake.opt.Session != "session-a.write-articles.iter3.draft" {
		t.Fatalf("session = %q", fake.opt.Session)
	}
}

func TestFormulaRuntimeAgentRunnerKeepsTopLevelSession(t *testing.T) {
	fake := &fakeFormulaDirectProcessor{}
	runner := formulaRuntimeAgentRunner{processor: fake, defaultAgent: "main", session: "session-a"}
	_, err := runner.RunAgent(context.Background(), steps.AgentRequest{NodeID: "article-plan", Prompt: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if fake.opt.Session != "session-a" {
		t.Fatalf("session = %q", fake.opt.Session)
	}
}

func TestFormulaRuntimeAgentRunnerInjectsStepAdvice(t *testing.T) {
	fake := &fakeFormulaDirectProcessor{}
	runner := formulaRuntimeAgentRunner{processor: fake, defaultAgent: "main", session: "session-a", stepAdvice: map[string]string{"step-1": "try a smaller change"}}
	_, err := runner.RunAgent(context.Background(), steps.AgentRequest{NodeID: "step-1", Prompt: "original prompt"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fake.opt.Message, "original prompt") || !strings.Contains(fake.opt.Message, "try a smaller change") {
		t.Fatalf("message = %q, want original prompt and retry advice", fake.opt.Message)
	}
}

func TestFormulaRuntimeAgentRunnerPropagatesContextCancellation(t *testing.T) {
	fake := &fakeContextFormulaDirectProcessor{}
	runner := formulaRuntimeAgentRunner{processor: fake, defaultAgent: "main"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := runner.RunAgent(ctx, steps.AgentRequest{Prompt: "hello"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if !errors.Is(fake.ctxErr, context.Canceled) {
		t.Fatalf("processor ctx err = %v, want context.Canceled", fake.ctxErr)
	}
}

func TestNewFormulaRuntimeExecutorBuildsWorkflowExecutor(t *testing.T) {
	workflow := testFormulaWorkflow("demo", steps.NoopStep{Base: steps.Base{Metadata: steps.Metadata{ID: "demo.start", Kind: steps.KindNoop, Title: "start"}}})
	exec, err := newFormulaRuntimeExecutor(formulaRuntimeRunOptions{Workflow: workflow, DryRun: true, AllowScripts: true})
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
	workflow := testFormulaWorkflow("demo", steps.NoopStep{Base: steps.Base{Metadata: steps.Metadata{ID: "demo.start", Kind: steps.KindNoop, Title: "start"}}})
	var out bytes.Buffer
	err := executeFormulaRecipeRuntime(context.Background(), executeFormulaRuntimeOptions{Workflow: workflow, DryRun: true, AllowScripts: true, Out: &out})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Runtime status: completed") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestFormulaRuntimeDashboardEventSinkUpdatesDashboard(t *testing.T) {
	workflow := testFormulaWorkflow("demo", steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "demo.work", Kind: steps.KindAgent, Title: "Work"}}})
	dashboard := newFormulaDashboardServer(workflow)
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

func TestRuntimeSnapshotToDashboardSnapshot(t *testing.T) {
	workflow := testFormulaWorkflow("demo", steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "demo.work", Kind: steps.KindAgent, Title: "Work"}}})
	raw, _ := json.Marshal("saved output")
	snapshot := formularuntime.Snapshot{
		WorkflowID: "demo",
		Status:     steps.StatusWaiting,
		Steps: map[ir.NodeID]formularuntime.StepState{
			"demo.work": {NodeID: "demo.work", Status: steps.StatusCompleted, Result: &steps.RunResult{Status: steps.StatusCompleted, Output: steps.Value{Type: "json", Raw: raw}}},
		},
	}
	dashboard := runtimeSnapshotToDashboardSnapshot(workflow, snapshot)
	if dashboard.Status != string(steps.StatusWaiting) {
		t.Fatalf("status = %s", dashboard.Status)
	}
	if len(dashboard.Steps) != 1 || dashboard.Steps[0].Output != "saved output" || dashboard.Steps[0].Status != "completed" {
		t.Fatalf("steps = %+v", dashboard.Steps)
	}
}

func TestResumeOutputValuePreservesJSON(t *testing.T) {
	value := resumeOutputValue(`[{"filename":"01.md","content":"# One"}]`)
	var got []map[string]string
	if err := json.Unmarshal(value.Raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0]["filename"] != "01.md" {
		t.Fatalf("value = %s", value.Raw)
	}
}

func TestResumeDependencyExclusionsRerunsLoopAncestorOfFailedStep(t *testing.T) {
	workflow := testFormulaWorkflow("demo",
		steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "plan", Kind: steps.KindAgent}}},
		steps.LoopStep{Base: steps.Base{Metadata: steps.Metadata{ID: "write-articles", Kind: steps.KindLoop}}},
		steps.ToolStep{Base: steps.Base{Metadata: steps.Metadata{ID: "write-doc-files", Kind: steps.KindTool}}},
	)
	workflow.Graph.AddEdge("plan", "write-articles", "blocks")
	workflow.Graph.AddEdge("write-articles", "write-doc-files", "blocks")
	snapshot := formulaui.Snapshot{Steps: []formulaui.Step{
		{ID: "plan", Status: "completed"},
		{ID: "write-articles", Status: "completed"},
		{ID: "write-doc-files", Status: "failed"},
	}}

	exclude := resumeDependencyExclusions(workflow, snapshot)
	if !exclude["write-articles"] {
		t.Fatalf("expected write-articles to be excluded, got %+v", exclude)
	}
	if exclude["plan"] {
		t.Fatalf("non-loop ancestor should remain reusable: %+v", exclude)
	}
}
