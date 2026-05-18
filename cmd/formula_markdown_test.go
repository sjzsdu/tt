package cmd

import (
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
		"- **Steps:** `2`",
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
