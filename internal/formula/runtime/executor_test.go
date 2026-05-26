package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

type fakeAgent struct{}

func (fakeAgent) RunAgent(context.Context, steps.AgentRequest) (steps.Value, error) {
	raw, _ := json.Marshal("agent-ok")
	return steps.Value{Type: "json", Raw: raw}, nil
}

type fixedOutputAgent struct{ raw string }

func (a fixedOutputAgent) RunAgent(context.Context, steps.AgentRequest) (steps.Value, error) {
	return steps.Value{Type: "json", Raw: json.RawMessage(a.raw)}, nil
}

func TestExecutorSeedsEnvironmentContext(t *testing.T) {
	tmp := t.TempDir()
	wf := &ir.Workflow{ID: "demo", Graph: ir.NewGraph()}
	exec := NewExecutor(wf, steps.Capabilities{})
	exec.SeedEnvironment(tmp)

	value, ok := exec.Context.Get("env.cwd")
	if !ok {
		t.Fatal("missing env.cwd")
	}
	var got string
	if err := json.Unmarshal(value.Raw, &got); err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs(tmp)
	if got != want {
		t.Fatalf("env.cwd = %q, want %q", got, want)
	}

	gitValue, ok := exec.Context.Get("env.git.is_repo")
	if !ok {
		t.Fatal("missing env.git.is_repo")
	}
	var isRepo bool
	if err := json.Unmarshal(gitValue.Raw, &isRepo); err != nil {
		t.Fatal(err)
	}
	if isRepo {
		t.Fatalf("temp dir %s unexpectedly detected as git repo", tmp)
	}
}

func TestBuildEnvironmentContextDetectsGitRepo(t *testing.T) {
	if _, err := os.Stat(".git"); err != nil {
		t.Skip("test workspace is not a git repository")
	}
	env := BuildEnvironmentContext(".")
	if !env.Git.IsRepo {
		t.Fatal("expected git repository")
	}
	if env.Git.Root == "" || env.Git.Repo == "" {
		t.Fatalf("incomplete git env: %+v", env.Git)
	}
}

type fakeScript struct{}

func (fakeScript) RunScript(context.Context, steps.ScriptRequest) (steps.Value, error) {
	raw, _ := json.Marshal("script-ok")
	return steps.Value{Type: "json", Raw: raw}, nil
}

type countingAgent struct{ calls int }

func (a *countingAgent) RunAgent(context.Context, steps.AgentRequest) (steps.Value, error) {
	a.calls++
	raw, _ := json.Marshal("fresh")
	return steps.Value{Type: "json", Raw: raw}, nil
}

type dynamicInputAgent struct{ prompt string }

func (a *dynamicInputAgent) RunAgent(_ context.Context, req steps.AgentRequest) (steps.Value, error) {
	a.prompt = req.Prompt
	output := "```tt-human-input json\n" + `{"reason":"need detail","form":{"title":"Clarify","fields":[{"name":"detail","label":"Detail","type":"textarea","required":true}]}}` + "\n```"
	raw, _ := json.Marshal(output)
	return steps.Value{Type: "json", Raw: raw}, nil
}

func TestExecutorRunsTypedWorkflowInTopologicalOrder(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "a", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "a", Kind: steps.KindAgent}}}})
	g.AddNode(&ir.Node{ID: "b", Step: steps.ScriptStep{Base: steps.Base{Metadata: steps.Metadata{ID: "b", Kind: steps.KindScript}}}})
	g.AddEdge("a", "b", "blocks")
	wf := &ir.Workflow{ID: "demo", Graph: g}
	exec := NewExecutor(wf, steps.Capabilities{Agents: fakeAgent{}, Scripts: fakeScript{}})
	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != steps.StatusCompleted {
		t.Fatalf("status = %s", result.Status)
	}
	if len(result.Nodes) != 2 {
		t.Fatalf("nodes = %d", len(result.Nodes))
	}
}

func TestExecutorValidatesJSONArrayItems(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "plan", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "plan", Kind: steps.KindAgent}}, Validation: &steps.OutputValidationSpec{Format: "json", MinItems: 1, ItemRequired: []string{"filename", "title"}}}})
	wf := &ir.Workflow{ID: "demo", Graph: g}

	exec := NewExecutor(wf, steps.Capabilities{Agents: fixedOutputAgent{raw: `[{"filename":"01-intro.md"}]`}})
	result, err := exec.Run(context.Background())
	if err == nil {
		t.Fatal("expected validation error")
	}
	if result.Status != steps.StatusFailed {
		t.Fatalf("status = %s, want failed", result.Status)
	}
	if got := result.Nodes["plan"].Error.Error(); !strings.Contains(got, "output[0].title is required") {
		t.Fatalf("error = %q", got)
	}
}

func TestExecutorAcceptsValidJSONArrayItems(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "plan", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "plan", Kind: steps.KindAgent}}, Validation: &steps.OutputValidationSpec{Format: "json", MinItems: 1, ItemRequired: []string{"filename", "title"}}}})
	wf := &ir.Workflow{ID: "demo", Graph: g}

	exec := NewExecutor(wf, steps.Capabilities{Agents: fixedOutputAgent{raw: `[{"filename":"01-intro.md","title":"Intro"}]`}})
	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != steps.StatusCompleted {
		t.Fatalf("status = %s, want completed", result.Status)
	}
}

func TestExecutorReturnsWaitingForHumanInput(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "ask", Step: steps.HumanInputStep{Base: steps.Base{Metadata: steps.Metadata{ID: "ask", Kind: steps.KindHumanInput}}, Reason: "need input"}})
	wf := &ir.Workflow{ID: "demo", Graph: g}
	result, err := NewExecutor(wf, steps.Capabilities{}).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != steps.StatusWaiting {
		t.Fatalf("status = %s", result.Status)
	}
	if result.Nodes["ask"].Await == nil {
		t.Fatal("missing await request")
	}
}

func TestExecutorReturnsWaitingForDynamicHumanInput(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "triage", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "triage", Kind: steps.KindAgent}}, Prompt: "triage", DynamicForm: true}})
	wf := &ir.Workflow{ID: "demo", Graph: g}
	agent := &dynamicInputAgent{}
	result, err := NewExecutor(wf, steps.Capabilities{Agents: agent}).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != steps.StatusWaiting {
		t.Fatalf("status = %s", result.Status)
	}
	if result.Nodes["triage"].Await == nil || result.Nodes["triage"].Await.Form == nil {
		t.Fatal("missing dynamic await request")
	}
	if !strings.Contains(agent.prompt, "tt-human-input") {
		t.Fatal("dynamic form protocol was not injected into prompt")
	}
}

func TestExecutorSkipsCompletedStoredStepsAndRestoresOutputContext(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "a", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "a", Kind: steps.KindAgent}}, OutputKey: "decision"}})
	g.AddNode(&ir.Node{ID: "b", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "b", Kind: steps.KindAgent}}}})
	g.AddEdge("a", "b", "blocks")
	wf := &ir.Workflow{ID: "demo", Graph: g}
	agent := &countingAgent{}
	exec := NewExecutor(wf, steps.Capabilities{Agents: agent})
	raw, _ := json.Marshal("stored")
	if err := exec.Store.SaveStep(StepState{WorkflowID: wf.ID, NodeID: "a", Status: steps.StatusCompleted, Result: &steps.RunResult{Status: steps.StatusCompleted, Output: steps.Value{Type: "json", Raw: raw}}}); err != nil {
		t.Fatal(err)
	}
	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != steps.StatusCompleted {
		t.Fatalf("status = %s", result.Status)
	}
	if agent.calls != 1 {
		t.Fatalf("agent calls = %d, want only the non-stored step", agent.calls)
	}
	value, ok := exec.Context.Get("decision")
	if !ok {
		t.Fatal("missing restored output context")
	}
	var got string
	if err := json.Unmarshal(value.Raw, &got); err != nil {
		t.Fatal(err)
	}
	if got != "stored" {
		t.Fatalf("context = %q", got)
	}
}

func TestExecutorSkipsStepWhenRuntimeConditionIsFalse(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "make-decision", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "make-decision", Kind: steps.KindAgent}}}})
	g.AddNode(&ir.Node{ID: "b", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "b", Kind: steps.KindAgent, Condition: "make-decision.approved == true"}}}})
	g.AddEdge("make-decision", "b", "blocks")
	wf := &ir.Workflow{ID: "demo", Graph: g}
	agent := &countingAgent{}
	exec := NewExecutor(wf, steps.Capabilities{Agents: agent})
	raw, _ := json.Marshal(map[string]any{"approved": false})
	if err := exec.Store.SaveStep(StepState{WorkflowID: wf.ID, NodeID: "make-decision", Status: steps.StatusCompleted, Result: &steps.RunResult{Status: steps.StatusCompleted, Output: steps.Value{Type: "json", Raw: raw}}}); err != nil {
		t.Fatal(err)
	}
	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Nodes["b"].Status != steps.StatusSkipped {
		t.Fatalf("b status = %s, want skipped", result.Nodes["b"].Status)
	}
	if agent.calls != 0 {
		t.Fatalf("agent calls = %d, want skipped target not executed", agent.calls)
	}
}
