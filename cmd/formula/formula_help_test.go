package formulacmd

import (
	"bytes"
	"strings"
	"testing"

	spec "github.com/sjzsdu/tt/internal/formula/spec"
)

func TestRenderFormulaHelpIncludesUsageVarsAndSteps(t *testing.T) {
	def := "dry-run"
	f := &spec.Formula{
		Formula:     "demo-help",
		Title:       "Demo Help",
		Description: "Demonstrates help output.",
		Category:    "demo",
		Tags:        []string{"one", "two"},
		Vars: map[string]*spec.VarDef{
			"topic": {Description: "What to work on", Required: true},
			"mode":  {Description: "Run mode", Default: &def, Enum: []string{"dry-run", "auto"}},
		},
		Steps: []*spec.Step{
			{ID: "analyze", Title: "Analyze request"},
			{ID: "run", Title: "Run script", Execution: "script", DependsOn: []string{"analyze"}, Condition: "analyze.ok == true"},
		},
	}

	var buf bytes.Buffer
	renderFormulaHelp(&buf, f)
	out := buf.String()
	for _, want := range []string{
		"# Demo Help",
		"Formula: demo-help",
		"tt formula run demo-help <topic>",
		"tt formula schedule demo-help --every 2m --run-now",
		"- topic (required)",
		"- mode (default: dry-run, one of: dry-run|auto)",
		"1. Analyze request [agent]",
		"2. Run script [script]; depends on analyze; if analyze.ok == true",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("help output missing %q\noutput:\n%s", want, out)
		}
	}
}

func TestFormulaHelpCommandIsRegistered(t *testing.T) {
	cmd := New(Dependencies{})
	helpCmd, _, err := cmd.Find([]string{"help", "bug-fix"})
	if err != nil {
		t.Fatalf("find help command: %v", err)
	}
	if helpCmd == nil || helpCmd.Use != "help <name>" {
		t.Fatalf("formula help command not registered; got %#v", helpCmd)
	}
}
