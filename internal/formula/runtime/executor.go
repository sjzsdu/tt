package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

type Executor struct {
	Workflow     *ir.Workflow
	Context      *ContextStore
	Capabilities steps.Capabilities
	Events       EventSink
	Store        StateStore
}

type EventSink interface{ Emit(Event) }

type Event struct {
	WorkflowID ir.WorkflowID
	NodeID     ir.NodeID
	Type       string
	Payload    any
	Time       time.Time
}

type RunResult struct {
	WorkflowID ir.WorkflowID
	Status     steps.Status
	Nodes      map[ir.NodeID]*steps.RunResult
}

func NewExecutor(workflow *ir.Workflow, capabilities steps.Capabilities) *Executor {
	store := NewMemoryStateStore()
	exec := &Executor{Workflow: workflow, Context: NewContextStore(), Capabilities: capabilities, Store: store}
	exec.SeedEnvironment("")
	return exec
}

func (e *Executor) Run(ctx context.Context) (*RunResult, error) {
	if e.Workflow == nil {
		return nil, fmt.Errorf("workflow is required")
	}
	if e.Store == nil {
		e.Store = NewMemoryStateStore()
	}
	if err := e.Store.StartWorkflow(e.Workflow.ID); err != nil {
		return nil, err
	}
	e.emit("", "workflow.started", nil)
	order, err := PlanTopological(e.Workflow.Graph)
	if err != nil {
		return nil, err
	}
	out := &RunResult{WorkflowID: e.Workflow.ID, Status: steps.StatusCompleted, Nodes: map[ir.NodeID]*steps.RunResult{}}
	for _, nodeID := range order {
		if err := ctx.Err(); err != nil {
			out.Status = steps.StatusFailed
			e.emit(nodeID, "step.interrupted", map[string]string{"error": err.Error()})
			_ = e.Store.FinishWorkflow(e.Workflow.ID, steps.StatusFailed)
			return out, err
		}
		node := e.Workflow.Graph.Nodes[nodeID]
		if node == nil || node.Step == nil {
			continue
		}
		if state, ok, err := e.Store.GetStep(e.Workflow.ID, nodeID); err != nil {
			return out, err
		} else if ok && state.Status == steps.StatusCompleted {
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
			_ = e.Store.FinishWorkflow(e.Workflow.ID, steps.StatusFailed)
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
		res, err := exec.Run(ctx, steps.RunRequest{RunID: string(e.Workflow.ID), NodeID: string(nodeID), Step: node.Step, Context: e.Context, Outputs: e.Context, Capabilities: e.Capabilities, Emit: func(childNodeID string, eventType string, payload any) {
			e.emit(ir.NodeID(childNodeID), eventType, payload)
		}})
		if res == nil {
			res = &steps.RunResult{}
		}
		out.Nodes[nodeID] = res
		if err != nil || res.Status == steps.StatusFailed {
			out.Status = steps.StatusFailed
			if err != nil && ctx.Err() != nil {
				if res.Error == nil {
					res.Error = &steps.StepError{Message: ctx.Err().Error(), Cause: ctx.Err()}
				}
				e.saveStep(StepState{WorkflowID: e.Workflow.ID, NodeID: nodeID, Status: steps.StatusFailed, Result: res, StartedAt: started, UpdatedAt: time.Now(), CompletedAt: time.Now()})
				e.emit(nodeID, "step.interrupted", res)
				_ = e.Store.FinishWorkflow(e.Workflow.ID, steps.StatusFailed)
				return out, ctx.Err()
			}
			e.saveStep(StepState{WorkflowID: e.Workflow.ID, NodeID: nodeID, Status: steps.StatusFailed, Result: res, StartedAt: started, UpdatedAt: time.Now(), CompletedAt: time.Now()})
			e.emit(nodeID, "step.failed", res)
			_ = e.Store.FinishWorkflow(e.Workflow.ID, steps.StatusFailed)
			if err != nil {
				return out, err
			}
			return out, res.Error
		}
		if res.Status == steps.StatusWaiting {
			out.Status = steps.StatusWaiting
			e.saveStep(StepState{WorkflowID: e.Workflow.ID, NodeID: nodeID, Status: steps.StatusWaiting, Result: res, StartedAt: started, UpdatedAt: time.Now()})
			e.emit(nodeID, "step.waiting", res.Await)
			_ = e.Store.FinishWorkflow(e.Workflow.ID, steps.StatusWaiting)
			return out, nil
		}
		if err := validateStepOutput(node.Step, res.Output); err != nil {
			out.Status = steps.StatusFailed
			res.Status = steps.StatusFailed
			res.Error = &steps.StepError{Message: "step output validation failed", Cause: err}
			e.saveStep(StepState{WorkflowID: e.Workflow.ID, NodeID: nodeID, Status: steps.StatusFailed, Result: res, StartedAt: started, UpdatedAt: time.Now(), CompletedAt: time.Now()})
			e.emit(nodeID, "step.failed", res)
			_ = e.Store.FinishWorkflow(e.Workflow.ID, steps.StatusFailed)
			return out, res.Error
		}
		e.rememberStepOutput(node.Step, res)
		e.saveStep(StepState{WorkflowID: e.Workflow.ID, NodeID: nodeID, Status: steps.StatusCompleted, Result: res, StartedAt: started, UpdatedAt: time.Now(), CompletedAt: time.Now()})
		e.emit(nodeID, "step.completed", res)
	}
	_ = e.Store.FinishWorkflow(e.Workflow.ID, steps.StatusCompleted)
	e.emit("", "workflow.completed", out)
	return out, nil
}

func validateStepOutput(step steps.Step, out steps.Value) error {
	spec := outputValidationForStep(step)
	if spec == nil {
		return nil
	}
	format := strings.ToLower(strings.TrimSpace(spec.Format))
	if format == "" && (len(spec.Required) > 0 || len(spec.ItemRequired) > 0 || spec.MinItems > 0) {
		format = "json"
	}
	if format != "json" {
		return nil
	}

	decodedValues, err := decodedOutputCandidates(out.Raw, spec)
	if err != nil {
		return fmt.Errorf("output must be valid JSON: %w", err)
	}
	var validationErr error
	for _, decoded := range decodedValues {
		if err := validateDecodedStepOutput(decoded, spec); err == nil {
			return nil
		} else if validationErr == nil {
			validationErr = err
		}
	}
	return validationErr
}

func validateDecodedStepOutput(decoded any, spec *steps.OutputValidationSpec) error {
	if len(spec.Required) > 0 {
		obj, ok := decoded.(map[string]any)
		if !ok {
			return fmt.Errorf("output must be a JSON object with required fields %v", spec.Required)
		}
		if err := validateRequiredFields(obj, spec.Required, "output"); err != nil {
			return err
		}
	}
	if spec.MinItems > 0 || len(spec.ItemRequired) > 0 {
		items, ok := decoded.([]any)
		if !ok {
			return fmt.Errorf("output must be a JSON array")
		}
		if len(items) < spec.MinItems {
			return fmt.Errorf("output array must contain at least %d item(s), got %d", spec.MinItems, len(items))
		}
		for i, item := range items {
			obj, ok := item.(map[string]any)
			if !ok {
				return fmt.Errorf("output[%d] must be a JSON object", i)
			}
			if err := validateRequiredFields(obj, spec.ItemRequired, fmt.Sprintf("output[%d]", i)); err != nil {
				return err
			}
		}
	}
	return nil
}

func decodedOutputCandidates(raw []byte, spec *steps.OutputValidationSpec) ([]any, error) {
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
	candidates := jsonTextCandidatesForSpec(text, spec)
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

func jsonTextCandidatesForSpec(text string, spec *steps.OutputValidationSpec) []string {
	candidates := []string{text}
	if fenced, ok := extractFencedJSON(text); ok {
		candidates = append(candidates, fenced)
	}
	if spec != nil && len(spec.Required) > 0 {
		if extracted, ok := extractFirstJSONContainerOfType(text, '{', '}'); ok {
			candidates = append(candidates, extracted)
		}
	}
	if spec != nil && (spec.MinItems > 0 || len(spec.ItemRequired) > 0) {
		if extracted, ok := extractFirstJSONContainerOfType(text, '[', ']'); ok {
			candidates = append(candidates, extracted)
		}
	}
	if extracted, ok := extractFirstJSONContainer(text); ok {
		candidates = append(candidates, extracted)
	}
	return candidates
}

func extractFirstJSONContainerOfType(text string, open, close byte) (string, bool) {
	start := strings.IndexByte(text, open)
	if start < 0 {
		return "", false
	}
	end := strings.LastIndexByte(text, close)
	if end <= start {
		return "", false
	}
	return strings.TrimSpace(text[start : end+1]), true
}

func extractFencedJSON(text string) (string, bool) {
	start := strings.Index(text, "```")
	if start < 0 {
		return "", false
	}
	rest := text[start+3:]
	if newline := strings.Index(rest, "\n"); newline >= 0 {
		rest = rest[newline+1:]
	}
	end := strings.Index(rest, "```")
	if end < 0 {
		return "", false
	}
	return strings.TrimSpace(rest[:end]), true
}

func extractFirstJSONContainer(text string) (string, bool) {
	arrayStart := strings.Index(text, "[")
	objectStart := strings.Index(text, "{")
	start := -1
	close := byte(0)
	switch {
	case arrayStart >= 0 && (objectStart < 0 || arrayStart < objectStart):
		start, close = arrayStart, ']'
	case objectStart >= 0:
		start, close = objectStart, '}'
	default:
		return "", false
	}
	end := strings.LastIndexByte(text, close)
	if end <= start {
		return "", false
	}
	return strings.TrimSpace(text[start : end+1]), true
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
	if e.Context == nil || step == nil || result == nil || len(result.Output.Raw) == 0 {
		return
	}
	key := stepOutputKey(step)
	if key == "" {
		return
	}
	_ = e.Context.Set(key, result.Output)
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
	}
	if key != "" {
		return key
	}
	return string(step.Meta().ID)
}

func (e *Executor) saveStep(state StepState) {
	if e.Store != nil {
		_ = e.Store.SaveStep(state)
	}
}

func (e *Executor) emit(nodeID ir.NodeID, typ string, payload any) {
	event := Event{WorkflowID: e.Workflow.ID, NodeID: nodeID, Type: typ, Payload: payload, Time: time.Now()}
	if e.Store != nil {
		_ = e.Store.AppendEvent(event)
	}
	if e.Events != nil {
		e.Events.Emit(event)
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
