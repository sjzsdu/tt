package formulacmd

import "testing"

func TestFormulaAgentWorkspaceUsesProjectRoot(t *testing.T) {
	if got := formulaAgentWorkspace("/repo/project"); got != "/repo/project" {
		t.Fatalf("formulaAgentWorkspace = %q, want project root", got)
	}
}

func TestFormulaCodeWorkspaceMapsDotTTToParent(t *testing.T) {
	if got := formulaCodeWorkspace("/repo/project/.tt"); got != "/repo/project" {
		t.Fatalf("formulaCodeWorkspace(.tt) = %q, want parent project", got)
	}
	if got := formulaCodeWorkspace("/repo/project/.tt/worktrees/run-1"); got != "/repo/project/.tt/worktrees/run-1" {
		t.Fatalf("formulaCodeWorkspace(worktree) = %q, want unchanged worktree", got)
	}
}
