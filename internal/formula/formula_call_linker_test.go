package formula

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLinkerFormula(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name+".toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFormulaCallLinkerRejectsMissingTarget(t *testing.T) {
	dir := t.TempDir()
	writeLinkerFormula(t, dir, "parent", `formula = "parent"
version = 1
type = "workflow"
[[steps]]
id = "call"
title = "Call missing"
execution = "formula"
formula = "missing"
`)
	_, err := CompileWorkflowByName(context.Background(), "parent", []string{dir}, nil)
	if err == nil || !strings.Contains(err.Error(), `target "missing"`) {
		t.Fatalf("err = %v, want missing Formula target", err)
	}
}

func TestValidateFormulaCallGraphUsesParsedRoot(t *testing.T) {
	dir := t.TempDir()
	writeLinkerFormula(t, dir, "custom-filename", `formula = "canonical-parent"
version = 1
type = "workflow"
[[steps]]
id = "call"
title = "Call missing"
execution = "formula"
formula = "missing"
`)
	parsed, err := NewParser().ParseFile(filepath.Join(dir, "custom-filename.toml"))
	if err != nil {
		t.Fatal(err)
	}
	err = ValidateFormulaCallGraph(parsed, []string{dir})
	if err == nil || !strings.Contains(err.Error(), `target "missing"`) {
		t.Fatalf("err = %v, want parsed root FormulaCall validation", err)
	}
}

func TestFormulaCallLinkerRejectsIndirectCycle(t *testing.T) {
	dir := t.TempDir()
	writeLinkerFormula(t, dir, "a", `formula = "a"
version = 1
type = "workflow"
[[steps]]
id = "call-b"
title = "Call B"
execution = "formula"
formula = "b"
`)
	writeLinkerFormula(t, dir, "b", `formula = "b"
version = 1
type = "workflow"
[[steps]]
id = "call-a"
title = "Call A"
execution = "formula"
formula = "a"
`)
	_, err := CompileWorkflowByName(context.Background(), "a", []string{dir}, nil)
	if err == nil || !strings.Contains(err.Error(), "formula call cycle: a -> b -> a") {
		t.Fatalf("err = %v, want indirect cycle", err)
	}
}

func TestFormulaCallLinkerValidatesInputAndOutputContracts(t *testing.T) {
	dir := t.TempDir()
	writeLinkerFormula(t, dir, "child", `formula = "child"
version = 1
type = "workflow"
[vars]
request = { required = true }
[outputs.report]
from = "result"
required = true
`)
	writeLinkerFormula(t, dir, "missing-input", `formula = "missing-input"
version = 1
type = "workflow"
[[steps]]
id = "child"
title = "Call child"
execution = "formula"
formula = "child"
`)
	if _, err := CompileWorkflowByName(context.Background(), "missing-input", []string{dir}, nil); err == nil || !strings.Contains(err.Error(), `does not bind required input "request"`) {
		t.Fatalf("err = %v, want required input error", err)
	}

	writeLinkerFormula(t, dir, "bad-output", `formula = "bad-output"
version = 1
type = "workflow"
[[steps]]
id = "child"
title = "Call child"
execution = "formula"
formula = "child"
[steps.with]
request = "hello"
[[steps]]
id = "report"
title = "Report"
depends_on = ["child"]
input_context = ["child.private_result"]
description = "Report"
`)
	if _, err := CompileWorkflowByName(context.Background(), "bad-output", []string{dir}, nil); err == nil || !strings.Contains(err.Error(), `undeclared output "private_result"`) {
		t.Fatalf("err = %v, want public output error", err)
	}
}

func TestFormulaCallLinkerRequiresParallelOptIn(t *testing.T) {
	dir := t.TempDir()
	writeLinkerFormula(t, dir, "child", `formula = "child"
version = 1
type = "workflow"
`)
	parent := func(allow string) string {
		return `formula = "parent"
version = 1
type = "workflow"
[[steps]]
id = "loop"
title = "Parallel calls"
[steps.loop]
max = 2
parallel = true
[[steps.loop.body]]
id = "call"
title = "Call child"
execution = "formula"
formula = "child"
` + allow + "\n"
	}
	writeLinkerFormula(t, dir, "parent", parent(""))
	if _, err := CompileWorkflowByName(context.Background(), "parent", []string{dir}, nil); err == nil || !strings.Contains(err.Error(), "allow_parallel=true") {
		t.Fatalf("err = %v, want parallel opt-in error", err)
	}
	writeLinkerFormula(t, dir, "parent", parent("allow_parallel = true"))
	if _, err := CompileWorkflowByName(context.Background(), "parent", []string{dir}, nil); err != nil {
		t.Fatalf("parallel opt-in should compile: %v", err)
	}
}

func TestFormulaDefinitionHashIncludesTransitiveChildren(t *testing.T) {
	dir := t.TempDir()
	writeLinkerFormula(t, dir, "parent", `formula = "parent"
version = 1
type = "workflow"
[[steps]]
id = "call"
title = "Call child"
execution = "formula"
formula = "child"
`)
	writeLinkerFormula(t, dir, "child", `formula = "child"
description = "before"
version = 1
type = "workflow"
`)
	before, err := CompileWorkflowByName(context.Background(), "parent", []string{dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	writeLinkerFormula(t, dir, "child", `formula = "child"
description = "after"
version = 1
type = "workflow"
`)
	after, err := CompileWorkflowByName(context.Background(), "parent", []string{dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if before.DefinitionHash == "" || before.DefinitionHash == after.DefinitionHash {
		t.Fatalf("definition hashes should include child changes: before=%q after=%q", before.DefinitionHash, after.DefinitionHash)
	}
}
