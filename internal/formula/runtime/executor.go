package runtime

import (
	"context"
	"fmt"
	"sort"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

type Executor struct {
	Workflow     *ir.Workflow
	Context      *ContextStore
	Capabilities steps.Capabilities
	Events       EventSink
}

type EventSink interface{ Emit(Event) }

type Event struct {
	WorkflowID ir.WorkflowID
	NodeID     ir.NodeID
	Type       string
	Payload    any
}

type RunResult struct {
	WorkflowID ir.WorkflowID
	Status     steps.Status
	Nodes      map[ir.NodeID]*steps.RunResult
}

func NewExecutor(workflow *ir.Workflow, capabilities steps.Capabilities) *Executor {
	return &Executor{Workflow: workflow, Context: NewContextStore(), Capabilities: capabilities}
}

func (e *Executor) Run(ctx context.Context) (*RunResult, error) {
	if e.Workflow == nil {
		return nil, fmt.Errorf("workflow is required")
	}
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
		exec, ok := node.Step.(steps.Executable)
		if !ok {
			continue
		}
		e.emit(nodeID, "step.started", nil)
		res, err := exec.Run(ctx, steps.RunRequest{RunID: string(e.Workflow.ID), NodeID: string(nodeID), Step: node.Step, Context: e.Context, Outputs: e.Context, Capabilities: e.Capabilities})
		if res == nil {
			res = &steps.RunResult{}
		}
		out.Nodes[nodeID] = res
		if err != nil || res.Status == steps.StatusFailed {
			out.Status = steps.StatusFailed
			e.emit(nodeID, "step.failed", res)
			if err != nil {
				return out, err
			}
			return out, res.Error
		}
		if res.Status == steps.StatusWaiting {
			out.Status = steps.StatusWaiting
			e.emit(nodeID, "step.waiting", res.Await)
			return out, nil
		}
		e.emit(nodeID, "step.completed", res)
	}
	return out, nil
}

func (e *Executor) emit(nodeID ir.NodeID, typ string, payload any) {
	if e.Events != nil {
		e.Events.Emit(Event{WorkflowID: e.Workflow.ID, NodeID: nodeID, Type: typ, Payload: payload})
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
