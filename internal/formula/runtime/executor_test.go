package runtime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

type fakeAgent struct{}

func (fakeAgent) RunAgent(context.Context, steps.AgentRequest) (steps.Value, error) {
	raw, _ := json.Marshal("agent-ok")
	return steps.Value{Type: "json", Raw: raw}, nil
}

type fakeScript struct{}

func (fakeScript) RunScript(context.Context, steps.ScriptRequest) (steps.Value, error) {
	raw, _ := json.Marshal("script-ok")
	return steps.Value{Type: "json", Raw: raw}, nil
}

func TestExecutorRunsTypedWorkflowInTopologicalOrder(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "a", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "a", Kind: steps.KindAgent}}}})
	g.AddNode(&ir.Node{ID: "b", Step: steps.ScriptStep{Base: steps.Base{Metadata: steps.Metadata{ID: "b", Kind: steps.KindScript}}}})
	g.AddEdge("a", "b", "blocks")
	wf := &ir.Workflow{ID: "demo", Graph: g}
	exec := NewExecutor(wf, steps.Capabilities{Agents: fakeAgent{}, Scripts: fakeScript{}})
	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != steps.StatusCompleted {
		t.Fatalf("status = %s", result.Status)
	}
	if len(result.Nodes) != 2 {
		t.Fatalf("nodes = %d", len(result.Nodes))
	}
}

func TestExecutorReturnsWaitingForHumanInput(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "ask", Step: steps.HumanInputStep{Base: steps.Base{Metadata: steps.Metadata{ID: "ask", Kind: steps.KindHumanInput}}, Reason: "need input"}})
	wf := &ir.Workflow{ID: "demo", Graph: g}
	result, err := NewExecutor(wf, steps.Capabilities{}).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != steps.StatusWaiting {
		t.Fatalf("status = %s", result.Status)
	}
	if result.Nodes["ask"].Await == nil {
		t.Fatal("missing await request")
	}
}
