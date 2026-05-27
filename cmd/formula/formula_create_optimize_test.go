package formulacmd

import (
	"strings"
	"testing"
)

func TestBuildFormulaCreatePromptDisallowsOutputKey(t *testing.T) {
	prompt := buildFormulaCreatePrompt("demo", "do work")
	if !strings.Contains(prompt, "Do not add output_key") {
		t.Fatalf("create prompt should explicitly disallow output_key:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Step id is the output key by default") {
		t.Fatalf("create prompt should explain step id output key default:\n%s", prompt)
	}
}

func TestStripFormulaOutputKeyLines(t *testing.T) {
	input := `formula = "demo"
version = 1
type = "workflow"

[[steps]]
id = "analyze"
output_key = "analysis"
title = "Analyze"
# output_key = "commented"
output_key_extra = "keep"
`
	got := stripFormulaOutputKeyLines(input)
	if strings.Contains(got, `output_key = "analysis"`) {
		t.Fatalf("output_key line was not stripped:\n%s", got)
	}
	if !strings.Contains(got, `# output_key = "commented"`) {
		t.Fatalf("comment mentioning output_key should be preserved:\n%s", got)
	}
	if !strings.Contains(got, `output_key_extra = "keep"`) {
		t.Fatalf("similarly-prefixed keys should be preserved:\n%s", got)
	}
}
