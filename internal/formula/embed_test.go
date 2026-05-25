package formula

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sjzsdu/tt/internal/formula/ir"
)

func TestEmbedDownstreamDependsOnEmbeddedExits(t *testing.T) {
	dir := t.TempDir()
	writeFormula := func(name, content string) {
		t.Helper()
		path := filepath.Join(dir, name+".toml")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}

	writeFormula("child", `formula = "child"
version = 1
type = "workflow"

[[steps]]
id = "entry"
title = "Entry"

[[steps]]
id = "left-exit"
title = "Left exit"
depends_on = ["entry"]

[[steps]]
id = "right-exit"
title = "Right exit"
depends_on = ["entry"]
`)

	writeFormula("parent", `formula = "parent"
version = 1
type = "workflow"

[[steps]]
id = "before"
title = "Before"

[[steps]]
id = "embedded"
title = "Embedded child"
depends_on = ["before"]
embed = "child"

[[steps]]
id = "after"
title = "After"
depends_on = ["embedded"]
`)

	workflow, err := CompileWorkflowByName(context.Background(), "parent", []string{dir}, nil)
	if err != nil {
		t.Fatalf("Compile(parent) error = %v", err)
	}

	afterID := "after"
	boundaryID := "embedded"
	leftExitID := "embedded.left-exit"
	rightExitID := "embedded.right-exit"

	if hasBlockDep(workflow, afterID, boundaryID) {
		t.Fatalf("after step should not depend on noop embed boundary %q", boundaryID)
	}
	for _, want := range []string{leftExitID, rightExitID} {
		if !hasBlockDep(workflow, afterID, want) {
			t.Fatalf("after step missing dependency on embedded exit %q; deps=%+v", want, workflow.Graph.Edges)
		}
	}
	if !hasBlockDep(workflow, "embedded.entry", boundaryID) {
		t.Fatalf("embedded entry should still depend on visible boundary %q", boundaryID)
	}
}

func hasBlockDep(workflow *ir.Workflow, stepID, dependsOnID string) bool {
	for _, dep := range workflow.Graph.Edges {
		if string(dep.To) == stepID && string(dep.From) == dependsOnID && dep.Type == "blocks" {
			return true
		}
	}
	return false
}
