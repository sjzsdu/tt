package cmd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sjzsdu/tt/internal/formula"
)

func TestGenerateFormulaMarkdownDetailsAndRunExamples(t *testing.T) {
	f := &formula.Formula{
		Formula:     "bug-fix",
		Description: "Fix a bug",
		Version:     1,
		Type:        formula.TypeWorkflow,
		Phase:       "liquid",
		Steps: []*formula.Step{
			{ID: "diagnose", Title: "Diagnose bug"},
		},
	}
	recipe := &formula.Recipe{
		Name: "bug-fix",
		Steps: []formula.RecipeStep{
			{ID: "root", Title: "Root", IsRoot: true},
			{ID: "diagnose", Title: "Diagnose bug"},
		},
	}

	md := generateFormulaMarkdown(f, recipe)
	for _, want := range []string{
		"## Formula Details",
		"- **Version:** `1`",
		"- **Type:** `workflow`",
		"- **Phase:** `liquid`",
		"- **Steps:** `1`",
		"tt formula run bug-fix --dry-run",
		"tt formula run bug-fix",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "| | |") || strings.Contains(md, "|---|---|") {
		t.Fatalf("formula details should not use a markdown table:\n%s", md)
	}
}

func TestCollectFormulaShowAllMarkdownIncludesBuiltins(t *testing.T) {
	oldDir := formulaDir
	t.Cleanup(func() { formulaDir = oldDir })
	formulaDir = t.TempDir()

	formulas := collectFormulaShowAllMarkdownFormulas()
	seen := map[string]bool{}
	for _, f := range formulas {
		seen[f.Formula] = true
	}
	for _, want := range []string{"fresh-topic-docs", "san-sheng-liu-bu"} {
		if !seen[want] {
			t.Fatalf("show --markdown all formulas missing builtin %q; got %v", want, seen)
		}
	}
}

func TestGenerateFormulaMarkdownCountsAuthoredStepsOnly(t *testing.T) {
	f := &formula.Formula{Formula: "runtime-control", Version: 1, Type: formula.TypeWorkflow, Steps: []*formula.Step{
		{ID: "decide", Title: "Decide"},
		{ID: "frontend", Title: "Frontend"},
		{ID: "improve", Title: "Improve", Loop: &formula.LoopSpec{Body: []*formula.Step{
			{ID: "draft", Title: "Draft"},
			{ID: "review", Title: "Review"},
		}}},
	}}
	recipe := &formula.Recipe{Steps: []formula.RecipeStep{
		{ID: "runtime-control", IsRoot: true},
		{ID: "runtime-control.start", Title: "start", Execution: "noop", Metadata: map[string]string{"formula_boundary": "start"}},
		{ID: "runtime-control.decide", Title: "Decide"},
		{ID: "runtime-control.frontend", Title: "Frontend"},
		{ID: "runtime-control.improve", Title: "Improve", Loop: &formula.LoopSpec{Body: []*formula.Step{{ID: "draft"}, {ID: "review"}}}},
		{ID: "runtime-control.end", Title: "end", Execution: "noop", Metadata: map[string]string{"formula_boundary": "end"}},
	}}

	md := generateFormulaMarkdown(f, recipe)
	for _, want := range []string{"- **Steps:** `3`", "- **Loop body steps:** `2`"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "- **Steps:** `6`") {
		t.Fatalf("markdown should not count root/generated boundary recipe steps:\n%s", md)
	}
}

func TestApplyFormulaRunPositionalVars(t *testing.T) {
	issue := &formula.VarDef{Required: true}
	f := &formula.Formula{Formula: "bug-fix", Vars: map[string]*formula.VarDef{"issue_summary": issue}}
	vars := map[string]string{}
	if err := applyFormulaRunPositionalVars(f, []string{"按钮", "点击后", "报错"}, vars); err != nil {
		t.Fatalf("applyFormulaRunPositionalVars returned error: %v", err)
	}
	if got := vars["issue_summary"]; got != "按钮 点击后 报错" {
		t.Fatalf("issue_summary = %q", got)
	}
}

func TestApplyFormulaRunPositionalVarsRejectsAmbiguousRequiredVars(t *testing.T) {
	f := &formula.Formula{Formula: "multi", Vars: map[string]*formula.VarDef{
		"a": {Required: true},
		"b": {Required: true},
	}}
	if err := applyFormulaRunPositionalVars(f, []string{"value"}, map[string]string{}); err == nil {
		t.Fatalf("expected error for multiple required vars")
	}
}

func TestGenerateFormulaMarkdownUsesRunShorthandForSingleRequiredVar(t *testing.T) {
	f := &formula.Formula{
		Formula: "bug-fix",
		Version: 1,
		Type:    formula.TypeWorkflow,
		Vars: map[string]*formula.VarDef{
			"issue_summary": {Required: true},
		},
	}
	recipe := &formula.Recipe{Steps: []formula.RecipeStep{{ID: "root", IsRoot: true}}}
	md := generateFormulaMarkdown(f, recipe)
	if !strings.Contains(md, "tt formula run bug-fix <value>") {
		t.Fatalf("markdown missing positional run shorthand:\n%s", md)
	}
}

func TestDefaultFormulaAgentUsesPicoclawMain(t *testing.T) {
	if got := defaultFormulaAgent(""); got != "main" {
		t.Fatalf("defaultFormulaAgent(empty) = %q, want main", got)
	}
	if got := defaultFormulaAgent("  "); got != "main" {
		t.Fatalf("defaultFormulaAgent(blank) = %q, want main", got)
	}
	if got := defaultFormulaAgent("planner"); got != "planner" {
		t.Fatalf("defaultFormulaAgent(explicit) = %q, want planner", got)
	}
}

func TestGenerateMermaidGraphShowsRuntimeControlSemantics(t *testing.T) {
	recipe := &formula.Recipe{
		Name: "runtime-control",
		Steps: []formula.RecipeStep{
			{ID: "root", Title: "Root", IsRoot: true},
			{ID: "start", Title: "start", Execution: "noop", Metadata: map[string]string{"formula_boundary": "start"}},
			{ID: "decide", Title: "Decide", OutputKey: "decision"},
			{ID: "frontend-plan", Title: "Frontend", Condition: "decision.path == frontend", InputCtx: []string{"decision"}},
			{
				ID:        "improve",
				Title:     "Improve",
				Condition: "decision.path == frontend",
				Loop: &formula.LoopSpec{
					Until: "review.approved == true",
					Max:   3,
					Body: []*formula.Step{
						{ID: "draft", Title: "Draft", OutputKey: "draft"},
						{ID: "review", Title: "Review", OutputKey: "review"},
					},
				},
			},
			{ID: "end", Title: "end", Execution: "noop", Metadata: map[string]string{"formula_boundary": "end"}},
		},
		Deps: []formula.RecipeDep{
			{StepID: "start", DependsOnID: "root"},
			{StepID: "decide", DependsOnID: "start"},
			{StepID: "frontend-plan", DependsOnID: "decide"},
			{StepID: "improve", DependsOnID: "frontend-plan"},
			{StepID: "end", DependsOnID: "improve"},
		},
	}

	graph := generateMermaidGraph(recipe)
	for _, want := range []string{
		"frontend_plan{\"frontend-plan:",
		"class frontend_plan nodeCondition",
		"if: decision.path == frontend",
		"out: decision",
		"in: decision",
		"loop: until review.approved == true; max 3",
		"subgraph improve_loop_body[\"loop body: improve\"]",
		"body: draft<br/>Draft<br/>out: draft",
		"body: review<br/>Review<br/>out: review",
		"-.-> |iterate|",
		"class improve nodeCondition",
		"classDef nodeCondition",
		"classDef nodeLoopBody",
	} {
		if !strings.Contains(graph, want) {
			t.Fatalf("mermaid graph missing %q:\n%s", want, graph)
		}
	}
	if strings.Contains(graph, "root:") {
		t.Fatalf("mermaid graph should use synthetic start instead of rendering recipe root:\n%s", graph)
	}
	for _, unwanted := range []string{"start([\"start: start\"])", "end([\"end: end\"])", "start --> decide", "improve --> end"} {
		if strings.Contains(graph, unwanted) {
			t.Fatalf("mermaid graph should hide generated boundary %q:\n%s", unwanted, graph)
		}
	}
}

func TestGenerateFormulaMarkdownShowsLoopDetails(t *testing.T) {
	f := &formula.Formula{Formula: "runtime-control", Version: 1, Type: formula.TypeWorkflow}
	recipe := &formula.Recipe{
		Name: "runtime-control",
		Steps: []formula.RecipeStep{
			{ID: "runtime-control", Title: "Root", IsRoot: true},
			{
				ID:        "runtime-control.improve",
				Title:     "Improve",
				Condition: "decision.path == frontend",
				Loop: &formula.LoopSpec{Until: "review.approved == true", Max: 3, Body: []*formula.Step{
					{ID: "draft", Title: "Draft iteration {{iteration}}", OutputKey: "draft"},
					{ID: "review", Title: "Review iteration {{iteration}}", InputCtx: []string{"draft"}, OutputKey: "review"},
				}},
			},
		},
	}

	md := generateFormulaMarkdown(f, recipe)
	for _, want := range []string{
		"#### Runtime Loop",
		"- **Control:** until `review.approved == true`; max `3`",
		"- **Step condition:** `decision.path == frontend`",
		"| # | Body Step | Title | Input | Output | Condition | Agent |",
		"| 1 | `draft` | Draft iteration {{iteration}} | - | `draft` | - | default |",
		"| 2 | `review` | Review iteration {{iteration}} | `draft` | `review` | - | default |",
		"runtime-control.improve.iter1.<body>",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
}

func TestGenerateFormulaMarkdownHidesGeneratedBoundarySteps(t *testing.T) {
	f := &formula.Formula{Formula: "demo", Version: 1, Type: formula.TypeWorkflow, Steps: []*formula.Step{{ID: "work", Title: "Work"}}}
	recipe := &formula.Recipe{Steps: []formula.RecipeStep{
		{ID: "demo", Title: "Root", IsRoot: true},
		{ID: "demo.start", Title: "start", Execution: "noop", Metadata: map[string]string{"formula_boundary": "start"}},
		{ID: "demo.work", Title: "Work"},
		{ID: "demo.end", Title: "end", Execution: "noop", Metadata: map[string]string{"formula_boundary": "end"}},
	}}
	md := generateFormulaMarkdown(f, recipe)
	if !strings.Contains(md, "### 1. `demo.work`") {
		t.Fatalf("markdown should include real step with display index 1:\n%s", md)
	}
	for _, unwanted := range []string{"`demo.start`", "`demo.end`", "start: start", "end: end"} {
		if strings.Contains(md, unwanted) {
			t.Fatalf("markdown should hide generated boundary %q:\n%s", unwanted, md)
		}
	}
}

func TestGenerateMermaidGraphConvergesRootOnlyRecipe(t *testing.T) {
	recipe := &formula.Recipe{
		Steps: []formula.RecipeStep{
			{ID: "root", Title: "Root", IsRoot: true},
			{ID: "start", Title: "start", Execution: "noop", Metadata: map[string]string{"formula_boundary": "start"}},
			{ID: "end", Title: "end", Execution: "noop", Metadata: map[string]string{"formula_boundary": "end"}},
		},
		Deps: []formula.RecipeDep{{StepID: "start", DependsOnID: "root"}, {StepID: "end", DependsOnID: "start"}},
	}
	graph := generateMermaidGraph(recipe)
	for _, unwanted := range []string{"start([\"start: start\"])", "end([\"end: end\"])", "start --> end", "root:"} {
		if strings.Contains(graph, unwanted) {
			t.Fatalf("root-only mermaid graph should hide generated boundary %q:\n%s", unwanted, graph)
		}
	}
}

func TestExtractFormulaTOMLFromFencedResponse(t *testing.T) {
	resp := "Here is the formula:\n```toml\nformula = \"demo\"\nversion = 1\ntype = \"workflow\"\n```\n"
	got := extractFormulaTOML(resp)
	if got != "formula = \"demo\"\nversion = 1\ntype = \"workflow\"" {
		t.Fatalf("extractFormulaTOML() = %q", got)
	}
}

func TestFormulaCreateOutputPathUsesDir(t *testing.T) {
	oldDir, oldOutput := formulaDir, formulaCreateOutput
	defer func() { formulaDir, formulaCreateOutput = oldDir, oldOutput }()
	formulaDir = "/tmp/formulas"
	formulaCreateOutput = ""
	got := formulaCreateOutputPath("demo")
	if got != "/tmp/formulas/demo.toml" {
		t.Fatalf("formulaCreateOutputPath() = %q", got)
	}
	formulaCreateOutput = "/tmp/custom.toml"
	if got := formulaCreateOutputPath("demo"); got != "/tmp/custom.toml" {
		t.Fatalf("formulaCreateOutputPath() with output = %q", got)
	}
}

func TestBuildFormulaOptimizePromptPreservesNameAndSuggestion(t *testing.T) {
	prompt := buildFormulaOptimizePrompt("demo", "formula = \"demo\"", "add script validation")
	for _, want := range []string{"Formula name: demo", "add script validation", "Preserve formula = \"demo\" exactly", "formula = \"demo\""} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("optimize prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestValidateFormulaTOMLContent(t *testing.T) {
	f, err := validateFormulaTOMLContent(`formula = "demo"
version = 1
type = "workflow"

[[steps]]
id = "plan"
title = "Plan"
`)
	if err != nil {
		t.Fatal(err)
	}
	if f.Formula != "demo" {
		t.Fatalf("formula = %q", f.Formula)
	}
	if _, err := validateFormulaTOMLContent(`formula = "bad"
version = 1
type = "invalid"
`); err == nil {
		t.Fatalf("expected validation error for invalid formula type")
	}
}

func TestBuildFormulaOptimizePromptWarnsAgainstAgentTableMixing(t *testing.T) {
	prompt := buildFormulaOptimizePrompt("demo", "agent.name = \"coder\"", "improve")
	for _, want := range []string{"never both", "keep using dotted agent.name", "do not add [steps.agent]"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("optimize prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestBuildFormulaOptimizeRepairPromptIncludesValidationError(t *testing.T) {
	prompt := buildFormulaOptimizeRepairPrompt("demo", "improve", "agent.name = \"coder\"\n[steps.agent]\nname = \"coder\"", fmt.Errorf("Key 'steps.agent' has already been defined"))
	for _, want := range []string{"failed local validation", "Key 'steps.agent' has already been defined", "Do not mix dotted agent keys", "Preserve formula = \"demo\" exactly"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("repair prompt missing %q:\n%s", want, prompt)
		}
	}
}
