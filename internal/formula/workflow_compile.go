package formula

import (
	"github.com/sjzsdu/tt/internal/formula/compile"
	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/schema"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

// CompileWorkflow decodes a kind-discriminated formula document and compiles it
// directly into the graph-first typed Workflow IR. This is the new preferred
// formula pipeline; legacy Compile still returns Recipe for older command paths.
func CompileWorkflow(name string, data []byte, registry *steps.Registry) (*ir.Workflow, error) {
	doc, err := schema.Decode(name, data)
	if err != nil {
		return nil, err
	}
	return compile.New(registry).Compile(doc)
}

func CompileWorkflowFile(path string, registry *steps.Registry) (*ir.Workflow, error) {
	doc, err := schema.LoadFile(path)
	if err != nil {
		return nil, err
	}
	return compile.New(registry).Compile(doc)
}
