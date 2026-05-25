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

type countingAgent struct{ calls int }

func (a *countingAgent) RunAgent(context.Context, steps.AgentRequest) (steps.Value, error) {
	a.calls++
	raw, _ := json.Marshal("fresh")
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

func TestExecutorSkipsCompletedStoredStepsAndRestoresOutputContext(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "a", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "a", Kind: steps.KindAgent}}, OutputKey: "decision"}})
	g.AddNode(&ir.Node{ID: "b", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "b", Kind: steps.KindAgent}}}})
	g.AddEdge("a", "b", "blocks")
	wf := &ir.Workflow{ID: "demo", Graph: g}
	agent := &countingAgent{}
	exec := NewExecutor(wf, steps.Capabilities{Agents: agent})
	raw, _ := json.Marshal("stored")
	if err := exec.Store.SaveStep(StepState{WorkflowID: wf.ID, NodeID: "a", Status: steps.StatusCompleted, Result: &steps.RunResult{Status: steps.StatusCompleted, Output: steps.Value{Type: "json", Raw: raw}}}); err != nil {
		t.Fatal(err)
	}
	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != steps.StatusCompleted {
		t.Fatalf("status = %s", result.Status)
	}
	if agent.calls != 1 {
		t.Fatalf("agent calls = %d, want only the non-stored step", agent.calls)
	}
	value, ok := exec.Context.Get("decision")
	if !ok {
		t.Fatal("missing restored output context")
	}
	var got string
	if err := json.Unmarshal(value.Raw, &got); err != nil {
		t.Fatal(err)
	}
	if got != "stored" {
		t.Fatalf("context = %q", got)
	}
}

func TestExecutorSkipsStepWhenRuntimeConditionIsFalse(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "a", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "a", Kind: steps.KindAgent}}, OutputKey: "decision"}})
	g.AddNode(&ir.Node{ID: "b", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "b", Kind: steps.KindAgent, Condition: "decision.approved == true"}}}})
	g.AddEdge("a", "b", "blocks")
	wf := &ir.Workflow{ID: "demo", Graph: g}
	agent := &countingAgent{}
	exec := NewExecutor(wf, steps.Capabilities{Agents: agent})
	raw, _ := json.Marshal(map[string]any{"approved": false})
	if err := exec.Store.SaveStep(StepState{WorkflowID: wf.ID, NodeID: "a", Status: steps.StatusCompleted, Result: &steps.RunResult{Status: steps.StatusCompleted, Output: steps.Value{Type: "json", Raw: raw}}}); err != nil {
		t.Fatal(err)
	}
	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Nodes["b"].Status != steps.StatusSkipped {
		t.Fatalf("b status = %s, want skipped", result.Nodes["b"].Status)
	}
	if agent.calls != 0 {
		t.Fatalf("agent calls = %d, want skipped target not executed", agent.calls)
	}
}
