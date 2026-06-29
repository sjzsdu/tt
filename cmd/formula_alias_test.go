package cmd

import (
	"reflect"
	"strings"
	"testing"

	"github.com/sjzsdu/tt/internal/formula"
)

func TestFormulaShortcutForwardArgs(t *testing.T) {
	tests := []struct {
		name        string
		formulaName string
		subcmd      string
		args        []string
		want        []string
	}{
		{name: "run inserts formula name", formulaName: "keep-coding", subcmd: "run", args: []string{"--no-web"}, want: []string{"run", "keep-coding", "--no-web"}},
		{name: "show inserts formula name", formulaName: "keep-coding", subcmd: "show", args: nil, want: []string{"show", "keep-coding"}},
		{name: "runs filters formula", formulaName: "keep-coding", subcmd: "runs", args: []string{"--limit", "5"}, want: []string{"runs", "--formula", "keep-coding", "--limit", "5"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formulaShortcutForwardArgs(tt.formulaName, tt.subcmd, tt.args)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("formulaShortcutForwardArgs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestNewFormulaShortcutCommandIncludesExpectedSubcommands(t *testing.T) {
	cmd := newFormulaShortcutCommand("keep-coding", "keep-coding", "")
	for _, name := range []string{"run", "show", "help", "compile", "copy", "runs"} {
		if child, _, err := cmd.Find([]string{name}); err != nil || child == nil || child.Name() != name {
			t.Fatalf("shortcut missing subcommand %q: child=%v err=%v", name, child, err)
		}
	}
}

func TestBuildFormulaShortcutHelpListsAliasSubcommands(t *testing.T) {
	help := buildFormulaShortcutHelp([]formula.BuiltinEntry{{Name: "keep-coding", Aliases: []string{"keep-coding"}}})
	for _, want := range []string{
		"Formula shortcut aliases:",
		"tt keep-coding",
		"run/show/help/compile/copy/runs",
		"formula: keep-coding",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("buildFormulaShortcutHelp() missing %q in:\n%s", want, help)
		}
	}
}

func TestBuildFormulaShortcutHelpIgnoresEmptyAliases(t *testing.T) {
	help := buildFormulaShortcutHelp([]formula.BuiltinEntry{{Name: "no-alias"}, {Name: "blank", Aliases: []string{" "}}})
	if help != "" {
		t.Fatalf("buildFormulaShortcutHelp() = %q, want empty", help)
	}
}
