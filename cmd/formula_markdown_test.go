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
