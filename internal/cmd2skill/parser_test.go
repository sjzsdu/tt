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

func TestRenderAllOnlyEmitsTopLevelReferenceFiles(t *testing.T) {
	model := &CLIModel{
		Name: "demo",
		Root: &CommandNode{Name: "demo", Path: []string{"demo"}, Children: []*CommandNode{
			{Name: "parent", Path: []string{"demo", "parent"}, Description: "Parent command", Children: []*CommandNode{
				{Name: "child", Path: []string{"demo", "parent", "child"}, Description: "Child command"},
			}},
		}},
	}
	var buf bytes.Buffer
	if err := RenderAll(model, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "references/parent-child.md") {
		t.Fatalf("unexpected third-level reference link in:\n%s", out)
	}
	if !strings.Contains(out, "references/parent.md") {
		t.Fatalf("missing second-level reference link in:\n%s", out)
	}
	if !strings.Contains(out, "### demo parent child") {
		t.Fatalf("third-level command should be embedded in parent reference:\n%s", out)
	}
}
