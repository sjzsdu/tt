package formula

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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

	recipe, err := Compile(context.Background(), "parent", []string{dir}, nil)
	if err != nil {
		t.Fatalf("Compile(parent) error = %v", err)
	}

	afterID := "parent.after"
	boundaryID := "parent.embedded"
	leftExitID := "parent.embedded.left-exit"
	rightExitID := "parent.embedded.right-exit"

	if hasBlockDep(recipe, afterID, boundaryID) {
		t.Fatalf("after step should not depend on noop embed boundary %q", boundaryID)
	}
	for _, want := range []string{leftExitID, rightExitID} {
		if !hasBlockDep(recipe, afterID, want) {
			t.Fatalf("after step missing dependency on embedded exit %q; deps=%+v", want, recipe.Deps)
		}
	}
	if !hasBlockDep(recipe, "parent.embedded.entry", boundaryID) {
		t.Fatalf("embedded entry should still depend on visible boundary %q", boundaryID)
	}
}

func hasBlockDep(recipe *Recipe, stepID, dependsOnID string) bool {
	for _, dep := range recipe.Deps {
		if dep.StepID == stepID && dep.DependsOnID == dependsOnID && dep.Type == "blocks" {
			return true
		}
	}
	return false
}
