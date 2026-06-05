package ui

import (
	"testing"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

func TestBuildWorkflowGraphIncludesTypedLoopBody(t *testing.T) {
	workflow := &ir.Workflow{ID: "demo", Name: "demo", Graph: ir.NewGraph()}
	loop := steps.LoopStep{
		Base:  steps.Base{Metadata: steps.Metadata{ID: "monitor", Kind: steps.KindLoop, Title: "Monitor"}},
		Until: "done == true",
		Max:   5,
		Body: []steps.Step{
			steps.ScriptStep{Base: steps.Base{Metadata: steps.Metadata{ID: "fetch", Kind: steps.KindScript, Title: "Fetch"}}, OutputKey: "fetch_out"},
			steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "classify", Kind: steps.KindAgent, Title: "Classify", DependsOn: []steps.ID{"fetch"}}}, Agent: "coder", InputCtx: []string{"fetch"}},
		},
	}
	workflow.Graph.AddNode(&ir.Node{ID: "monitor", Step: loop})

	uiSteps, _ := BuildWorkflowGraph(workflow)
	if len(uiSteps) != 1 || uiSteps[0].Loop == nil {
		t.Fatalf("expected loop details, got %+v", uiSteps)
	}
	if uiSteps[0].Loop.Summary == "" || len(uiSteps[0].Loop.Body) != 2 {
		t.Fatalf("bad loop details: %+v", uiSteps[0].Loop)
	}
	if uiSteps[0].Loop.Body[1].DependsOn[0] != "fetch" || uiSteps[0].Loop.Body[1].Agent != "coder" {
		t.Fatalf("bad loop body: %+v", uiSteps[0].Loop.Body[1])
	}
}
