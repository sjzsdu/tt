package formulacmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/sjzsdu/tt/internal/executor"
	"github.com/sjzsdu/tt/internal/formula"
	"github.com/sjzsdu/tt/internal/formula/ir"
	formularuntime "github.com/sjzsdu/tt/internal/formula/runtime"
	"github.com/sjzsdu/tt/internal/formula/steps"
	"github.com/sjzsdu/tt/internal/formularun"
	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
)

type formulaDirectProcessor interface {
	ProcessDirect(pcwrap.RunOptions) (string, error)
}

func loadFormulaRunSnapshot(dir string, recipe *formula.Recipe) (formulaDashboardSnapshot, error) {
	var dashboardSnapshot formulaDashboardSnapshot
	if err := formularun.LoadState(dir, &dashboardSnapshot); err == nil {
		if dashboardSnapshot.RecipeName != "" || len(dashboardSnapshot.Steps) > 0 {
			return dashboardSnapshot, nil
		}
	}
	var runtimeSnapshot formularuntime.Snapshot
	if err := formularun.LoadState(dir, &runtimeSnapshot); err != nil {
		return dashboardSnapshot, err
	}
	return runtimeSnapshotToDashboardSnapshot(recipe, runtimeSnapshot), nil
}

func runtimeSnapshotToDashboardSnapshot(recipe *formula.Recipe, snapshot formularuntime.Snapshot) formulaDashboardSnapshot {
	if recipe == nil {
		return formulaDashboardSnapshot{RecipeName: string(snapshot.WorkflowID), Status: string(snapshot.Status)}
	}
	dashboard := newFormulaDashboardServer(recipe)
	out := dashboard.state
	out.Status = string(snapshot.Status)
	for i := range out.Steps {
		state, ok := snapshot.Steps[ir.NodeID(out.Steps[i].ID)]
		if !ok {
			continue
		}
		applyRuntimeStepStateToDashboardStep(&out.Steps[i], state)
	}
	return out
}

func applyRuntimeStepStateToDashboardStep(step *formulaDashboardStep, state formularuntime.StepState) {
	if step == nil {
		return
	}
	step.Status = runtimeStatusToDashboardStatus(state.Status)
	if !state.StartedAt.IsZero() {
		step.StartedAt = state.StartedAt.Format(time.RFC3339)
	}
	if !state.CompletedAt.IsZero() {
		step.FinishedAt = state.CompletedAt.Format(time.RFC3339)
	}
	if state.Result != nil {
		step.Output = runtimeEventOutput(state.Result)
		step.Error = runtimeEventError(state.Result)
		if state.Result.Await != nil {
			step.HumanInputRequest = runtimeEventHumanInputRequest(state.Result.Await)
		}
	}
}

func runtimeStatusToDashboardStatus(status steps.Status) string {
	if status == steps.StatusWaiting {
		return string(executor.StatusWaitingInput)
	}
	return string(status)
}

type formulaRuntimeAgentRunner struct {
	processor    formulaDirectProcessor
	defaultAgent string
	defaultModel string
	session      string
	workspace    string
	debug        bool
	quiet        bool
}

func (r formulaRuntimeAgentRunner) RunAgent(_ context.Context, req steps.AgentRequest) (steps.Value, error) {
	if r.processor == nil {
		return steps.Value{}, fmt.Errorf("picoclaw direct runner is required")
	}
	agent := strings.TrimSpace(req.Agent)
	if agent == "" {
		agent = r.defaultAgent
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = r.defaultModel
	}
	resp, err := r.processor.ProcessDirect(pcwrap.RunOptions{
		Message:   req.Prompt,
		Session:   r.session,
		Agent:     agent,
		Model:     model,
		Workspace: r.workspace,
		Debug:     r.debug,
		Quiet:     r.quiet,
	})
	if err != nil {
		return steps.Value{}, err
	}
	data, err := json.Marshal(strings.TrimSpace(resp))
	if err != nil {
		return steps.Value{}, err
	}
	return steps.Value{Type: "json", Raw: data}, nil
}

type formulaRuntimeRunOptions struct {
	Recipe       *formula.Recipe
	RunStore     *formularun.Store
	AgentRunner  steps.AgentRunner
	DryRun       bool
	AllowScripts bool
}

func newFormulaRuntimeExecutor(opt formulaRuntimeRunOptions) (*formularuntime.Executor, error) {
	if opt.Recipe == nil {
		return nil, fmt.Errorf("recipe is required")
	}
	workflow := formula.WorkflowFromRecipe(opt.Recipe)
	capabilities := steps.Capabilities{}
	if opt.DryRun {
		capabilities.Agents = formularuntime.DryRunAgentCapability{}
		capabilities.Scripts = formularuntime.DryRunScriptCapability{}
	} else {
		capabilities.Agents = opt.AgentRunner
		if opt.AllowScripts {
			capabilities.Scripts = formularuntime.ScriptCapability{DenyUnsafe: true}
		}
	}
	exec := formularuntime.NewExecutor(workflow, capabilities)
	if opt.RunStore != nil {
		exec.Store = formularuntime.NewFormulaRunStateStore(opt.RunStore)
	}
	return exec, nil
}

type executeFormulaRuntimeOptions struct {
	Recipe       *formula.Recipe
	RunStore     *formularun.Store
	Processor    formulaDirectProcessor
	DefaultAgent string
	DefaultModel string
	Session      string
	Workspace    string
	Debug        bool
	DryRun       bool
	AllowScripts bool
	Dashboard    *formulaDashboardServer
	Out          io.Writer
}

func executeFormulaRecipeRuntime(ctx context.Context, opt executeFormulaRuntimeOptions) error {
	agentRunner := formulaRuntimeAgentRunner{
		processor:    opt.Processor,
		defaultAgent: opt.DefaultAgent,
		defaultModel: opt.DefaultModel,
		session:      opt.Session,
		workspace:    opt.Workspace,
		debug:        opt.Debug,
		quiet:        true,
	}
	exec, err := newFormulaRuntimeExecutor(formulaRuntimeRunOptions{
		Recipe:       opt.Recipe,
		RunStore:     opt.RunStore,
		AgentRunner:  agentRunner,
		DryRun:       opt.DryRun,
		AllowScripts: opt.AllowScripts,
	})
	if err != nil {
		return err
	}
	if opt.Dashboard != nil {
		exec.Events = formulaRuntimeDashboardEventSink{dashboard: opt.Dashboard, workflow: exec.Workflow}
	}
	if opt.Out != nil {
		fmt.Fprintf(opt.Out, "Executing formula with typed runtime: %s\n", opt.Recipe.Name)
	}
	result, err := exec.Run(ctx)
	if opt.Out != nil && result != nil {
		fmt.Fprintf(opt.Out, "Runtime status: %s\n", result.Status)
		fmt.Fprintf(opt.Out, "Runtime steps: %d\n", len(result.Nodes))
	}
	if err != nil {
		return err
	}
	return nil
}

type formulaRuntimeDashboardEventSink struct {
	dashboard *formulaDashboardServer
	workflow  *ir.Workflow
}

func (s formulaRuntimeDashboardEventSink) Emit(event formularuntime.Event) {
	if s.dashboard == nil || event.NodeID == "" {
		return
	}
	node := s.workflow.Graph.Nodes[event.NodeID]
	title := ""
	agent := ""
	model := ""
	if node != nil && node.Step != nil {
		meta := node.Step.Meta()
		title = meta.Title
		if agentStep, ok := node.Step.(steps.AgentStep); ok {
			agent = agentStep.Agent
			model = agentStep.Model
		}
	}
	switch event.Type {
	case "step.started":
		s.dashboard.markStepRunning(string(event.NodeID), title, agent, model, "")
	case "step.completed":
		s.dashboard.markStepCompleted(string(event.NodeID), runtimeEventOutput(event.Payload))
	case "step.failed":
		s.dashboard.markStepFailed(string(event.NodeID), runtimeEventError(event.Payload), runtimeEventOutput(event.Payload))
	case "step.waiting":
		s.dashboard.markStepWaitingInput(string(event.NodeID), title, runtimeEventHumanInputRequest(event.Payload))
	}
}

func runtimeEventOutput(payload any) string {
	result, ok := payload.(*steps.RunResult)
	if !ok || result == nil || len(result.Output.Raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(result.Output.Raw, &text); err == nil {
		return text
	}
	return string(result.Output.Raw)
}

func runtimeEventError(payload any) string {
	result, ok := payload.(*steps.RunResult)
	if !ok || result == nil || result.Error == nil {
		return ""
	}
	return result.Error.Error()
}

func runtimeEventHumanInputRequest(payload any) *executor.HumanInputRequest {
	await, ok := payload.(*steps.AwaitRequest)
	if !ok || await == nil {
		return nil
	}
	req := &executor.HumanInputRequest{Reason: await.Reason}
	if form, ok := await.Form.(*formula.FormSpec); ok {
		req.Form = form
	}
	return req
}
