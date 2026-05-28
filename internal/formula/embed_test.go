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

func TestEmbedInsideRuntimeLoopBodyExpands(t *testing.T) {
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
id = "first"
title = "First"
execution = "script"

[steps.script]
command = ["echo", "ok"]
format = "json"

[[steps]]
id = "last"
title = "Last"
depends_on = ["first"]
`)

	writeFormula("parent", `formula = "parent"
version = 1
type = "workflow"

[[steps]]
id = "loop"
title = "Loop"

  [steps.loop]
  for_each = "items"
  var = "item"

  [[steps.loop.body]]
  id = "embedded"
  title = "Embedded child"
  embed = "child"

  [[steps.loop.body]]
  id = "collect"
  title = "Collect"
  execution = "script"
  depends_on = ["embedded"]

  [steps.loop.body.script]
  command = ["echo", "done"]
`)

	workflow, err := CompileWorkflowByName(context.Background(), "parent", []string{dir}, nil)
	if err != nil {
		t.Fatalf("Compile(parent) error = %v", err)
	}
	if workflow == nil {
		t.Fatalf("Compile(parent) returned nil workflow")
	}

	// Runtime loop bodies are not graph nodes, so verify through the typed step
	// shape by compiling without error and checking embedded body IDs are present
	// in the formula model after resolution.
	parser := NewParser(dir)
	resolved, err := parser.LoadByName("parent")
	if err != nil {
		t.Fatalf("LoadByName(parent) error = %v", err)
	}
	resolved, err = parser.Resolve(resolved)
	if err != nil {
		t.Fatalf("Resolve(parent) error = %v", err)
	}
	expanded, err := ApplyEmbedsWithVars(resolved.Steps, parser, nil, []string{"parent"})
	if err != nil {
		t.Fatalf("ApplyEmbedsWithVars error = %v", err)
	}
	if len(expanded) != 1 || expanded[0].Loop == nil {
		t.Fatalf("expected one runtime loop step, got %+v", expanded)
	}
	ids := map[string]bool{}
	for _, child := range expanded[0].Loop.Body {
		ids[child.ID] = true
	}
	for _, want := range []string{"embedded", "embedded.first", "embedded.last", "collect"} {
		if !ids[want] {
			t.Fatalf("expanded loop body missing %q; ids=%v", want, ids)
		}
	}
	if expanded[0].Loop.Body[len(expanded[0].Loop.Body)-1].DependsOn[0] != "embedded.last" {
		t.Fatalf("collect should depend on embedded exit, got %+v", expanded[0].Loop.Body[len(expanded[0].Loop.Body)-1].DependsOn)
	}
}

func TestEmbedVarsInsideRuntimeLoopBodyApplyToEmbeddedSteps(t *testing.T) {
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

[vars]
pr_ref = { default = "" }

[[steps]]
id = "show"
title = "Show"
execution = "script"

[steps.script]
command = ["echo", "{{pr_ref}}"]
`)

	writeFormula("parent", `formula = "parent"
version = 1
type = "workflow"

[[steps]]
id = "loop"
title = "Loop"

  [steps.loop]
  for_each = "items"
  var = "pr"

  [[steps.loop.body]]
  id = "embedded"
  title = "Embedded child"
  embed = "child"

  [steps.loop.body.embed_vars]
  pr_ref = "{{pr.pr_ref}}"
`)

	parser := NewParser(dir)
	resolved, err := parser.LoadByName("parent")
	if err != nil {
		t.Fatalf("LoadByName(parent) error = %v", err)
	}
	resolved, err = parser.Resolve(resolved)
	if err != nil {
		t.Fatalf("Resolve(parent) error = %v", err)
	}
	expanded, err := ApplyEmbedsWithVars(resolved.Steps, parser, nil, []string{"parent"})
	if err != nil {
		t.Fatalf("ApplyEmbedsWithVars error = %v", err)
	}
	if len(expanded) != 1 || expanded[0].Loop == nil {
		t.Fatalf("expected runtime loop step, got %+v", expanded)
	}
	for _, child := range expanded[0].Loop.Body {
		if child.ID == "embedded.show" {
			if child.Script == nil || len(child.Script.Command) != 2 || child.Script.Command[1] != "{{pr.pr_ref}}" {
				t.Fatalf("embed_vars not applied to embedded command, got %+v", child.Script)
			}
			return
		}
	}
	t.Fatalf("expanded loop body missing embedded.show: %+v", expanded[0].Loop.Body)
}

func TestEmbedRewritesInternalContextRefsInTemplatesAndInputContext(t *testing.T) {
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
id = "fetch"
title = "Fetch"
execution = "script"

[steps.script]
command = ["echo", "ok"]
format = "json"

[[steps]]
id = "use"
title = "Use"
execution = "script"
depends_on = ["fetch"]
condition = "fetch.stdout.ok == true"
input_context = ["fetch.stdout"]

[steps.script]
command = ["echo", "{{fetch.stdout}}"]
format = "json"

[steps.script.env]
CHILD_FETCH = "{{fetch.stdout}}"
`)

	writeFormula("parent", `formula = "parent"
version = 1
type = "workflow"

[[steps]]
id = "embedded"
title = "Embedded child"
embed = "child"
`)

	parser := NewParser(dir)
	resolved, err := parser.LoadByName("parent")
	if err != nil {
		t.Fatalf("LoadByName(parent) error = %v", err)
	}
	resolved, err = parser.Resolve(resolved)
	if err != nil {
		t.Fatalf("Resolve(parent) error = %v", err)
	}
	expanded, err := ApplyEmbedsWithVars(resolved.Steps, parser, nil, []string{"parent"})
	if err != nil {
		t.Fatalf("ApplyEmbedsWithVars error = %v", err)
	}

	var use *Step
	for _, step := range expanded {
		if step.ID == "embedded.use" {
			use = step
			break
		}
	}
	if use == nil {
		t.Fatalf("expanded steps missing embedded.use: %+v", expanded)
	}
	if got, want := use.Condition, "embedded.fetch.stdout.ok == true"; got != want {
		t.Fatalf("condition = %q, want %q", got, want)
	}
	if len(use.InputCtx) != 1 || use.InputCtx[0] != "embedded.fetch.stdout" {
		t.Fatalf("input_context = %+v, want embedded.fetch.stdout", use.InputCtx)
	}
	if got, want := use.Script.Command[1], "{{embedded.fetch.stdout}}"; got != want {
		t.Fatalf("command template = %q, want %q", got, want)
	}
	if got, want := use.Script.Env["CHILD_FETCH"], "{{embedded.fetch.stdout}}"; got != want {
		t.Fatalf("env template = %q, want %q", got, want)
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
