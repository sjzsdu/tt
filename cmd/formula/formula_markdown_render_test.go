package formulacmd

import (
	"strings"
	"testing"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

func TestGenerateMermaidGraphRespectsFormulaMarkdownHideGraphVars(t *testing.T) {
	previous := formulaMarkdownHideGraphVars
	t.Cleanup(func() { formulaMarkdownHideGraphVars = previous })

	workflow := &ir.Workflow{
		ID:    "demo",
		Name:  "demo",
		Vars:  map[string]ir.VarSchema{"repo": {Required: true}},
		Graph: ir.NewGraph(),
	}
	workflow.Graph.AddNode(&ir.Node{ID: "inspect", Step: steps.AgentStep{
		Base:   steps.Base{Metadata: steps.Metadata{ID: "inspect", Kind: steps.KindAgent, Title: "Inspect"}},
		Prompt: "Inspect {{repo}}",
	}})

	formulaMarkdownHideGraphVars = false
	withVars := generateMermaidGraph(workflow)
	if !strings.Contains(withVars, `var__repo["$ repo"]`) || !strings.Contains(withVars, `var__repo -. var .-> inspect`) {
		t.Fatalf("graph should include variable node and edge when flag is false:\n%s", withVars)
	}

	formulaMarkdownHideGraphVars = true
	hiddenVars := generateMermaidGraph(workflow)
	for _, notWant := range []string{`var__repo[`, `-. var .->`} {
		if strings.Contains(hiddenVars, notWant) {
			t.Fatalf("graph should hide %q when flag is true:\n%s", notWant, hiddenVars)
		}
	}
	if !strings.Contains(hiddenVars, `inspect["Inspect"]`) {
		t.Fatalf("graph should keep step nodes when hiding variable nodes:\n%s", hiddenVars)
	}
}
