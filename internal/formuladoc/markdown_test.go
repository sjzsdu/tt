package formuladoc

import (
	"strings"
	"testing"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

func TestGenerateMermaidGraphExpandsLoopBody(t *testing.T) {
	workflow := &ir.Workflow{ID: "demo", Name: "demo", Graph: ir.NewGraph()}
	loop := steps.LoopStep{
		Base:  steps.Base{Metadata: steps.Metadata{ID: "monitor", Kind: steps.KindLoop, Title: "Monitor"}},
		Until: "done == true",
		Max:   3,
		Body: []steps.Step{
			steps.ScriptStep{Base: steps.Base{Metadata: steps.Metadata{ID: "fetch", Kind: steps.KindScript, Title: "Fetch"}}},
			steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "classify", Kind: steps.KindAgent, Title: "Classify", DependsOn: []steps.ID{"fetch"}}}},
		},
	}
	workflow.Graph.AddNode(&ir.Node{ID: "monitor", Step: loop})

	graph := GenerateMermaidGraph(workflow)
	for _, want := range []string{"subgraph monitor_loop", "monitor__fetch", "monitor__classify", "monitor__fetch --> monitor__classify"} {
		if !strings.Contains(graph, want) {
			t.Fatalf("graph missing %q:\n%s", want, graph)
		}
	}
}

func TestGenerateMermaidGraphEscapesMustacheLabels(t *testing.T) {
	workflow := &ir.Workflow{ID: "demo", Name: "demo", Graph: ir.NewGraph()}
	loop := steps.LoopStep{
		Base: steps.Base{Metadata: steps.Metadata{ID: "write-articles", Kind: steps.KindLoop, Title: "并发撰写每篇系列文章"}},
		Body: []steps.Step{
			steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "draft", Kind: steps.KindAgent, Title: "撰写 {{article.title}}"}}},
		},
	}
	workflow.Graph.AddNode(&ir.Node{ID: "write-articles", Step: loop})

	graph := GenerateMermaidGraph(workflow)
	if strings.Contains(graph, "{{article.title}}") {
		t.Fatalf("graph contains unescaped mustache label:\n%s", graph)
	}
	if want := `write_articles__draft["撰写 ((article.title))"]`; !strings.Contains(graph, want) {
		t.Fatalf("graph missing escaped label %q:\n%s", want, graph)
	}
}
