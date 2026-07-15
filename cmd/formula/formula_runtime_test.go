package formulacmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sjzsdu/tt/internal/formula/executionpath"
	"github.com/sjzsdu/tt/internal/formula/ir"
	formularuntime "github.com/sjzsdu/tt/internal/formula/runtime"
	"github.com/sjzsdu/tt/internal/formula/steps"
	"github.com/sjzsdu/tt/internal/formula/ui"
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

type fakeDashboardDirectProcessor struct {
	opt    pcwrap.RunOptions
	called chan struct{}
}

func (f *fakeDashboardDirectProcessor) ProcessDirectContext(ctx context.Context, opt pcwrap.RunOptions) (string, error) {
	f.opt = opt
	if f.called != nil {
		close(f.called)
	}
	return "assistant reply", nil
}

type blockingDashboardDirectProcessor struct {
	called  chan struct{}
	release chan struct{}
}

func (f *blockingDashboardDirectProcessor) ProcessDirectContext(ctx context.Context, opt pcwrap.RunOptions) (string, error) {
	close(f.called)
	<-f.release
	return "assistant reply", nil
}

func TestFormulaRuntimeAgentRunnerUsesDefaults(t *testing.T) {
	fake := &fakeFormulaDirectProcessor{}
	runner := formulaRuntimeAgentRunner{processor: fake, defaultAgent: "main", defaultModel: "model-a", session: "session-a", workspace: "/tmp/ws", quiet: true}
	value, err := runner.RunAgent(context.Background(), steps.AgentRequest{Prompt: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if fake.opt.Agent != "main" || fake.opt.Model != "model-a" || fake.opt.Session != "session-a" {
		t.Fatalf("unexpected options: %+v", fake.opt)
	}
	if !strings.Contains(fake.opt.Message, "hello") || !strings.Contains(fake.opt.Message, "/tmp/ws") {
		t.Fatalf("message = %q, want prompt and workspace guard", fake.opt.Message)
	}
	var got string
	if err := json.Unmarshal(value.Raw, &got); err != nil {
		t.Fatal(err)
	}
	if got != "agent response" {
		t.Fatalf("response = %q", got)
	}
}

func TestFormulaRuntimeAgentRunnerUsesRequestWorkspace(t *testing.T) {
	fake := &fakeFormulaDirectProcessor{}
	runner := formulaRuntimeAgentRunner{processor: fake, defaultAgent: "main", session: "session-a", workspace: "/tmp/original"}
	_, err := runner.RunAgent(context.Background(), steps.AgentRequest{Prompt: "hello", Workspace: "/tmp/worktree"})
	if err != nil {
		t.Fatal(err)
	}
	if fake.opt.Workspace != "/tmp/worktree" {
		t.Fatalf("workspace = %q, want /tmp/worktree", fake.opt.Workspace)
	}
	if fake.opt.Session == "session-a" || !strings.HasPrefix(fake.opt.Session, "session-a.ws-") {
		t.Fatalf("session = %q, want workspace-isolated session", fake.opt.Session)
	}
	if !strings.Contains(fake.opt.Message, "/tmp/worktree") || !strings.Contains(fake.opt.Message, "MUST be performed inside this formula workspace") {
		t.Fatalf("message = %q, want formula workspace guard", fake.opt.Message)
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

func TestFormulaRuntimeDashboardEventSinkMarksInterrupted(t *testing.T) {
	workflow := testFormulaWorkflow("demo", steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "demo.work", Kind: steps.KindAgent, Title: "Work"}}})
	dashboard := newFormulaDashboardServer(workflow)
	sink := formulaRuntimeDashboardEventSink{dashboard: dashboard, workflow: workflow}

	sink.Emit(formularuntime.Event{Type: "step.started", NodeID: "demo.work"})
	sink.Emit(formularuntime.Event{Type: "step.interrupted", NodeID: "demo.work", Payload: map[string]string{"error": "context canceled"}})

	step := dashboard.state.Steps[0]
	if step.Status != "interrupted" || !strings.Contains(step.Error, "context canceled") {
		t.Fatalf("step = %+v", step)
	}
	if dashboard.state.Status != "interrupted" {
		t.Fatalf("workflow status = %s", dashboard.state.Status)
	}
}

func TestFormulaRuntimeDashboardEventSinkMarksLoopParentInterrupted(t *testing.T) {
	workflow := testFormulaWorkflow("demo", steps.LoopStep{
		Base: steps.Base{Metadata: steps.Metadata{ID: "demo.cycle", Kind: steps.KindLoop, Title: "Cycle"}},
		Body: []steps.Step{steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "work", Kind: steps.KindAgent, Title: "Work"}}}},
	})
	dashboard := newFormulaDashboardServer(workflow)
	sink := formulaRuntimeDashboardEventSink{dashboard: dashboard, workflow: workflow}

	childID := ir.NodeID("demo.cycle.iter1.work")
	sink.Emit(formularuntime.Event{Type: "step.started", NodeID: childID})
	sink.Emit(formularuntime.Event{Type: "step.interrupted", NodeID: childID, Payload: map[string]string{"error": "context canceled"}})

	step := dashboard.state.Steps[0]
	if step.Status != "interrupted" || !strings.Contains(step.Error, "context canceled") {
		t.Fatalf("loop parent = %+v", step)
	}
	if len(step.Activities) != 1 || step.Activities[0].Status != "interrupted" {
		t.Fatalf("activities = %+v", step.Activities)
	}
}

func TestFormulaRuntimeDashboardEventSinkCompletesWorkflow(t *testing.T) {
	workflow := testFormulaWorkflow("demo", steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "demo.final", Kind: steps.KindAgent, Title: "Final"}}})
	dashboard := newFormulaDashboardServer(workflow)
	sink := formulaRuntimeDashboardEventSink{dashboard: dashboard, workflow: workflow}
	raw, _ := json.Marshal("final report")
	result := &formularuntime.RunResult{WorkflowID: "demo", Status: steps.StatusCompleted, Nodes: map[ir.NodeID]*steps.RunResult{
		"demo.final": {Status: steps.StatusCompleted, Output: steps.Value{Type: "json", Raw: raw}},
	}}

	sink.Emit(formularuntime.Event{Type: "workflow.completed", Payload: result})

	if dashboard.state.Status != "completed" || dashboard.state.FinalOutput != "final report" {
		t.Fatalf("dashboard = %+v", dashboard.state)
	}
}

func TestFormulaRuntimeDashboardEventSinkRecordsRepairs(t *testing.T) {
	workflow := testFormulaWorkflow("demo", steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "demo.work", Kind: steps.KindAgent, Title: "Work"}}})
	dashboard := newFormulaDashboardServer(workflow)
	sink := formulaRuntimeDashboardEventSink{dashboard: dashboard, workflow: workflow}
	sink.Emit(formularuntime.Event{Type: "step.repair.recorded", NodeID: "demo.work", Payload: formularuntime.RepairRecord{StepID: "demo.work", Attempt: 1, Status: "succeeded", FormulaUpdateHint: "tighten prompt"}})
	if len(dashboard.state.Repairs) != 1 {
		t.Fatalf("repairs = %+v", dashboard.state.Repairs)
	}
	if dashboard.state.Repairs[0].FormulaUpdateHint != "tighten prompt" {
		t.Fatalf("repair = %+v", dashboard.state.Repairs[0])
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
	if len(dashboard.ExecutionInstances) != 1 || dashboard.ExecutionInstances[0].Address != "demo.work" {
		t.Fatalf("execution instances = %+v", dashboard.ExecutionInstances)
	}
}

func TestRuntimeSnapshotRestoresNestedFormulaExecution(t *testing.T) {
	workflow := testFormulaWorkflow("demo", steps.FormulaCallStep{Base: steps.Base{Metadata: steps.Metadata{ID: "invoke", Kind: steps.KindFormula, Title: "Child"}}, Formula: "child"})
	path := executionpath.RootStep("invoke").Formula("child").ChildStep("work")
	snapshot := formularuntime.Snapshot{WorkflowID: "demo", Status: steps.StatusWaiting, Steps: map[ir.NodeID]formularuntime.StepState{
		ir.NodeID(path.String()): {NodeID: ir.NodeID(path.String()), Path: path, Status: steps.StatusWaiting, UpdatedAt: time.Now()},
	}}
	dashboard := runtimeSnapshotToDashboardSnapshot(workflow, snapshot)
	if len(dashboard.ExecutionInstances) != 1 {
		t.Fatalf("instances = %+v", dashboard.ExecutionInstances)
	}
	instance := dashboard.ExecutionInstances[0]
	if instance.Address != "invoke.formula(child).work" || len(instance.FormulaPath) != 1 || instance.FormulaPath[0] != "child" {
		t.Fatalf("instance = %+v", instance)
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
	snapshot := ui.Snapshot{Steps: []ui.Step{
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

func TestFinalReportChatMessageHandler(t *testing.T) {
	dashboard := newFormulaDashboardServer(nil)
	dashboard.state.RunID = "run-123"
	dashboard.state.WorkspaceDir = "/repo/project/.tt"
	dashboard.state.FinalOutput = "report body"
	fake := &fakeDashboardDirectProcessor{called: make(chan struct{})}
	dashboard.directProcessor = fake
	body := bytes.NewBufferString(`{"message":"Please revise it"}`)
	req, err := http.NewRequest(http.MethodPost, "/api/final-report-chat/message", body)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	dashboard.handleFinalReportChatMessage(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	select {
	case <-fake.called:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for final report chat processor")
	}
	if fake.opt.Agent != finalReportChatAgent || fake.opt.Session != "run-123:final-report-chat" {
		t.Fatalf("opt = %+v", fake.opt)
	}
	if fake.opt.Workspace != "/repo/project" {
		t.Fatalf("workspace = %q, want project root", fake.opt.Workspace)
	}
	if !strings.Contains(fake.opt.Message, "report body") || !strings.Contains(fake.opt.Message, "Please revise it") {
		t.Fatalf("message = %q", fake.opt.Message)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if dashboard.state.FinalReportChat != nil && len(dashboard.state.FinalReportChat.Messages) == 2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if dashboard.state.FinalReportChat == nil || len(dashboard.state.FinalReportChat.Messages) != 2 {
		t.Fatalf("chat = %+v", dashboard.state.FinalReportChat)
	}
}

func TestFinalReportChatMessageHandlerReturnsWhileProcessorRuns(t *testing.T) {
	dashboard := newFormulaDashboardServer(nil)
	dashboard.state.RunID = "run-123"
	dashboard.state.FinalOutput = "report body"
	fake := &blockingDashboardDirectProcessor{called: make(chan struct{}), release: make(chan struct{})}
	dashboard.directProcessor = fake

	body := bytes.NewBufferString(`{"message":"Please revise it"}`)
	req, err := http.NewRequest(http.MethodPost, "/api/final-report-chat/message", body)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	dashboard.handleFinalReportChatMessage(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}

	select {
	case <-fake.called:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for final report chat processor")
	}

	snap := dashboard.snapshot()
	if snap.FinalReportChat == nil || snap.FinalReportChat.Status != "running" || len(snap.FinalReportChat.Messages) != 1 {
		t.Fatalf("chat should be running with user message, got %+v", snap.FinalReportChat)
	}

	close(fake.release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snap = dashboard.snapshot()
		if snap.FinalReportChat != nil && snap.FinalReportChat.Status == "idle" && len(snap.FinalReportChat.Messages) == 2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("chat did not finish after processor release: %+v", dashboard.snapshot().FinalReportChat)
}

func TestFinalReportChatPromoteHandler(t *testing.T) {
	dashboard := newFormulaDashboardServer(nil)
	dashboard.state.FinalOutput = "old report"
	dashboard.state.FinalReportChat = &ui.FinalReportChat{Messages: []ui.FinalReportChatMessage{
		{Role: "user", Content: "please revise"},
		{Role: "assistant", Content: "new report"},
	}}
	req, err := http.NewRequest(http.MethodPost, "/api/final-report-chat/promote", nil)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	dashboard.handleFinalReportChatPromote(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if dashboard.state.FinalOutput != "new report" {
		t.Fatalf("final output = %q", dashboard.state.FinalOutput)
	}
	if len(dashboard.state.Logs) == 0 || !strings.Contains(dashboard.state.Logs[len(dashboard.state.Logs)-1].Text, "Final report updated") {
		t.Fatalf("logs = %+v", dashboard.state.Logs)
	}
}
