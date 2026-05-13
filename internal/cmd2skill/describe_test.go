package cmd2skill

import (
	"strings"
	"testing"
)

func TestSkillDescriptionIncludesCommandsAndKeywords(t *testing.T) {
	model := &CLIModel{Name: "gh", Root: &CommandNode{Name: "gh", Path: []string{"gh"}, Description: "Work seamlessly with GitHub from the command line", Children: []*CommandNode{
		{Name: "api", Description: "Make an authenticated GitHub API request"},
		{Name: "issue", Description: "Manage issues"},
		{Name: "pr", Description: "Manage pull requests"},
		{Name: "repo", Description: "Manage repositories"},
		{Name: "workflow", Description: "View GitHub Actions workflows"},
	}}}
	desc := SkillDescription(model)
	for _, want := range []string{"gh", "api", "issue", "repo", "workflow", "github"} {
		if !strings.Contains(strings.ToLower(desc), want) {
			t.Fatalf("description missing %q: %s", want, desc)
		}
	}
	if len(desc) > maxSkillDescriptionLen {
		t.Fatalf("description too long: %d", len(desc))
	}
}

func TestCommandDescriptionIncludesChildCommands(t *testing.T) {
	n := &CommandNode{Name: "pr", Path: []string{"gh", "pr"}, Description: "Manage pull requests", Children: []*CommandNode{
		{Name: "checkout", Description: "Check out a pull request in git"},
		{Name: "merge", Description: "Merge a pull request"},
		{Name: "review", Description: "Add a review to a pull request"},
	}}
	desc := CommandDescription(n)
	for _, want := range []string{"gh pr", "checkout", "merge", "review", "pull"} {
		if !strings.Contains(strings.ToLower(desc), want) {
			t.Fatalf("command description missing %q: %s", want, desc)
		}
	}
	if len(desc) > maxCommandDescriptionLen {
		t.Fatalf("description too long: %d", len(desc))
	}
}

func TestDescriptionDeterministic(t *testing.T) {
	model := &CLIModel{Name: "tool", Root: &CommandNode{Name: "tool", Children: []*CommandNode{
		{Name: "zeta", Description: "Manage zeta resources"},
		{Name: "alpha", Description: "Create alpha resources"},
	}}}
	first := SkillDescription(model)
	for i := 0; i < 20; i++ {
		if got := SkillDescription(model); got != first {
			t.Fatalf("description not deterministic:\n%s\n%s", first, got)
		}
	}
}
