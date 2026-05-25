package runtime

import (
	"context"
	"fmt"
	"sort"
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
	return &Executor{Workflow: workflow, Context: NewContextStore(), Capabilities: capabilities, Store: store}
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
		res, err := exec.Run(ctx, steps.RunRequest{RunID: string(e.Workflow.ID), NodeID: string(nodeID), Step: node.Step, Context: e.Context, Outputs: e.Context, Capabilities: e.Capabilities})
		if res == nil {
			res = &steps.RunResult{}
		}
		out.Nodes[nodeID] = res
		if err != nil || res.Status == steps.StatusFailed {
			out.Status = steps.StatusFailed
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
		e.rememberStepOutput(node.Step, res)
		e.saveStep(StepState{WorkflowID: e.Workflow.ID, NodeID: nodeID, Status: steps.StatusCompleted, Result: res, StartedAt: started, UpdatedAt: time.Now(), CompletedAt: time.Now()})
		e.emit(nodeID, "step.completed", res)
	}
	_ = e.Store.FinishWorkflow(e.Workflow.ID, steps.StatusCompleted)
	e.emit("", "workflow.completed", out)
	return out, nil
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
