package compile

import (
	"testing"

	"github.com/sjzsdu/tt/internal/formula/schema"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

func TestCompilerBuildsTypedWorkflow(t *testing.T) {
	doc, err := schema.Decode("demo.toml", []byte(`formula = "demo"
[[steps]]
id = "a"
kind = "agent"
[[steps]]
id = "b"
kind = "human_input"
depends_on = ["a"]
`))
	if err != nil {
		t.Fatal(err)
	}
	wf, err := New(nil).Compile(doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(wf.Graph.Nodes) != 2 {
		t.Fatalf("nodes = %d", len(wf.Graph.Nodes))
	}
	if got := wf.Graph.Nodes["b"].Step.Meta().Kind; got != steps.KindHumanInput {
		t.Fatalf("kind = %s", got)
	}
	if len(wf.Graph.Edges) != 1 {
		t.Fatalf("edges = %d", len(wf.Graph.Edges))
	}
}
