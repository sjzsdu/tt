package doc

import (
	"strings"
	"testing"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/spec"
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
	for _, want := range []string{`subgraph monitor_loop ["monitor loop body"]`, "monitor__fetch", "monitor__classify", "monitor__fetch --> monitor__classify"} {
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

func TestGenerateMermaidGraphShowsDeclaredVariableConsumers(t *testing.T) {
	workflow := &ir.Workflow{
		ID:    "github-repo-docs",
		Name:  "github-repo-docs",
		Vars:  map[string]ir.VarSchema{"repo": {Required: true}, "docs": {}},
		Graph: ir.NewGraph(),
	}
	workflow.Graph.AddNode(&ir.Node{ID: "prepare-repo", Step: steps.ScriptStep{
		Base:    steps.Base{Metadata: steps.Metadata{ID: "prepare-repo", Kind: steps.KindScript, Title: "Prepare repo"}},
		Command: []string{"bash", "-lc", "echo {{repo}} {{prepare-repo.stdout.path}}"},
		Env:     map[string]string{"REPO": "{{repo}}"},
	}})
	workflow.Graph.AddNode(&ir.Node{ID: "scope-analysis", Step: steps.AgentStep{
		Base:   steps.Base{Metadata: steps.Metadata{ID: "scope-analysis", Kind: steps.KindAgent, Title: "Scope"}},
		Prompt: "Analyze {{repo}} and {{prepare-repo.stdout.path}}",
	}})
	workflow.Graph.AddNode(&ir.Node{ID: "write-docs", Step: steps.LoopStep{
		Base:    steps.Base{Metadata: steps.Metadata{ID: "write-docs", Kind: steps.KindLoop, Title: "Write docs"}},
		ForEach: "{{docs}}",
		Var:     "doc",
		Body: []steps.Step{
			steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "draft", Kind: steps.KindAgent, Title: "Draft"}}, Prompt: "Write {{doc.title}} for {{repo}}"},
		},
	}})

	graph := GenerateMermaidGraph(workflow)
	for _, want := range []string{
		`var__docs["$ docs"]`,
		`var__repo["$ repo"]`,
		`var__repo -. var .-> prepare_repo`,
		`var__repo -. var .-> scope_analysis`,
		`var__docs -. var .-> write_docs`,
		`var__repo -. var .-> write_docs__draft`,
	} {
		if !strings.Contains(graph, want) {
			t.Fatalf("graph missing %q:\n%s", want, graph)
		}
	}
	for _, notWant := range []string{`var__doc[`, `var__prepare_repo[`} {
		if strings.Contains(graph, notWant) {
			t.Fatalf("graph should not contain %q for local loop vars or step output refs:\n%s", notWant, graph)
		}
	}
}

func TestGenerateMarkdownUsesExecutionOrderAndFoldsScripts(t *testing.T) {
	f := &spec.Formula{
		Formula: "demo",
		Version: 1,
		Type:    spec.TypeWorkflow,
		Steps: []*spec.Step{
			{ID: "first", Title: "First"},
			{ID: "second", Title: "Second", DependsOn: []string{"first"}},
			{ID: "third", Title: "Third", DependsOn: []string{"second"}},
		},
	}
	workflow := &ir.Workflow{ID: "demo", Name: "demo", Graph: ir.NewGraph()}
	workflow.Graph.AddNode(&ir.Node{ID: "third", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "third", Kind: steps.KindAgent, Title: "Third"}}}})
	workflow.Graph.AddNode(&ir.Node{ID: "first", Step: steps.ScriptStep{Base: steps.Base{Metadata: steps.Metadata{ID: "first", Kind: steps.KindScript, Title: "First"}}, Command: []string{"bash", "-lc", "echo one\necho two"}}})
	workflow.Graph.AddNode(&ir.Node{ID: "second", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "second", Kind: steps.KindAgent, Title: "Second"}}}})
	workflow.Graph.AddEdge("first", "second", "depends_on")
	workflow.Graph.AddEdge("second", "third", "depends_on")

	markdown := GenerateMarkdown(f, workflow)
	firstPos := strings.Index(markdown, "### 1. `first`")
	secondPos := strings.Index(markdown, "### 2. `second`")
	thirdPos := strings.Index(markdown, "### 3. `third`")
	if firstPos < 0 || secondPos < 0 || thirdPos < 0 || !(firstPos < secondPos && secondPos < thirdPos) {
		t.Fatalf("steps are not rendered in execution order:\n%s", markdown)
	}
	for _, want := range []string{"**Command summary:** `bash -lc (2 lines)`", "<details>", "<summary>Show script command</summary>", "echo one\necho two"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown missing %q:\n%s", want, markdown)
		}
	}
}
