package formula

import (
	"strings"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/spec"
)

func normalizeFormulaWorkspace(f *spec.Formula) {
	if f == nil {
		return
	}
	if f.Workspace == nil && f.Worktree {
		f.Workspace = &spec.WorkspaceSpec{Kind: "worktree"}
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

func formulaWorkspacePolicy(f *spec.Formula) *ir.WorkspacePolicy {
	if f == nil {
		return nil
	}
	ws := f.Workspace
	if ws == nil && f.Worktree {
		ws = &spec.WorkspaceSpec{Kind: "worktree"}
	}
	if ws == nil {
		return nil
	}
	kind := strings.TrimSpace(ws.Kind)
	if kind == "" {
		kind = "worktree"
	}
	cleanup := true
	if ws.Cleanup != nil {
		cleanup = *ws.Cleanup
	}
	return &ir.WorkspacePolicy{Kind: kind, Path: strings.TrimSpace(ws.Path), Cleanup: cleanup, Branch: strings.TrimSpace(ws.Branch), Base: strings.TrimSpace(ws.Base), BranchSlugFrom: strings.TrimSpace(ws.BranchSlugFrom), BranchPrefix: strings.TrimSpace(ws.BranchPrefix)}
}
