package formula

import (
	"testing"

	"github.com/sjzsdu/tt/internal/formula/steps"
)

func TestCompileWorkflowUsesTypedPipeline(t *testing.T) {
	wf, err := CompileWorkflow("demo.toml", []byte(`formula = "demo"
[[steps]]
id = "ask"
kind = "human_input"
`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if wf.ID != "demo" {
		t.Fatalf("workflow id = %q", wf.ID)
	}
	if got := wf.Graph.Nodes["ask"].Step.Meta().Kind; got != steps.KindHumanInput {
		t.Fatalf("kind = %s", got)
	}
}

func TestCompileWorkflowPropagatesWorkspacePolicy(t *testing.T) {
	wf, err := CompileWorkflow("demo.toml", []byte(`formula = "demo"
workspace = { kind = "worktree", cleanup = false, path = ".tt/worktrees/demo", branch = "{{branch_name}}", base = "{{base_branch}}", branch_slug_from = "feature_request", branch_prefix = "feature" }
[[steps]]
id = "ask"
kind = "human_input"
`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if wf.Workspace == nil {
		t.Fatal("workspace policy missing")
	}
	if wf.Workspace.Kind != "worktree" || wf.Workspace.Path != ".tt/worktrees/demo" || wf.Workspace.Cleanup {
		t.Fatalf("workspace policy = %+v", wf.Workspace)
	}
	if wf.Workspace.Branch != "{{branch_name}}" || wf.Workspace.Base != "{{base_branch}}" || wf.Workspace.BranchSlugFrom != "feature_request" || wf.Workspace.BranchPrefix != "feature" {
		t.Fatalf("workspace branch policy = %+v", wf.Workspace)
	}
}
