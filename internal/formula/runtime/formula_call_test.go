package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

func TestExecutorRunsFormulaCallWithHierarchicalState(t *testing.T) {
	parentGraph := ir.NewGraph()
	parentGraph.AddNode(&ir.Node{ID: "invoke", Step: steps.FormulaCallStep{
		Base:    steps.Base{Metadata: steps.Metadata{ID: "invoke", Kind: steps.KindFormula}},
		Formula: "child",
		With:    map[string]string{"topic": "{{request}}"},
	}})
	parent := &ir.Workflow{
		ID: "parent", Name: "parent", Graph: parentGraph,
		Outputs: map[string]ir.OutputSchema{"answer": {From: "invoke.answer", Required: true}},
	}

	childGraph := ir.NewGraph()
	childGraph.AddNode(&ir.Node{ID: "produce", Step: steps.AgentStep{
		Base: steps.Base{Metadata: steps.Metadata{ID: "produce", Kind: steps.KindAgent}}, OutputKey: "result",
	}})
	child := &ir.Workflow{
		ID: "child", Name: "child", Graph: childGraph,
		Outputs: map[string]ir.OutputSchema{"answer": {From: "result", Required: true}},
	}

	exec := NewExecutor(parent, steps.Capabilities{Agents: fixedOutputAgent{raw: `{"message":"done"}`}})
	exec.SeedValues(map[string]steps.Value{"request": {Type: "json", Raw: json.RawMessage(`{"id":7}`)}})
	exec.ResolveWorkflow = func(_ context.Context, name string, inputs map[string]steps.Value) (*ir.Workflow, error) {
		if name != "child" || string(inputs["topic"].Raw) != `{"id":7}` {
			t.Fatalf("child request name=%q inputs=%s", name, inputs["topic"].Raw)
		}
		return child, nil
	}
	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != steps.StatusCompleted || string(result.Outputs["answer"].Raw) != `{"message":"done"}` {
		t.Fatalf("result = %+v output=%s", result, result.Outputs["answer"].Raw)
	}
	snapshot, err := exec.Store.Snapshot("parent")
	if err != nil {
		t.Fatal(err)
	}
	childState, ok := snapshot.Steps["invoke.formula(child).produce"]
	if !ok || childState.Status != steps.StatusCompleted {
		t.Fatalf("child state = %+v, ok=%v; all=%#v", childState, ok, snapshot.Steps)
	}
	if got := childState.Path.FormulaPath(); len(got) != 1 || got[0] != "child" {
		t.Fatalf("formula path = %#v", got)
	}
}

func TestExecutorPreviewPropagatesThroughFormulaCall(t *testing.T) {
	parentGraph := ir.NewGraph()
	parentGraph.AddNode(&ir.Node{ID: "invoke", Step: steps.FormulaCallStep{
		Base: steps.Base{Metadata: steps.Metadata{ID: "invoke", Kind: steps.KindFormula}}, Formula: "child",
	}})
	parent := &ir.Workflow{
		ID: "parent", Name: "parent", Graph: parentGraph,
		Outputs: map[string]ir.OutputSchema{"report": {From: "invoke.report", Required: true}},
	}
	childGraph := ir.NewGraph()
	childGraph.AddNode(&ir.Node{ID: "plan", Step: steps.AgentStep{
		Base:       steps.Base{Metadata: steps.Metadata{ID: "plan", Kind: steps.KindAgent}},
		Validation: &steps.OutputValidationSpec{Format: "json", Required: []string{"answer"}},
	}})
	child := &ir.Workflow{
		ID: "child", Name: "child", Graph: childGraph,
		Outputs: map[string]ir.OutputSchema{"report": {From: "never-produced", Required: true}},
	}
	exec := NewExecutor(parent, steps.Capabilities{Agents: fixedOutputAgent{raw: `{"dry_run":true}`}})
	exec.Mode = ExecutionModePreview
	exec.ResolveWorkflow = func(context.Context, string, map[string]steps.Value) (*ir.Workflow, error) { return child, nil }
	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != steps.StatusCompleted || result.Nodes["invoke"].Status != steps.StatusCompleted {
		t.Fatalf("preview FormulaCall result = %+v", result)
	}
	snapshot, err := exec.Store.Snapshot("parent")
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshot.Steps["invoke.formula(child).plan"].Status; got != steps.StatusCompleted {
		t.Fatalf("nested preview status = %q", got)
	}
}

func TestExecutorRejectsRecursiveFormulaCalls(t *testing.T) {
	workflow := func(name, child string) *ir.Workflow {
		graph := ir.NewGraph()
		graph.AddNode(&ir.Node{ID: "call", Step: steps.FormulaCallStep{
			Base: steps.Base{Metadata: steps.Metadata{ID: "call", Kind: steps.KindFormula}}, Formula: child,
		}})
		return &ir.Workflow{ID: ir.WorkflowID(name), Name: name, Graph: graph}
	}
	workflows := map[string]*ir.Workflow{"a": workflow("a", "b"), "b": workflow("b", "a")}
	exec := NewExecutor(workflows["a"], steps.Capabilities{})
	exec.ResolveWorkflow = func(_ context.Context, name string, _ map[string]steps.Value) (*ir.Workflow, error) {
		return workflows[name], nil
	}
	result, err := exec.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "recursive formula call") {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	snapshot, snapshotErr := exec.Store.Snapshot("a")
	if snapshotErr != nil {
		t.Fatal(snapshotErr)
	}
	if snapshot.Status != steps.StatusFailed {
		t.Fatalf("root status = %q", snapshot.Status)
	}
}

func TestExecutorRejectsRecursiveFormulaCallThroughAlternateLookupName(t *testing.T) {
	graph := ir.NewGraph()
	graph.AddNode(&ir.Node{ID: "call", Step: steps.FormulaCallStep{
		Base: steps.Base{Metadata: steps.Metadata{ID: "call", Kind: steps.KindFormula}}, Formula: "alternate-name",
	}})
	workflow := &ir.Workflow{ID: "canonical", Name: "canonical", Graph: graph}
	exec := NewExecutor(workflow, steps.Capabilities{})
	exec.ResolveWorkflow = func(context.Context, string, map[string]steps.Value) (*ir.Workflow, error) {
		return workflow, nil
	}
	_, err := exec.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "recursive formula call detected: canonical -> canonical") {
		t.Fatalf("err = %v, want canonical recursive call error", err)
	}
}

func TestExecutorFormulaCallTargetsNestedWaitingStep(t *testing.T) {
	parentGraph := ir.NewGraph()
	parentGraph.AddNode(&ir.Node{ID: "invoke", Step: steps.FormulaCallStep{
		Base: steps.Base{Metadata: steps.Metadata{ID: "invoke", Kind: steps.KindFormula}}, Formula: "child",
	}})
	childGraph := ir.NewGraph()
	childGraph.AddNode(&ir.Node{ID: "approve", Step: steps.HumanInputStep{
		Base: steps.Base{Metadata: steps.Metadata{ID: "approve", Kind: steps.KindHumanInput}}, Reason: "Approve",
	}})
	child := &ir.Workflow{ID: "child", Name: "child", Graph: childGraph}
	exec := NewExecutor(&ir.Workflow{ID: "parent", Name: "parent", Graph: parentGraph}, steps.Capabilities{})
	exec.ResolveWorkflow = func(context.Context, string, map[string]steps.Value) (*ir.Workflow, error) { return child, nil }
	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wait := result.Nodes["invoke"].Await
	if wait == nil || wait.StepID != "invoke.formula(child).approve" {
		t.Fatalf("await = %+v", wait)
	}
}
