package formulacmd

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sjzsdu/tt/internal/formulaui"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

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

type formulaContextDirectProcessor interface {
	ProcessDirectContext(context.Context, pcwrap.RunOptions) (string, error)
}

func loadFormulaRunSnapshot(dir string, workflow *ir.Workflow) (formulaui.Snapshot, error) {
	var dashboardSnapshot formulaui.Snapshot
	if err := formularun.LoadState(dir, &dashboardSnapshot); err == nil {
		if dashboardSnapshot.RecipeName != "" || len(dashboardSnapshot.Steps) > 0 {
			return dashboardSnapshot, nil
		}
	}
	var runtimeSnapshot formularuntime.Snapshot
	if err := formularun.LoadState(dir, &runtimeSnapshot); err != nil {
		return dashboardSnapshot, err
	}
	return runtimeSnapshotToDashboardSnapshot(workflow, runtimeSnapshot), nil
}

func runtimeSnapshotToDashboardSnapshot(workflow *ir.Workflow, snapshot formularuntime.Snapshot) formulaui.Snapshot {
	if workflow == nil {
		return formulaui.Snapshot{RecipeName: string(snapshot.WorkflowID), Status: string(snapshot.Status)}
	}
	dashboard := newFormulaDashboardServer(workflow)
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

func applyRuntimeStepStateToDashboardStep(step *formulaui.Step, state formularuntime.StepState) {
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
		return formulaui.StatusWaitingInput
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
	stepAdvice   map[string]string
}

func (r formulaRuntimeAgentRunner) RunAgent(ctx context.Context, req steps.AgentRequest) (steps.Value, error) {
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
	prompt := req.Prompt
	if advice := strings.TrimSpace(r.stepAdvice[req.NodeID]); advice != "" {
		prompt = strings.TrimSpace(prompt) + "\n\nRetry advice from dashboard:\n" + advice
	}
	opt := pcwrap.RunOptions{
		Message:   prompt,
		Session:   r.session,
		Agent:     agent,
		Model:     model,
		Workspace: r.workspace,
		Debug:     r.debug,
		Quiet:     r.quiet,
	}
	var resp string
	var err error
	if contextProcessor, ok := r.processor.(formulaContextDirectProcessor); ok {
		resp, err = contextProcessor.ProcessDirectContext(ctx, opt)
	} else {
		resp, err = r.processor.ProcessDirect(opt)
	}
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
	Workflow     *ir.Workflow
	RunStore     *formularun.Store
	AgentRunner  steps.AgentRunner
	Workspace    string
	Vars         map[string]string
	DryRun       bool
	AllowScripts bool
}

func newFormulaRuntimeExecutor(opt formulaRuntimeRunOptions) (*formularuntime.Executor, error) {
	workflow := opt.Workflow
	if workflow == nil {
		return nil, fmt.Errorf("workflow is required")
	}
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
	exec.SeedEnvironment(opt.Workspace)
	exec.SeedWorkflowVars(workflow)
	exec.SeedVars(opt.Vars)
	if opt.RunStore != nil {
		exec.Store = formularuntime.NewFormulaRunStateStore(opt.RunStore)
	}
	return exec, nil
}

type executeFormulaRuntimeOptions struct {
	Workflow     *ir.Workflow
	RunStore     *formularun.Store
	Processor    formulaDirectProcessor
	DefaultAgent string
	DefaultModel string
	Session      string
	Workspace    string
	Vars         map[string]string
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
		Workflow:     opt.Workflow,
		RunStore:     opt.RunStore,
		AgentRunner:  agentRunner,
		Workspace:    opt.Workspace,
		Vars:         opt.Vars,
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
		fmt.Fprintf(opt.Out, "Executing formula with typed runtime: %s\n", exec.Workflow.Name)
	}
	result, err := exec.Run(ctx)
	if opt.Out != nil && result != nil {
		fmt.Fprintf(opt.Out, "Runtime status: %s\n", result.Status)
		fmt.Fprintf(opt.Out, "Runtime steps: %d\n", len(result.Nodes))
	}
	if opt.RunStore != nil {
		status := formularun.StatusCompleted
		errMsg := ""
		if ctx.Err() != nil {
			status = formularun.StatusInterrupted
			errMsg = ctx.Err().Error()
		} else if result != nil && result.Status == steps.StatusWaiting {
			status = formularun.StatusWaitingInput
		} else if err != nil || (result != nil && result.Status == steps.StatusFailed) {
			status = formularun.StatusFailed
			if err != nil {
				errMsg = err.Error()
			}
		}
		if status != formularun.StatusWaitingInput {
			_ = opt.RunStore.Finish(status, errMsg)
		}
	}
	if err != nil {
		return err
	}
	return nil
}

func seedFormulaRuntimeResumeState(exec *formularuntime.Executor, initialResults []formulaui.ResumeStepResult, initialContext map[string]string) {
	if exec == nil || exec.Workflow == nil || exec.Store == nil {
		return
	}
	for key, value := range initialContext {
		if strings.TrimSpace(key) == "" {
			continue
		}
		data, err := json.Marshal(value)
		if err != nil {
			continue
		}
		_ = exec.Context.Set(key, steps.Value{Type: "json", Raw: data})
	}
	for _, result := range initialResults {
		if result.Status != formulaui.StatusCompleted || strings.TrimSpace(result.StepID) == "" {
			continue
		}
		data, err := json.Marshal(result.Output)
		if err != nil {
			continue
		}
		runtimeResult := &steps.RunResult{Status: steps.StatusCompleted, Output: steps.Value{Type: "json", Raw: data}}
		_ = exec.Store.SaveStep(formularuntime.StepState{
			WorkflowID: exec.Workflow.ID,
			NodeID:     ir.NodeID(result.StepID),
			Status:     steps.StatusCompleted,
			Result:     runtimeResult,
			UpdatedAt:  time.Now(),
		})
	}
}

func renderFormulaRuntimeResult(cmd *cobra.Command, workflow *ir.Workflow, result *formularuntime.RunResult, hasError bool) {
	out := cmd.OutOrStdout()
	name := ""
	if workflow != nil {
		name = workflow.Name
	}
	if result == nil {
		fmt.Fprintf(out, "\nRuntime Result: %s\n", name)
		return
	}
	completed, failed, waiting, skipped := 0, 0, 0, 0
	for _, nodeResult := range result.Nodes {
		if nodeResult == nil {
			continue
		}
		switch nodeResult.Status {
		case steps.StatusCompleted:
			completed++
		case steps.StatusFailed:
			failed++
		case steps.StatusWaiting:
			waiting++
		case steps.StatusSkipped:
			skipped++
		}
	}
	fmt.Fprintf(out, "\nRuntime Result: %s\n", name)
	fmt.Fprintf(out, "Status: %s | Total: %d | Completed: %d | Failed: %d | Skipped: %d | Waiting input: %d\n\n", result.Status, len(result.Nodes), completed, failed, skipped, waiting)
	_ = hasError
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
	} else {
		title, agent, model = loopBodyEventDetails(s.workflow, string(event.NodeID))
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

func loopBodyEventDetails(workflow *ir.Workflow, nodeID string) (title, agent, model string) {
	parentID := formulaui.LoopParentStepID(nodeID)
	if workflow == nil || parentID == "" {
		return "", "", ""
	}
	bodyID := nodeID[strings.LastIndex(nodeID, ".")+1:]
	parent := workflow.Graph.Nodes[ir.NodeID(parentID)]
	if parent == nil || parent.Step == nil {
		return "", "", ""
	}
	var body []steps.Step
	switch loop := parent.Step.(type) {
	case steps.LoopStep:
		body = loop.Body
	case *steps.LoopStep:
		body = loop.Body
	}
	for _, child := range body {
		if child == nil || string(child.Meta().ID) != bodyID {
			continue
		}
		title = child.Meta().Title
		if agentStep, ok := child.(steps.AgentStep); ok {
			agent = agentStep.Agent
			model = agentStep.Model
		}
		return title, agent, model
	}
	return "", "", ""
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

func runtimeEventHumanInputRequest(payload any) *formulaui.HumanInputRequest {
	await, ok := payload.(*steps.AwaitRequest)
	if !ok || await == nil {
		return nil
	}
	req := &formulaui.HumanInputRequest{Reason: await.Reason}
	if form, ok := await.Form.(*formula.FormSpec); ok {
		req.Form = form
	} else if await.Form != nil {
		data, err := json.Marshal(await.Form)
		if err == nil {
			var form formula.FormSpec
			if err := json.Unmarshal(data, &form); err == nil {
				req.Form = &form
			}
		}
	}
	return req
}
