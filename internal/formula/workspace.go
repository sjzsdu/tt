package formula

import (
	"strings"

	"github.com/sjzsdu/tt/internal/formula/ir"
)

// WorkspaceSpec declares a formula-level execution workspace policy.
type WorkspaceSpec struct {
	Kind           string `json:"kind,omitempty" toml:"kind,omitempty"`
	Path           string `json:"path,omitempty" toml:"path,omitempty"`
	Cleanup        *bool  `json:"cleanup,omitempty" toml:"cleanup,omitempty"`
	Branch         string `json:"branch,omitempty" toml:"branch,omitempty"`
	Base           string `json:"base,omitempty" toml:"base,omitempty"`
	BranchSlugFrom string `json:"branch_slug_from,omitempty" toml:"branch_slug_from,omitempty"`
	BranchPrefix   string `json:"branch_prefix,omitempty" toml:"branch_prefix,omitempty"`
}

func normalizeFormulaWorkspace(f *Formula) {
	if f == nil {
		return
	}
	if f.Workspace == nil && f.Worktree {
		f.Workspace = &WorkspaceSpec{Kind: "worktree"}
	}
	if f.Workspace == nil {
		return
	}
	f.Workspace.Kind = strings.TrimSpace(f.Workspace.Kind)
	if f.Workspace.Kind == "" {
		f.Workspace.Kind = "worktree"
	}
	f.Workspace.Path = strings.TrimSpace(f.Workspace.Path)
	f.Workspace.Branch = strings.TrimSpace(f.Workspace.Branch)
	f.Workspace.Base = strings.TrimSpace(f.Workspace.Base)
	f.Workspace.BranchSlugFrom = strings.TrimSpace(f.Workspace.BranchSlugFrom)
	f.Workspace.BranchPrefix = strings.TrimSpace(f.Workspace.BranchPrefix)
}

func formulaWorkspacePolicy(f *Formula) *ir.WorkspacePolicy {
	if f == nil {
		return nil
	}
	spec := f.Workspace
	if spec == nil && f.Worktree {
		spec = &WorkspaceSpec{Kind: "worktree"}
	}
	if spec == nil {
		return nil
	}
	kind := strings.TrimSpace(spec.Kind)
	if kind == "" {
		kind = "worktree"
	}
	cleanup := true
	if spec.Cleanup != nil {
		cleanup = *spec.Cleanup
	}
	return &ir.WorkspacePolicy{Kind: kind, Path: strings.TrimSpace(spec.Path), Cleanup: cleanup, Branch: strings.TrimSpace(spec.Branch), Base: strings.TrimSpace(spec.Base), BranchSlugFrom: strings.TrimSpace(spec.BranchSlugFrom), BranchPrefix: strings.TrimSpace(spec.BranchPrefix)}
}
