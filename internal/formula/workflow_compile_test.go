package formula

import (
	"testing"

	"github.com/sjzsdu/tt/internal/formula/steps"
)

func TestCompileWorkflowUsesTypedPipeline(t *testing.T) {
	wf, err := CompileWorkflow("demo.toml", []byte(`formula = "demo"
[[steps]]
id = "ask"
kind = "human_input"
`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if wf.ID != "demo" {
		t.Fatalf("workflow id = %q", wf.ID)
	}
	if got := wf.Graph.Nodes["ask"].Step.Meta().Kind; got != steps.KindHumanInput {
		t.Fatalf("kind = %s", got)
	}
}
