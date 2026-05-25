package formula

import (
	"testing"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

func TestWorkflowFromRecipeMapsExecutionKinds(t *testing.T) {
	recipe := &Recipe{Name: "demo", Vars: map[string]*VarDef{}, Steps: []RecipeStep{
		{ID: "demo", Title: "root", IsRoot: true},
		{ID: "demo.start", Title: "start", Execution: "noop"},
		{ID: "demo.ask", Title: "Ask", Execution: "human_input", Description: "Need input"},
		{ID: "demo.script", Title: "Script", Execution: "script", Script: &ScriptSpec{Command: []string{"echo", "ok"}}},
	}}
	wf := WorkflowFromRecipe(recipe)
	if wf == nil {
		t.Fatal("workflow is nil")
	}
	cases := map[string]steps.Kind{"demo.start": steps.KindNoop, "demo.ask": steps.KindHumanInput, "demo.script": steps.KindScript}
	for id, want := range cases {
		node := wf.Graph.Nodes[ir.NodeID(id)]
		if node == nil {
			t.Fatalf("missing node %s", id)
		}
		if got := node.Step.Meta().Kind; got != want {
			t.Fatalf("%s kind = %s, want %s", id, got, want)
		}
	}
}
