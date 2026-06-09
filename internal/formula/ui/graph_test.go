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

func TestBuildWorkflowGraphTemplateVarRefsUseDeclaredVars(t *testing.T) {
	workflow := &ir.Workflow{
		ID:    "github-repo-docs",
		Name:  "github-repo-docs",
		Vars:  map[string]ir.VarSchema{"repo": {Required: true}},
		Graph: ir.NewGraph(),
	}
	workflow.Graph.AddNode(&ir.Node{ID: "prepare-repo", Step: steps.ScriptStep{
		Base:    steps.Base{Metadata: steps.Metadata{ID: "prepare-repo", Kind: steps.KindScript, Title: "Prepare repo"}},
		Env:     map[string]string{"TT_REPO": "{{repo}}"},
		Command: []string{"echo", "{{repo}}"},
	}})
	workflow.Graph.AddNode(&ir.Node{ID: "scope-analysis", Step: steps.AgentStep{
		Base:   steps.Base{Metadata: steps.Metadata{ID: "scope-analysis", Kind: steps.KindAgent, Title: "Scope"}},
		Prompt: "Analyze {{repo}} and {{prepare-repo.stdout.repo_path}}",
	}})
	workflow.Graph.AddNode(&ir.Node{ID: "write-docs", Step: steps.LoopStep{
		Base:    steps.Base{Metadata: steps.Metadata{ID: "write-docs", Kind: steps.KindLoop, Title: "Write docs"}},
		ForEach: "doc-plan",
		Var:     "doc",
		Body: []steps.Step{
			steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "draft", Kind: steps.KindAgent, Title: "Draft"}}, Prompt: "Write {{doc.title}} for {{repo}}"},
		},
	}})

	uiSteps, _ := BuildWorkflowGraph(workflow)
	byID := map[string]Step{}
	for _, step := range uiSteps {
		byID[step.ID] = step
	}
	if got := byID["prepare-repo"].VarRefs; len(got) != 1 || got[0] != "repo" {
		t.Fatalf("prepare-repo VarRefs = %#v, want [repo]", got)
	}
	if got := byID["scope-analysis"].VarRefs; len(got) != 1 || got[0] != "repo" {
		t.Fatalf("scope-analysis VarRefs = %#v, want [repo]", got)
	}
	loop := byID["write-docs"].Loop
	if loop == nil || len(loop.Body) != 1 {
		t.Fatalf("missing loop body: %+v", byID["write-docs"])
	}
	if got := loop.Body[0].VarRefs; len(got) != 1 || got[0] != "repo" {
		t.Fatalf("loop body VarRefs = %#v, want only [repo] and no local doc", got)
	}
}
