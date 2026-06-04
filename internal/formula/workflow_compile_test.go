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

func TestCompileWorkflowExternalAgentStep(t *testing.T) {
	wf, err := CompileWorkflow("demo.toml", []byte(`formula = "demo"
[[steps]]
id = "review"
kind = "external_agent"
prompt = "Review the diff"
input_context = ["prepare"]
output_key = "review_result"
driver = "codex"
model = "gpt-5"
mode = "exec"
resume = "sess-1"
cwd = "."
timeout = "2m"
extra_args = ["--sandbox", "read-only"]
`), nil)
	if err != nil {
		t.Fatal(err)
	}
	step, ok := wf.Graph.Nodes["review"].Step.(steps.ExternalAgentStep)
	if !ok {
		t.Fatalf("step type = %T", wf.Graph.Nodes["review"].Step)
	}
	if step.Meta().Kind != steps.KindExternalAgent || step.Driver != "codex" || step.Model != "gpt-5" || step.Timeout != "2m" {
		t.Fatalf("external agent step = %+v", step)
	}
	if got := len(step.InputCtx); got != 1 || step.InputCtx[0] != "prepare" {
		t.Fatalf("input ctx = %#v", step.InputCtx)
	}
}
