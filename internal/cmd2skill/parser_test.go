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

func TestParseDoltStyleHelp(t *testing.T) {
	help := `Valid commands for dolt are
                init - Create an empty Dolt data repository.
              status - Show the working tree status.
                 add - Add table changes to the list of staged table changes.

Dolt subcommands are in transition to using the flags listed below as global flags.

usage: dolt <--data-dir=<path>> subcommand <subcommand arguments>

Specific dolt options
    --profile=<profile>
      The name of the profile to use when executing SQL queries.
    -u <user>, --user=<user>
      Defines the local superuser.
`
	n := parseHelp(help, []string{"dolt"})
	if len(n.Children) != 3 {
		t.Fatalf("children = %d", len(n.Children))
	}
	if n.Children[0].Name != "add" || n.Children[0].Description != "Add table changes to the list of staged table changes." {
		t.Fatalf("unexpected first child: %#v", n.Children[0])
	}
	if len(n.Flags) != 2 {
		t.Fatalf("flags = %d", len(n.Flags))
	}
	if n.Flags[1].Name != "user" || n.Flags[1].Shorthand != "u" || n.Flags[1].Type != "user" {
		t.Fatalf("unexpected user flag: %#v", n.Flags[1])
	}
}

func TestExtractDoltManStyleDescription(t *testing.T) {
	help := `NAME
	dolt add - Add table contents to the list of staged tables

SYNOPSIS
	dolt add [<table>...]

DESCRIPTION
	This command updates the list of tables.
`
	if got := extractDescriptionFromHelp(help); got != "Add table contents to the list of staged tables" {
		t.Fatalf("description = %q", got)
	}
}

func TestParseUppercaseCommandSections(t *testing.T) {
	help := `Work seamlessly with a service from the command line.

USAGE
  tool <command> <subcommand> [flags]

CORE COMMANDS
  auth:          Authenticate with the service
  repo:          Manage repositories

ADDITIONAL COMMANDS
  api:           Make an authenticated API request
  config:        Manage configuration

HELP TOPICS
  formatting:    Formatting options

FLAGS
  --help      Show help for command
  --version   Show version

EXAMPLES
  $ tool repo list
`
	n := parseHelp(help, []string{"tool"})
	if n.Usage != "USAGE\n  tool <command> <subcommand> [flags]" {
		t.Fatalf("usage = %q", n.Usage)
	}
	if len(n.Children) != 4 {
		t.Fatalf("children = %d: %#v", len(n.Children), n.Children)
	}
	if n.Children[0].Name != "api" || n.Children[0].Description != "Make an authenticated API request" {
		t.Fatalf("unexpected first child: %#v", n.Children[0])
	}
	if len(n.Flags) != 1 || n.Flags[0].Name != "version" {
		t.Fatalf("flags = %#v", n.Flags)
	}
}

func TestParseFlagLineDoesNotMatchHyphenatedDescriptionWords(t *testing.T) {
	flag := parseFlagLine("  -c, --clipboard                  Copy one-time OAuth device code to clipboard")
	if flag.Name != "clipboard" || flag.Shorthand != "c" {
		t.Fatalf("flag = %#v", flag)
	}
	if strings.Contains(formatFlag(flag), "time") {
		t.Fatalf("hyphenated description word was parsed as flag: %#v", flag)
	}
}
