package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sjzsdu/tt/internal/formula/executionpath"
	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

type Executor struct {
	Workflow        *ir.Workflow
	Mode            ExecutionMode
	Context         *ContextStore
	Capabilities    steps.Capabilities
	Events          EventSink
	Store           StateStore
	ResolveWorkflow WorkflowResolver
	AddressPrefix   executionpath.Path
	StateWorkflowID ir.WorkflowID
	CallStack       []string
	Nested          bool
	runID           string
	formulaRunDir   string
}

// ExecutionMode separates normal execution from a side-effect-free preview.
// Preview still traverses the runtime graph using dry-run capabilities, but
// their synthetic outputs are not expected to satisfy business schemas.
type ExecutionMode string

const (
	ExecutionModeRun     ExecutionMode = "run"
	ExecutionModePreview ExecutionMode = "preview"
)

func (e *Executor) isPreview() bool {
	return e != nil && e.Mode == ExecutionModePreview
}

type WorkflowResolver func(context.Context, string, map[string]steps.Value) (*ir.Workflow, error)

type EventSink interface{ Emit(Event) }

type Event struct {
	WorkflowID ir.WorkflowID
	NodeID     ir.NodeID
	Path       executionpath.Path
	Type       string
	Payload    any
	Time       time.Time
}

type RunResult struct {
	WorkflowID ir.WorkflowID
	Status     steps.Status
	Nodes      map[ir.NodeID]*steps.RunResult
	Outputs    map[string]steps.Value
}

func NewExecutor(workflow *ir.Workflow, capabilities steps.Capabilities) *Executor {
	store := NewMemoryStateStore()
	exec := &Executor{Workflow: workflow, Context: NewContextStore(), Capabilities: capabilities, Store: store}
	if exec.Capabilities.Workflows == nil {
		exec.Capabilities.Workflows = executorWorkflowRunner{executor: exec}
	}
	exec.SeedEnvironment("")
	return exec
}

func (e *Executor) Run(ctx context.Context) (out *RunResult, err error) {
	if e.Workflow == nil {
		return nil, fmt.Errorf("workflow is required")
	}
	if e.Store == nil {
		e.Store = NewMemoryStateStore()
	}
	if !e.Nested {
		err = e.Store.StartWorkflow(e.stateWorkflowID())
	}
	if err != nil {
		return nil, err
	}
	e.emit("", "workflow.started", nil)
	order, err := PlanTopological(e.Workflow.Graph)
	if err != nil {
		return nil, err
	}
	out = &RunResult{WorkflowID: e.Workflow.ID, Status: steps.StatusCompleted, Nodes: map[ir.NodeID]*steps.RunResult{}, Outputs: map[string]steps.Value{}}
	var workspace *workspaceSession
	defer func() {
		if workspace == nil || out == nil || out.Status == steps.StatusWaiting {
			return
		}
		if cleanupErr := e.finalizeWorkspace(workspace); cleanupErr != nil {
			if out.Status == steps.StatusCompleted {
				out.Status = steps.StatusFailed
				e.finishWorkflow(steps.StatusFailed)
			}
			if err == nil {
				err = cleanupErr
			} else {
				err = fmt.Errorf("%w; workspace cleanup failed: %v", err, cleanupErr)
			}
		}
	}()
	workspace, err = e.prepareWorkspace(ctx)
	if err != nil {
		out.Status = steps.StatusFailed
		e.finishWorkflow(steps.StatusFailed)
		return out, err
	}
	if workspace != nil && strings.TrimSpace(workspace.path) != "" {
		e.emit("", "workflow.workspace.ready", map[string]string{"path": workspace.path, "invocation_cwd": workspace.invocationWD})
	}
	for _, nodeID := range order {
		if err := ctx.Err(); err != nil {
			out.Status = steps.StatusFailed
			e.emit(nodeID, "step.interrupted", map[string]string{"error": err.Error()})
			e.finishWorkflow(steps.StatusFailed)
			return out, err
		}
		node := e.Workflow.Graph.Nodes[nodeID]
		if node == nil || node.Step == nil {
			continue
		}
		if state, ok, err := e.Store.GetStep(e.stateWorkflowID(), e.runtimeNodeID(nodeID)); err != nil {
			return out, err
		} else if ok && state.Status == steps.StatusCompleted {
			if state.Result != nil {
				state.Result.NormalizeOutputs()
			}
			out.Nodes[nodeID] = state.Result
			e.rememberStepOutput(node.Step, state.Result)
			continue
		}
		shouldRun, err := shouldRunStep(node.Step.Meta().Condition, e.Context)
		if err != nil {
			out.Status = steps.StatusFailed
			res := &steps.RunResult{Status: steps.StatusFailed, Error: &steps.StepError{Message: err.Error()}}
			out.Nodes[nodeID] = res
			e.saveStep(StepState{WorkflowID: e.Workflow.ID, NodeID: nodeID, Status: steps.StatusFailed, Result: res, UpdatedAt: time.Now(), CompletedAt: time.Now()})
			e.emit(nodeID, "step.failed", res)
			e.finishWorkflow(steps.StatusFailed)
			return out, err
		}
		if !shouldRun {
			res := &steps.RunResult{Status: steps.StatusSkipped}
			out.Nodes[nodeID] = res
			e.saveStep(StepState{WorkflowID: e.Workflow.ID, NodeID: nodeID, Status: steps.StatusSkipped, Result: res, UpdatedAt: time.Now(), CompletedAt: time.Now()})
			e.emit(nodeID, "step.skipped", res)
			continue
		}
		exec, ok := node.Step.(steps.Executable)
		if !ok {
			continue
		}
		started := time.Now()
		e.saveStep(StepState{WorkflowID: e.Workflow.ID, NodeID: nodeID, Status: "running", StartedAt: started, UpdatedAt: started})
		e.emit(nodeID, "step.started", nil)
		stepToRun := node.Step
		res, err := exec.Run(ctx, e.stepRunRequest(nodeID, stepToRun))
		if res == nil {
			res = &steps.RunResult{}
		}
		res.NormalizeOutputs()
		out.Nodes[nodeID] = res
		if err != nil || res.Status == steps.StatusFailed {
			if repairedRes, repairedErr, ok := e.tryFixAndRerun(ctx, nodeID, node.Step, res, err, nil); ok {
				res, err = repairedRes, repairedErr
				out.Nodes[nodeID] = res
				if err == nil && res != nil && res.Status != steps.StatusFailed {
					goto handleStepResult
				}
			}
			out.Status = steps.StatusFailed
			if err != nil && ctx.Err() != nil {
				if res.Error == nil {
					res.Error = &steps.StepError{Message: ctx.Err().Error(), Cause: ctx.Err()}
				}
				e.saveStep(StepState{WorkflowID: e.Workflow.ID, NodeID: nodeID, Status: steps.StatusFailed, Result: res, StartedAt: started, UpdatedAt: time.Now(), CompletedAt: time.Now()})
				e.emit(nodeID, "step.interrupted", res)
				e.finishWorkflow(steps.StatusFailed)
				return out, ctx.Err()
			}
			e.saveStep(StepState{WorkflowID: e.Workflow.ID, NodeID: nodeID, Status: steps.StatusFailed, Result: res, StartedAt: started, UpdatedAt: time.Now(), CompletedAt: time.Now()})
			e.emit(nodeID, "step.failed", res)
			e.finishWorkflow(steps.StatusFailed)
			if err != nil {
				return out, err
			}
			return out, res.Error
		}
	handleStepResult:
		if res.Status == steps.StatusWaiting {
			out.Status = steps.StatusWaiting
			e.saveStep(StepState{WorkflowID: e.Workflow.ID, NodeID: nodeID, Status: steps.StatusWaiting, Result: res, StartedAt: started, UpdatedAt: time.Now()})
			e.emit(nodeID, "step.waiting", res.Await)
			e.finishWorkflow(steps.StatusWaiting)
			return out, nil
		}
		normalizeStepOutputForContext(node.Step, res)
		var validationErr error
		if !e.isPreview() {
			validationErr = validateStepOutput(node.Step, res.Output)
		}
		if validationErr != nil {
			if repairedRes, repairedErr, ok := e.tryFixAndRerun(ctx, nodeID, node.Step, res, nil, validationErr); ok {
				res, err = repairedRes, repairedErr
				if res == nil {
					res = &steps.RunResult{}
				}
				res.NormalizeOutputs()
				out.Nodes[nodeID] = res
				if err != nil || res.Status == steps.StatusFailed {
					out.Status = steps.StatusFailed
					e.saveStep(StepState{WorkflowID: e.Workflow.ID, NodeID: nodeID, Status: steps.StatusFailed, Result: res, StartedAt: started, UpdatedAt: time.Now(), CompletedAt: time.Now()})
					e.emit(nodeID, "step.failed", res)
					e.finishWorkflow(steps.StatusFailed)
					if err != nil {
						return out, err
					}
					return out, res.Error
				}
				if res.Status == steps.StatusWaiting {
					out.Status = steps.StatusWaiting
					e.saveStep(StepState{WorkflowID: e.Workflow.ID, NodeID: nodeID, Status: steps.StatusWaiting, Result: res, StartedAt: started, UpdatedAt: time.Now()})
					e.emit(nodeID, "step.waiting", res.Await)
					e.finishWorkflow(steps.StatusWaiting)
					return out, nil
				}
				normalizeStepOutputForContext(node.Step, res)
				validationErr = validateStepOutput(node.Step, res.Output)
			}
			if validationErr == nil {
				e.rememberStepOutput(node.Step, res)
				e.saveStep(StepState{WorkflowID: e.Workflow.ID, NodeID: nodeID, Status: steps.StatusCompleted, Result: res, StartedAt: started, UpdatedAt: time.Now(), CompletedAt: time.Now()})
				e.emit(nodeID, "step.completed", res)
				continue
			}
			out.Status = steps.StatusFailed
			res.Status = steps.StatusFailed
			res.Error = &steps.StepError{Message: "step output validation failed", Cause: validationErr}
			e.saveStep(StepState{WorkflowID: e.Workflow.ID, NodeID: nodeID, Status: steps.StatusFailed, Result: res, StartedAt: started, UpdatedAt: time.Now(), CompletedAt: time.Now()})
			e.emit(nodeID, "step.failed", res)
			e.finishWorkflow(steps.StatusFailed)
			return out, res.Error
		}
		e.rememberStepOutput(node.Step, res)
		e.saveStep(StepState{WorkflowID: e.Workflow.ID, NodeID: nodeID, Status: steps.StatusCompleted, Result: res, StartedAt: started, UpdatedAt: time.Now(), CompletedAt: time.Now()})
		e.emit(nodeID, "step.completed", res)
	}
	if err := e.resolveWorkflowOutputs(out); err != nil {
		out.Status = steps.StatusFailed
		e.finishWorkflow(steps.StatusFailed)
		e.emit("", "workflow.failed", map[string]string{"error": err.Error()})
		return out, err
	}
	e.finishWorkflow(steps.StatusCompleted)
	e.emit("", "workflow.completed", out)
	return out, nil
}

func (e *Executor) resolveWorkflowOutputs(out *RunResult) error {
	if e == nil || e.Workflow == nil || out == nil || len(e.Workflow.Outputs) == 0 {
		return nil
	}
	if out.Outputs == nil {
		out.Outputs = map[string]steps.Value{}
	}
	names := make([]string, 0, len(e.Workflow.Outputs))
	for name := range e.Workflow.Outputs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		output := e.Workflow.Outputs[name]
		from := strings.TrimSpace(output.From)
		value, ok := e.Context.Get(from)
		if !ok {
			if output.Required && !e.isPreview() {
				return fmt.Errorf("required workflow output %q was not produced from context path %q", name, from)
			}
			continue
		}
		if output.Type != "" {
			value.Type = output.Type
		}
		out.Outputs[name] = value
	}
	return nil
}

func scriptStepValue(step steps.Step) (steps.ScriptStep, bool) {
	switch s := step.(type) {
	case steps.ScriptStep:
		return s, true
	case *steps.ScriptStep:
		if s == nil {
			return steps.ScriptStep{}, false
		}
		return *s, true
	default:
		return steps.ScriptStep{}, false
	}
}

func (e *Executor) stepRunRequest(nodeID ir.NodeID, step steps.Step) steps.RunRequest {
	return steps.RunRequest{RunID: string(e.Workflow.ID), NodeID: string(nodeID), Step: step, Inputs: steps.InputMap(e.Context.Snapshot()), Context: e.Context, Outputs: e.Context, Capabilities: e.Capabilities, Emit: func(childNodeID string, eventType string, payload any) {
		e.emit(ir.NodeID(childNodeID), eventType, payload)
	}}
}

func stepAllowsFix(step steps.Step) bool {
	meta := step.Meta()
	if meta.Idempotent {
		return true
	}
	switch meta.Kind {
	case steps.KindAgent, steps.KindExternalAgent:
		return true
	default:
		return false
	}
}

const maxFixAttempts = 3

func (e *Executor) tryFixAndRerun(ctx context.Context, nodeID ir.NodeID, step steps.Step, res *steps.RunResult, runErr error, validationErr error) (*steps.RunResult, error, bool) {
	if !stepAllowsFix(step) {
		e.recordRepair(nodeID, RepairRecord{
			StepID:            string(nodeID),
			Kind:              string(step.Meta().Kind),
			Attempt:           0,
			Status:            "skipped_non_idempotent",
			Reason:            "automatic repair skipped because this step is non-idempotent; retrying it could duplicate side effects such as sending a message",
			FormulaUpdateHint: "This repair record is informational and cannot be fixed by clicking it. Fix the external cause, then start a new run. Only mark the step idempotent if it is safe to retry automatically.",
		})
		e.emit(nodeID, "fix.skipped", map[string]any{
			"reason": "non_idempotent",
			"kind":   string(step.Meta().Kind),
		})
		return res, runErr, false
	}
	fixer, ok := defaultFixers.Lookup(step.Meta().Kind)
	if !ok {
		return res, runErr, false
	}
	currentStep := step
	currentRes := res
	currentRunErr := runErr
	currentValidationErr := validationErr
	tried := false
	for attempt := 1; attempt <= maxFixAttempts; attempt++ {
		emit := e.stepRunRequest(nodeID, currentStep).Emit
		fixedStep, report, fixErr := fixer.Fix(ctx, FixContext{
			NodeID:        nodeID,
			Step:          currentStep,
			Attempt:       attempt,
			RunErr:        currentRunErr,
			ValidationErr: currentValidationErr,
			Output:        currentRes.Output,
			Capabilities:  e.Capabilities,
			Context:       e.Context,
			Emit:          emit,
		})
		if fixErr != nil {
			e.recordRepair(nodeID, buildRepairRecord(currentStep, attempt, "fix_error", report, fixErr))
			e.emit(nodeID, "fix.failed", map[string]any{"error": fixErr.Error()})
			return currentRes, currentRunErr, tried
		}
		if fixedStep == nil {
			e.recordRepair(nodeID, buildRepairRecord(currentStep, attempt, "no_fix", report, currentRunErr))
			return currentRes, currentRunErr, tried
		}
		retryExec, ok := fixedStep.(steps.Executable)
		if !ok {
			e.recordRepair(nodeID, buildRepairRecord(currentStep, attempt, "invalid_fix", report, nil))
			return currentRes, currentRunErr, tried
		}
		tried = true
		retryRes, retryErr := retryExec.Run(ctx, e.stepRunRequest(nodeID, fixedStep))
		if retryRes == nil {
			retryRes = &steps.RunResult{}
		}
		retryRes.NormalizeOutputs()
		if retryRes.Status == steps.StatusWaiting {
			e.recordRepair(nodeID, buildRepairRecord(fixedStep, attempt, "waiting", report, retryErr))
			return retryRes, retryErr, true
		}
		validationAfterRetry := error(nil)
		if retryErr == nil && retryRes.Status != steps.StatusFailed {
			validationAfterRetry = validateStepOutput(step, retryRes.Output)
		}
		if retryErr == nil && retryRes.Status != steps.StatusFailed && validationAfterRetry == nil {
			e.recordRepair(nodeID, buildRepairRecord(fixedStep, attempt, "succeeded", report, nil))
			return retryRes, retryErr, true
		}
		status := "attempt_failed"
		if attempt == maxFixAttempts {
			status = "exhausted"
		}
		attemptErr := retryErr
		if attemptErr == nil && validationAfterRetry != nil {
			attemptErr = validationAfterRetry
		}
		currentStep = fixedStep
		currentRes = retryRes
		currentRunErr = retryErr
		currentValidationErr = validationAfterRetry
		e.recordRepair(nodeID, buildRepairRecord(fixedStep, attempt, status, report, attemptErr))
	}
	return currentRes, currentRunErr, tried
}

func buildRepairRecord(step steps.Step, attempt int, status string, report FixReport, err error) RepairRecord {
	record := RepairRecord{
		StepID:            string(step.Meta().ID),
		Kind:              string(step.Meta().Kind),
		Attempt:           attempt,
		Status:            status,
		Reason:            report.Reason,
		FormulaUpdateHint: report.FormulaUpdateHint,
		NextAttemptHint:   report.NextAttemptHint,
		Advice:            report.Advice,
		OriginalCommand:   append([]string(nil), report.OriginalCommand...),
		FixedCommand:      append([]string(nil), report.FixedCommand...),
	}
	if err != nil {
		record.Error = err.Error()
	}
	return record
}

func (e *Executor) recordRepair(nodeID ir.NodeID, record RepairRecord) {
	if e == nil || e.Store == nil || e.Workflow == nil {
		return
	}
	if strings.TrimSpace(record.StepID) == "" {
		record.StepID = string(nodeID)
	}
	record.StepID = string(e.runtimeNodeID(ir.NodeID(record.StepID)))
	_ = e.Store.SaveRepair(e.stateWorkflowID(), record)
	e.emit(nodeID, "step.repair.recorded", record)
}

func normalizeStepOutputForContext(step steps.Step, res *steps.RunResult) {
	if res == nil {
		return
	}
	if validation := outputValidationForStep(step); validation != nil && strings.ToLower(strings.TrimSpace(validation.Format)) == "json" {
		if normalized, ok := normalizeExternalAgentJSONText(res.Output.Raw, validation); ok {
			res.SetPrimaryOutput(steps.Value{Type: "json", Raw: normalized})
			return
		}
	}
	script, ok := scriptStepValue(step)
	if !ok || script.Validation == nil || strings.ToLower(strings.TrimSpace(script.Validation.Format)) != "json" {
		return
	}
	normalized, ok := normalizeScriptJSONStdout(res.Output.Raw)
	if !ok {
		return
	}
	res.SetPrimaryOutput(steps.Value{Type: "json", Raw: normalized})
}

func normalizeExternalAgentJSONText(raw []byte, validation *steps.OutputValidationSpec) ([]byte, bool) {
	var wrapper map[string]any
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, false
	}
	text, ok := wrapper["text"].(string)
	if !ok || strings.TrimSpace(text) == "" {
		return nil, false
	}
	for _, candidate := range jsonTextCandidatesForSpec(text, validation) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		var decoded any
		if err := json.Unmarshal([]byte(candidate), &decoded); err != nil {
			continue
		}
		if err := validateDecodedStepOutput(decoded, validation); err != nil {
			continue
		}
		data, err := json.Marshal(decoded)
		if err != nil {
			return nil, false
		}
		return data, true
	}
	return nil, false
}

func normalizeScriptJSONStdout(raw []byte) ([]byte, bool) {
	var wrapper map[string]any
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, false
	}
	stdout, ok := wrapper["stdout"].(string)
	if !ok {
		return nil, false
	}
	stdout = strings.TrimSpace(stdout)
	if stdout == "" {
		return nil, false
	}
	var decoded any
	if err := json.Unmarshal([]byte(stdout), &decoded); err != nil {
		return nil, false
	}
	wrapper["stdout_text"] = wrapper["stdout"]
	wrapper["stdout"] = decoded
	if obj, ok := decoded.(map[string]any); ok {
		for key, value := range obj {
			if _, exists := wrapper[key]; !exists {
				wrapper[key] = value
			}
		}
	}
	data, err := json.Marshal(wrapper)
	if err != nil {
		return nil, false
	}
	return data, true
}

func validateStepOutput(step steps.Step, out steps.Value) error {
	validationSpec := outputValidationForStep(step)
	if validationSpec == nil {
		return nil
	}
	format := strings.ToLower(strings.TrimSpace(validationSpec.Format))
	if format == "" && (len(validationSpec.Required) > 0 || len(validationSpec.ItemRequired) > 0 || validationSpec.MinItems > 0) {
		format = "json"
	}
	if format != "json" {
		return nil
	}

	decodedValues, err := decodedOutputCandidates(out.Raw, validationSpec)
	if err != nil {
		return fmt.Errorf("output must be valid JSON: %w", err)
	}
	var validationErr error
	for _, decoded := range decodedValues {
		if err := validateDecodedStepOutput(decoded, validationSpec); err == nil {
			return nil
		} else if validationErr == nil {
			validationErr = err
		}
	}
	return validationErr
}

func validateDecodedStepOutput(decoded any, validationSpec *steps.OutputValidationSpec) error {
	if len(validationSpec.Required) > 0 {
		obj, ok := decoded.(map[string]any)
		if !ok {
			return fmt.Errorf("output must be a JSON object with required fields %v", validationSpec.Required)
		}
		if err := validateRequiredFields(obj, validationSpec.Required, "output"); err != nil {
			return err
		}
	}
	if validationSpec.MinItems > 0 || len(validationSpec.ItemRequired) > 0 {
		items, ok := decoded.([]any)
		if !ok {
			return fmt.Errorf("output must be a JSON array")
		}
		if len(items) < validationSpec.MinItems {
			return fmt.Errorf("output array must contain at least %d item(s), got %d", validationSpec.MinItems, len(items))
		}
		for i, item := range items {
			obj, ok := item.(map[string]any)
			if !ok {
				return fmt.Errorf("output[%d] must be a JSON object", i)
			}
			if err := validateRequiredFields(obj, validationSpec.ItemRequired, fmt.Sprintf("output[%d]", i)); err != nil {
				return err
			}
		}
	}
	return nil
}

func decodedOutputCandidates(raw []byte, validationSpec *steps.OutputValidationSpec) ([]any, error) {
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	text, ok := decoded.(string)
	if !ok {
		return []any{decoded}, nil
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return []any{decoded}, nil
	}
	candidates := jsonTextCandidatesForSpec(text, validationSpec)
	out := make([]any, 0, len(candidates)+1)
	seen := map[string]bool{}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		var value any
		if err := json.Unmarshal([]byte(candidate), &value); err == nil {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		out = append(out, decoded)
	}
	return out, nil
}

func normalizeDecodedJSON(value any) any {
	text, ok := value.(string)
	if !ok {
		return value
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return value
	}
	for _, candidate := range jsonTextCandidates(text) {
		var decoded any
		if err := json.Unmarshal([]byte(candidate), &decoded); err == nil {
			return decoded
		}
	}
	return value
}

func jsonTextCandidates(text string) []string {
	return jsonTextCandidatesForSpec(text, nil)
}

func jsonTextCandidatesForSpec(text string, validationSpec *steps.OutputValidationSpec) []string {
	candidates := []string{text}
	candidates = append(candidates, extractFencedJSONBlocks(text)...)
	if validationSpec != nil && len(validationSpec.Required) > 0 {
		candidates = append(candidates, extractBalancedJSONContainers(text, '{', '}')...)
	}
	if validationSpec != nil && (validationSpec.MinItems > 0 || len(validationSpec.ItemRequired) > 0) {
		candidates = append(candidates, extractBalancedJSONContainers(text, '[', ']')...)
	}
	candidates = append(candidates, extractBalancedJSONContainers(text, '{', '}')...)
	candidates = append(candidates, extractBalancedJSONContainers(text, '[', ']')...)
	return candidates
}

func extractFencedJSON(text string) (string, bool) {
	blocks := extractFencedJSONBlocks(text)
	if len(blocks) == 0 {
		return "", false
	}
	return blocks[0], true
}

func extractFencedJSONBlocks(text string) []string {
	var blocks []string
	for {
		start := strings.Index(text, "```")
		if start < 0 {
			return blocks
		}
		rest := text[start+3:]
		if newline := strings.Index(rest, "\n"); newline >= 0 {
			rest = rest[newline+1:]
		}
		end := strings.Index(rest, "```")
		if end < 0 {
			return blocks
		}
		blocks = append(blocks, strings.TrimSpace(rest[:end]))
		text = rest[end+3:]
	}
}

func extractFirstJSONContainer(text string) (string, bool) {
	containers := append(extractBalancedJSONContainers(text, '{', '}'), extractBalancedJSONContainers(text, '[', ']')...)
	if len(containers) == 0 {
		return "", false
	}
	return containers[0], true
}

func extractBalancedJSONContainers(text string, open, close byte) []string {
	var out []string
	for i := 0; i < len(text); i++ {
		if text[i] != open {
			continue
		}
		if end, ok := findBalancedJSONContainerEnd(text, i, open, close); ok {
			out = append(out, strings.TrimSpace(text[i:end+1]))
		}
	}
	return out
}

func findBalancedJSONContainerEnd(text string, start int, open, close byte) (int, bool) {
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(text); i++ {
		ch := text[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case ch == '\\':
				escaped = true
			case ch == '"':
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

func outputValidationForStep(step steps.Step) *steps.OutputValidationSpec {
	switch s := step.(type) {
	case steps.AgentStep:
		return s.Validation
	case *steps.AgentStep:
		return s.Validation
	case steps.ScriptStep:
		return s.Validation
	case *steps.ScriptStep:
		return s.Validation
	case steps.ExternalAgentStep:
		return s.Validation
	case *steps.ExternalAgentStep:
		return s.Validation
	case steps.HumanInputStep:
		return s.Validation
	case *steps.HumanInputStep:
		return s.Validation
	case steps.AggregateStep:
		return s.Validation
	case *steps.AggregateStep:
		return s.Validation
	case steps.WriteFilesStep:
		return s.Validation
	case *steps.WriteFilesStep:
		return s.Validation
	case steps.ToolStep:
		return s.Validation
	case *steps.ToolStep:
		return s.Validation
	default:
		return nil
	}
}

func validateRequiredFields(obj map[string]any, fields []string, prefix string) error {
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		value, ok := obj[field]
		if !ok || isMissingRequiredJSONValue(value) {
			return fmt.Errorf("%s.%s is required", prefix, field)
		}
	}
	return nil
}

func isMissingRequiredJSONValue(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(v) == ""
	default:
		return false
	}
}

func (e *Executor) rememberStepOutput(step steps.Step, result *steps.RunResult) {
	if e.Context == nil || step == nil || result == nil {
		return
	}
	result.NormalizeOutputs()
	key := stepOutputKey(step)
	if key == "" {
		return
	}
	for name, value := range result.Outputs {
		if strings.TrimSpace(name) != "" && len(value.Raw) > 0 {
			_ = e.Context.Set(key+"."+name, value)
		}
	}
	if primary, ok := result.PrimaryOutput(); ok {
		_ = e.Context.Set(key, primary)
	}
}

func stepOutputKey(step steps.Step) string {
	key := ""
	switch s := step.(type) {
	case steps.AgentStep:
		key = s.OutputKey
	case *steps.AgentStep:
		key = s.OutputKey
	case steps.ScriptStep:
		key = s.OutputKey
	case *steps.ScriptStep:
		key = s.OutputKey
	case steps.HumanInputStep:
		key = s.OutputKey
	case *steps.HumanInputStep:
		key = s.OutputKey
	case steps.AggregateStep:
		key = s.OutputKey
	case *steps.AggregateStep:
		key = s.OutputKey
	case steps.WriteFilesStep:
		key = s.OutputKey
	case *steps.WriteFilesStep:
		key = s.OutputKey
	case steps.ToolStep:
		key = s.OutputKey
	case *steps.ToolStep:
		key = s.OutputKey
	case steps.FormulaCallStep:
		key = s.OutputKey
	case *steps.FormulaCallStep:
		key = s.OutputKey
	}
	if key != "" {
		return key
	}
	return string(step.Meta().ID)
}

func (e *Executor) saveStep(state StepState) {
	if e.Store != nil {
		if state.Result != nil {
			state.Result.NormalizeOutputs()
		}
		state.WorkflowID = e.stateWorkflowID()
		state.Path = e.executionPath(state.NodeID)
		state.NodeID = ir.NodeID(state.Path.String())
		_ = e.Store.SaveStep(state)
	}
}

func (e *Executor) emit(nodeID ir.NodeID, typ string, payload any) {
	now := time.Now()
	path := e.executionPath(nodeID)
	runtimeNodeID := ir.NodeID(path.String())
	if e.Nested && nodeID == "" {
		typ = strings.Replace(typ, "workflow.", "formula.", 1)
	}
	event := Event{WorkflowID: e.stateWorkflowID(), NodeID: runtimeNodeID, Path: path, Type: typ, Payload: payload, Time: now}
	if e.Store != nil {
		e.saveChildStepTransition(nodeID, runtimeNodeID, path, typ, payload, now)
		_ = e.Store.AppendEvent(event)
	}
	if e.Events != nil {
		e.Events.Emit(event)
	}
}

func (e *Executor) saveChildStepTransition(localNodeID, nodeID ir.NodeID, path executionpath.Path, typ string, payload any, now time.Time) {
	if nodeID == "" || e.Workflow == nil {
		return
	}
	if _, topLevel := e.Workflow.Graph.Nodes[localNodeID]; topLevel {
		return
	}
	state, _, _ := e.Store.GetStep(e.stateWorkflowID(), nodeID)
	state.WorkflowID = e.stateWorkflowID()
	state.NodeID = nodeID
	state.Path = path
	state.UpdatedAt = now
	switch typ {
	case "step.started":
		state.Status = "running"
		state.StartedAt = now
		state.CompletedAt = time.Time{}
		state.Result = nil
	case "step.completed", "step.failed", "step.skipped", "step.interrupted":
		state.Status = childTransitionStatus(typ)
		state.CompletedAt = now
		if result, ok := payload.(*steps.RunResult); ok {
			result.NormalizeOutputs()
			state.Result = result
		}
	case "step.waiting":
		state.Status = steps.StatusWaiting
		if await, ok := payload.(*steps.AwaitRequest); ok {
			state.Result = &steps.RunResult{Status: steps.StatusWaiting, Await: await}
		}
	default:
		return
	}
	_ = e.Store.SaveStep(state)
}

func (e *Executor) stateWorkflowID() ir.WorkflowID {
	if e.StateWorkflowID != "" {
		return e.StateWorkflowID
	}
	if e.Workflow != nil {
		return e.Workflow.ID
	}
	return ""
}

func (e *Executor) executionPath(nodeID ir.NodeID) executionpath.Path {
	local := executionpath.Parse(string(nodeID))
	if len(e.AddressPrefix.Segments) == 0 {
		return local
	}
	return e.AddressPrefix.Append(local)
}

func (e *Executor) runtimeNodeID(nodeID ir.NodeID) ir.NodeID {
	return ir.NodeID(e.executionPath(nodeID).String())
}

func (e *Executor) finishWorkflow(status steps.Status) {
	if e == nil || e.Nested || e.Store == nil {
		return
	}
	_ = e.Store.FinishWorkflow(e.stateWorkflowID(), status)
}

func childTransitionStatus(typ string) steps.Status {
	switch typ {
	case "step.completed":
		return steps.StatusCompleted
	case "step.skipped":
		return steps.StatusSkipped
	default:
		return steps.StatusFailed
	}
}

func PlanTopological(graph ir.Graph) ([]ir.NodeID, error) {
	inDegree := map[ir.NodeID]int{}
	adj := map[ir.NodeID][]ir.NodeID{}
	for id := range graph.Nodes {
		inDegree[id] = 0
	}
	for _, edge := range graph.Edges {
		if _, ok := graph.Nodes[edge.From]; !ok {
			return nil, fmt.Errorf("edge from unknown node %q", edge.From)
		}
		if _, ok := graph.Nodes[edge.To]; !ok {
			return nil, fmt.Errorf("edge to unknown node %q", edge.To)
		}
		adj[edge.From] = append(adj[edge.From], edge.To)
		inDegree[edge.To]++
	}
	var order []ir.NodeID
	for len(inDegree) > 0 {
		var ready []ir.NodeID
		for id, deg := range inDegree {
			if deg == 0 {
				ready = append(ready, id)
			}
		}
		if len(ready) == 0 {
			return nil, fmt.Errorf("workflow graph contains a cycle")
		}
		sort.Slice(ready, func(i, j int) bool { return ready[i] < ready[j] })
		for _, id := range ready {
			order = append(order, id)
			delete(inDegree, id)
			for _, next := range adj[id] {
				if _, ok := inDegree[next]; ok {
					inDegree[next]--
				}
			}
		}
	}
	return order, nil
}
