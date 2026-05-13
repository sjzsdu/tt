package cmd2skill

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseHelpCobraStyle(t *testing.T) {
	help := `A demo command

Usage:
  demo [command]

Available Commands:
  get         Get a resource
  apply       Apply a file

Flags:
  -n, --namespace string   namespace to use
      --dry-run string     dry run mode
`
	n := parseHelp(help, []string{"demo"})
	if n.Description != "A demo command" {
		t.Fatalf("description = %q", n.Description)
	}
	if len(n.Children) != 2 {
		t.Fatalf("children = %d", len(n.Children))
	}
	if got := strings.Join(n.Children[0].Path, " "); got != "demo apply" {
		t.Fatalf("first child path = %q", got)
	}
	if len(n.Flags) != 2 {
		t.Fatalf("flags = %d", len(n.Flags))
	}
}

func TestRenderMainSkillIncludesAgentGuidance(t *testing.T) {
	model := &CLIModel{Name: "demo", Root: &CommandNode{Name: "demo", Path: []string{"demo"}, Description: "Demo CLI"}}
	var buf bytes.Buffer
	if err := RenderMainSkill(model, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"## Agent operating guidance", "Prefer non-mutating commands", "description: Use demo."} {
		if !strings.Contains(out, want) {
			t.Fatalf("render missing %q in:\n%s", want, out)
		}
	}
}
