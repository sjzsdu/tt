package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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

type fixedExternalAgent struct{ raw string }

func (a fixedExternalAgent) RunExternalAgent(context.Context, steps.ExternalAgentRequest) (steps.Value, error) {
	return steps.Value{Type: "json", Raw: json.RawMessage(a.raw)}, nil
}

type retryValidationAgent struct {
	calls   int
	prompts []string
}

func (a *retryValidationAgent) RunAgent(_ context.Context, req steps.AgentRequest) (steps.Value, error) {
	a.calls++
	a.prompts = append(a.prompts, req.Prompt)
	if a.calls == 1 {
		raw, _ := json.Marshal("这里是一段解释，不是 JSON")
		return steps.Value{Type: "json", Raw: raw}, nil
	}
	return steps.Value{Type: "json", Raw: json.RawMessage(`{"feature_summary":"ok","acceptance_criteria":["passes"],"initial_search_targets":["runtime"]}`)}, nil
}

type retryValidationAgentThreeAttempts struct {
	calls   int
	prompts []string
}

func (a *retryValidationAgentThreeAttempts) RunAgent(_ context.Context, req steps.AgentRequest) (steps.Value, error) {
	a.calls++
	a.prompts = append(a.prompts, req.Prompt)
	if a.calls < 3 {
		raw, _ := json.Marshal("still not JSON enough")
		return steps.Value{Type: "json", Raw: raw}, nil
	}
	return steps.Value{Type: "json", Raw: json.RawMessage(`{"feature_summary":"ok","acceptance_criteria":["passes"],"initial_search_targets":["runtime"]}`)}, nil
}

type repairAgent struct {
	prompts []string
}

func (a *repairAgent) RunAgent(_ context.Context, req steps.AgentRequest) (steps.Value, error) {
	a.prompts = append(a.prompts, req.Prompt)
	return steps.Value{Type: "json", Raw: json.RawMessage(`{"fixed_command":["fixed"],"reason":"original command was wrong","formula_update_hint":"replace bad with fixed"}`)}, nil
}

type repairScript struct {
	commands [][]string
}

func (s *repairScript) RunScript(_ context.Context, req steps.ScriptRequest) (steps.Value, error) {
	s.commands = append(s.commands, append([]string(nil), req.Command...))
	if len(req.Command) == 1 && req.Command[0] == "fixed" {
		raw, _ := json.Marshal(map[string]any{"ok": true})
		return steps.Value{Type: "json", Raw: raw}, nil
	}
	raw, _ := json.Marshal(map[string]any{"stderr": "bad command"})
	return steps.Value{Type: "json", Raw: raw}, fmt.Errorf("exit status 1")
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

func TestExecutorResolvesDeclaredWorkflowOutputs(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "final-report", Step: steps.AgentStep{
		Base:      steps.Base{Metadata: steps.Metadata{ID: "final-report", Kind: steps.KindAgent}},
		OutputKey: "report-context",
	}})
	wf := &ir.Workflow{
		ID:    "output-demo",
		Graph: g,
		Outputs: map[string]ir.OutputSchema{
			"report": {From: "report-context", Required: true},
			"extra":  {From: "missing-optional"},
		},
	}
	exec := NewExecutor(wf, steps.Capabilities{Agents: fakeAgent{}})
	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	output, ok := result.Outputs["report"]
	if !ok || len(output.Raw) == 0 {
		t.Fatalf("report output = %+v, ok = %v", output, ok)
	}
	if _, ok := result.Outputs["extra"]; ok {
		t.Fatal("optional missing output should be omitted")
	}
	stepResult := result.Nodes["final-report"]
	if stepResult == nil || len(stepResult.Outputs[steps.OutputResult].Raw) == 0 {
		t.Fatalf("step output ports = %#v", stepResult)
	}
	if _, ok := exec.Context.Get("report-context.result"); !ok {
		t.Fatal("named result port was not stored in runtime context")
	}
}

func TestExecutorFailsWhenRequiredWorkflowOutputIsMissing(t *testing.T) {
	wf := &ir.Workflow{
		ID:      "missing-output",
		Graph:   ir.NewGraph(),
		Outputs: map[string]ir.OutputSchema{"report": {From: "never-produced", Required: true}},
	}
	result, err := NewExecutor(wf, steps.Capabilities{}).Run(context.Background())
	if err == nil {
		t.Fatal("expected required output error")
	}
	if result == nil || result.Status != steps.StatusFailed {
		t.Fatalf("result = %+v", result)
	}
}

func TestExecutorPreviewBypassesSyntheticOutputValidation(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "plan", Step: steps.AgentStep{
		Base:       steps.Base{Metadata: steps.Metadata{ID: "plan", Kind: steps.KindAgent}},
		Validation: &steps.OutputValidationSpec{Format: "json", Required: []string{"answer"}},
	}})
	wf := &ir.Workflow{
		ID:      "preview-output",
		Graph:   g,
		Outputs: map[string]ir.OutputSchema{"report": {From: "never-produced", Required: true}},
	}
	exec := NewExecutor(wf, steps.Capabilities{Agents: fixedOutputAgent{raw: `{"dry_run":true}`}})
	exec.Mode = ExecutionModePreview
	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != steps.StatusCompleted || result.Nodes["plan"].Status != steps.StatusCompleted {
		t.Fatalf("preview result = %+v", result)
	}
	if len(result.Outputs) != 0 {
		t.Fatalf("preview should omit unavailable workflow outputs, got %#v", result.Outputs)
	}
}

func TestExecutorPersistsLoopChildExecutionState(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "review", Step: steps.LoopStep{
		Base: steps.Base{Metadata: steps.Metadata{ID: "review", Kind: steps.KindLoop}},
		Max:  1,
		Body: []steps.Step{steps.NoopStep{Base: steps.Base{Metadata: steps.Metadata{ID: "check", Kind: steps.KindNoop}}}},
	}})
	exec := NewExecutor(&ir.Workflow{ID: "loop-state", Graph: g}, steps.Capabilities{})
	if _, err := exec.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	snapshot, err := exec.Store.Snapshot("loop-state")
	if err != nil {
		t.Fatal(err)
	}
	child, ok := snapshot.Steps["review.iter1.check"]
	if !ok {
		t.Fatalf("child state missing: %+v", snapshot.Steps)
	}
	if child.Status != steps.StatusCompleted || child.Path.DefinitionStepID() != "check" {
		t.Fatalf("child state = %+v", child)
	}
	if got := child.Path.IterationPath(); len(got) != 1 || got[0] != 1 {
		t.Fatalf("iteration path = %v", got)
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

type jsonStdoutScript struct{}

func (jsonStdoutScript) RunScript(context.Context, steps.ScriptRequest) (steps.Value, error) {
	stdout := `{"ready_for_agent":true,"current_branch":"feature/a"}` + "\n"
	raw, _ := json.Marshal(map[string]any{"command": []string{"demo"}, "exit_code": 0, "stdout": stdout})
	return steps.Value{Type: "json", Raw: raw}, nil
}

type conditionProbeAgent struct{ calls int }

func (a *conditionProbeAgent) RunAgent(context.Context, steps.AgentRequest) (steps.Value, error) {
	a.calls++
	return steps.Value{Type: "json", Raw: json.RawMessage(`{"ok":true}`)}, nil
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

func TestExecutorExposesScriptJSONStdoutForConditions(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "prepare", Step: steps.ScriptStep{
		Base:       steps.Base{Metadata: steps.Metadata{ID: "prepare", Kind: steps.KindScript}},
		Command:    []string{"demo"},
		Validation: &steps.OutputValidationSpec{Format: "json", Required: []string{"ready_for_agent", "current_branch"}},
	}})
	g.AddNode(&ir.Node{ID: "resolve", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "resolve", Kind: steps.KindAgent, Condition: "prepare.stdout.ready_for_agent == true"}}}})
	g.AddEdge("prepare", "resolve", "blocks")
	wf := &ir.Workflow{ID: "demo", Graph: g}
	agent := &conditionProbeAgent{}
	exec := NewExecutor(wf, steps.Capabilities{Agents: agent, Scripts: jsonStdoutScript{}})

	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != steps.StatusCompleted {
		t.Fatalf("status = %s", result.Status)
	}
	if agent.calls != 1 {
		t.Fatalf("resolve agent calls = %d, want 1", agent.calls)
	}
	value, ok := exec.Context.Get("prepare.stdout.current_branch")
	if !ok {
		t.Fatal("missing normalized stdout path")
	}
	var branch string
	if err := json.Unmarshal(value.Raw, &branch); err != nil {
		t.Fatal(err)
	}
	if branch != "feature/a" {
		t.Fatalf("branch = %q", branch)
	}
	if _, ok := exec.Context.Get("prepare.stdout_text"); !ok {
		t.Fatal("missing raw stdout_text")
	}
}

func TestExecutorNormalizesExternalAgentJSONTextForContext(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "plan", Step: steps.ExternalAgentStep{
		Base:       steps.Base{Metadata: steps.Metadata{ID: "plan", Kind: steps.KindExternalAgent}},
		Driver:     "codex",
		Prompt:     "plan",
		Validation: &steps.OutputValidationSpec{Format: "json", Required: []string{"approved", "summary"}},
	}})
	wf := &ir.Workflow{ID: "demo", Graph: g}
	wrapper := `{"driver":"codex","text":"noise before {\"approved\":true,\"summary\":\"ok\",\"issues\":[]} noise after","stderr":"very large prompt log","exit_code":0}`
	exec := NewExecutor(wf, steps.Capabilities{ExternalAgents: fixedExternalAgent{raw: wrapper}})

	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != steps.StatusCompleted {
		t.Fatalf("status = %s", result.Status)
	}
	value, ok := exec.Context.Get("plan")
	if !ok {
		t.Fatal("missing plan context output")
	}
	var got map[string]any
	if err := json.Unmarshal(value.Raw, &got); err != nil {
		t.Fatal(err)
	}
	if got["approved"] != true || got["summary"] != "ok" {
		t.Fatalf("normalized output = %#v", got)
	}
	if _, ok := got["stderr"]; ok {
		t.Fatalf("normalized context should not include stderr: %#v", got)
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

func TestExecutorValidatesJSONArrayItemsFromAgentString(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "plan", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "plan", Kind: steps.KindAgent}}, Validation: &steps.OutputValidationSpec{Format: "json", MinItems: 1, ItemRequired: []string{"filename", "title"}}}})
	wf := &ir.Workflow{ID: "demo", Graph: g}
	text, _ := json.Marshal(`[{"filename":"01-intro.md","title":"Intro"}]`)

	exec := NewExecutor(wf, steps.Capabilities{Agents: fixedOutputAgent{raw: string(text)}})
	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != steps.StatusCompleted {
		t.Fatalf("status = %s, want completed", result.Status)
	}
}

func TestExecutorValidatesJSONArrayItemsFromFencedAgentString(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "plan", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "plan", Kind: steps.KindAgent}}, Validation: &steps.OutputValidationSpec{Format: "json", MinItems: 1, ItemRequired: []string{"filename", "title"}}}})
	wf := &ir.Workflow{ID: "demo", Graph: g}
	text, _ := json.Marshal("```json\n" + `[{"filename":"01-intro.md","title":"Intro"}]` + "\n```")

	exec := NewExecutor(wf, steps.Capabilities{Agents: fixedOutputAgent{raw: string(text)}})
	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != steps.StatusCompleted {
		t.Fatalf("status = %s, want completed", result.Status)
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

func TestExecutorRequiredFieldsAllowEmptyCollections(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "manifest", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "manifest", Kind: steps.KindAgent}}, Validation: &steps.OutputValidationSpec{Format: "json", Required: []string{"articles", "meta"}}}})
	wf := &ir.Workflow{ID: "demo", Graph: g}

	exec := NewExecutor(wf, steps.Capabilities{Agents: fixedOutputAgent{raw: `{"articles":[],"meta":{}}`}})
	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != steps.StatusCompleted {
		t.Fatalf("status = %s, want completed", result.Status)
	}
}

func TestExecutorPrefersJSONObjectForRequiredFieldsFromAgentString(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "manifest", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "manifest", Kind: steps.KindAgent}}, Validation: &steps.OutputValidationSpec{Format: "json", Required: []string{"topic_name", "series_title", "audience", "reading_order"}}}})
	wf := &ir.Workflow{ID: "demo", Graph: g}
	text, _ := json.Marshal("Required fields: [topic_name series_title audience reading_order]\n" +
		`{"topic_name":"webgpu","series_title":"WebGPU 入门","audience":"开发者","reading_order":[]}`)

	exec := NewExecutor(wf, steps.Capabilities{Agents: fixedOutputAgent{raw: string(text)}})
	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != steps.StatusCompleted {
		t.Fatalf("status = %s, want completed", result.Status)
	}
}

func TestExecutorRetriesAgentAfterOutputValidationFailure(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "analyze", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "analyze", Kind: steps.KindAgent}}, Prompt: "analyze", Validation: &steps.OutputValidationSpec{Format: "json", Required: []string{"feature_summary", "acceptance_criteria", "initial_search_targets"}}}})
	wf := &ir.Workflow{ID: "demo", Graph: g}
	agent := &retryValidationAgent{}

	result, err := NewExecutor(wf, steps.Capabilities{Agents: agent}).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != steps.StatusCompleted {
		t.Fatalf("status = %s, want completed", result.Status)
	}
	if agent.calls != 2 {
		t.Fatalf("agent calls = %d, want 2", agent.calls)
	}
	if len(agent.prompts) != 2 || !strings.Contains(agent.prompts[1], "Previous output validation failed") {
		t.Fatalf("retry prompt did not include validation advice: %#v", agent.prompts)
	}
}

func TestExecutorRetriesAgentUpToThreeAttempts(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "analyze", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "analyze", Kind: steps.KindAgent}}, Prompt: "analyze", Validation: &steps.OutputValidationSpec{Format: "json", Required: []string{"feature_summary", "acceptance_criteria", "initial_search_targets"}}}})
	wf := &ir.Workflow{ID: "demo", Graph: g}
	agent := &retryValidationAgentThreeAttempts{}
	exec := NewExecutor(wf, steps.Capabilities{Agents: agent})

	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != steps.StatusCompleted {
		t.Fatalf("status = %s, want completed", result.Status)
	}
	if agent.calls != 3 {
		t.Fatalf("agent calls = %d, want 3", agent.calls)
	}
	snapshot, snapErr := exec.Store.Snapshot(wf.ID)
	if snapErr != nil {
		t.Fatal(snapErr)
	}
	if len(snapshot.Repairs) != 2 {
		t.Fatalf("repairs = %d, want 2", len(snapshot.Repairs))
	}
	if snapshot.Repairs[0].Attempt != 1 || snapshot.Repairs[1].Attempt != 2 {
		t.Fatalf("unexpected attempts: %+v", snapshot.Repairs)
	}
	if snapshot.Repairs[1].Status != "succeeded" {
		t.Fatalf("final repair status = %q, want succeeded", snapshot.Repairs[1].Status)
	}
}

func TestExecutorRepairsFailedScriptStepWithAgentAndRetries(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "script", Step: steps.ScriptStep{Base: steps.Base{Metadata: steps.Metadata{ID: "script", Kind: steps.KindScript, Idempotent: true}}, Command: []string{"bad"}}})
	wf := &ir.Workflow{ID: "demo", Graph: g}
	agent := &repairAgent{}
	scripts := &repairScript{}
	exec := NewExecutor(wf, steps.Capabilities{Agents: agent, Scripts: scripts})

	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != steps.StatusCompleted {
		t.Fatalf("status = %s, want completed", result.Status)
	}
	if len(scripts.commands) != 2 {
		t.Fatalf("script calls = %d, want 2", len(scripts.commands))
	}
	if got := strings.Join(scripts.commands[1], " "); got != "fixed" {
		t.Fatalf("retry command = %q, want fixed", got)
	}
	if len(agent.prompts) != 1 || !strings.Contains(agent.prompts[0], "Formula step execution guard") {
		t.Fatalf("repair prompt missing guard/instructions: %#v", agent.prompts)
	}
	if _, ok := exec.Context.Get("formula_repairs.script"); !ok {
		t.Fatal("missing formula repair context")
	}
}

func TestExecutorSkipsFixForNonIdempotentScript(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "script", Step: steps.ScriptStep{Base: steps.Base{Metadata: steps.Metadata{ID: "script", Kind: steps.KindScript}}, Command: []string{"bad"}}})
	wf := &ir.Workflow{ID: "demo", Graph: g}
	agent := &repairAgent{}
	scripts := &repairScript{}
	exec := NewExecutor(wf, steps.Capabilities{Agents: agent, Scripts: scripts})

	result, err := exec.Run(context.Background())
	if err == nil {
		t.Fatal("expected run to fail for non-idempotent script")
	}
	if result == nil || result.Status != steps.StatusFailed {
		t.Fatalf("status = %v, want failed", result)
	}
	if len(agent.prompts) != 0 {
		t.Fatalf("repair agent should not run, got prompts %#v", agent.prompts)
	}
	if len(scripts.commands) != 1 {
		t.Fatalf("script calls = %d, want 1", len(scripts.commands))
	}
	if _, ok := exec.Context.Get("formula_repairs.script"); ok {
		t.Fatal("unexpected formula repair context for non-idempotent script")
	}
	state, ok, getErr := exec.Store.GetStep(wf.ID, ir.NodeID("script"))
	if getErr != nil {
		t.Fatal(getErr)
	}
	if !ok {
		t.Fatal("missing step state")
	}
	if state.Status != steps.StatusFailed {
		t.Fatalf("step status = %s, want failed", state.Status)
	}
}

func TestExecutorFindsValidJSONObjectAmongMultipleAgentJSONSnippets(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "manifest", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "manifest", Kind: steps.KindAgent}}, Validation: &steps.OutputValidationSpec{Format: "json", Required: []string{"topic_name", "series_title", "audience", "reading_order"}}}})
	wf := &ir.Workflow{ID: "demo", Graph: g}
	text, _ := json.Marshal("Example:\n```json\n{\"topic_name\":\"draft\"}\n```\nFinal:\n" +
		`{"topic_name":"webgpu","series_title":"WebGPU 入门","audience":"开发者","reading_order":[{"filename":"01.md","title":"Intro","summary":"包含 {braces} 的摘要"}]}`)

	exec := NewExecutor(wf, steps.Capabilities{Agents: fixedOutputAgent{raw: string(text)}})
	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != steps.StatusCompleted {
		t.Fatalf("status = %s, want completed", result.Status)
	}
}

func TestExecutorFindsValidJSONArrayAmongProseAndOtherSnippets(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "plan", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "plan", Kind: steps.KindAgent}}, Validation: &steps.OutputValidationSpec{Format: "json", MinItems: 1, ItemRequired: []string{"filename", "title"}}}})
	wf := &ir.Workflow{ID: "demo", Graph: g}
	text, _ := json.Marshal("I will return an array. Note: {not json}\n" +
		`[{"filename":"01-intro.md","title":"Intro"}]` +
		"\nDone.")

	exec := NewExecutor(wf, steps.Capabilities{Agents: fixedOutputAgent{raw: string(text)}})
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

func TestExecutorAutoCreatesAndCleansWorkspaceWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(repo, "src", "module"), 0o755); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, root, "init", "repo")
	runGitCmd(t, repo, "config", "user.email", "test@example.com")
	runGitCmd(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "src", "module", "tracked.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "top.txt"), []byte("root"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docs", "extra.txt"), []byte("docs"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, repo, "add", ".")
	runGitCmd(t, repo, "commit", "-m", "initial")

	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "report", Step: steps.ScriptStep{Base: steps.Base{Metadata: steps.Metadata{ID: "report", Kind: steps.KindScript}}, Command: []string{"python3", "-c", `import json, os
print(json.dumps({"cwd": os.getcwd(), "tt_invocation_cwd": os.environ.get("TT_INVOCATION_CWD", ""), "tt_workspace_cwd": os.environ.get("TT_WORKSPACE_CWD", ""), "tt_formula_run_dir": os.environ.get("TT_FORMULA_RUN_DIR", ""), "tracked": os.path.exists("src/module/tracked.txt"), "docs": os.path.exists("docs/extra.txt"), "top": os.path.exists("top.txt")}, sort_keys=True))`}}})
	wf := &ir.Workflow{ID: "demo", Name: "demo", Graph: g, Workspace: &ir.WorkspacePolicy{Kind: "worktree", Cleanup: true}}
	exec := NewExecutor(wf, steps.Capabilities{Scripts: ScriptCapability{DenyUnsafe: true}})
	invocationCWD := filepath.Join(repo, "src", "module")
	exec.SeedEnvironment(invocationCWD)
	exec.SeedRunID("run-123")
	exec.SeedFormulaRunDir(filepath.Join(invocationCWD, ".tt", "runs", "formula", "demo-run"))
	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatalf("run error: %v\nresult: %+v\nreport: %+v", err, result, result.Nodes["report"])
	}
	if result.Status != steps.StatusCompleted {
		t.Fatalf("status = %s", result.Status)
	}
	raw := result.Nodes["report"].Output.Raw
	var out scriptCapabilityOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	wantWorkspace, err := filepath.Abs(filepath.Join(repo, "src", "module", ".tt", "worktrees", "run-123"))
	if err != nil {
		t.Fatal(err)
	}
	canon := func(path string) string {
		if strings.HasPrefix(path, "/var/") && !strings.HasPrefix(path, "/private/") {
			return "/private" + path
		}
		return path
	}
	var payload struct {
		CWD             string `json:"cwd"`
		TTInvocationCWD string `json:"tt_invocation_cwd"`
		TTWorkspaceCWD  string `json:"tt_workspace_cwd"`
		TTFormulaRunDir string `json:"tt_formula_run_dir"`
		Tracked         bool   `json:"tracked"`
		Docs            bool   `json:"docs"`
		Top             bool   `json:"top"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.Stdout)), &payload); err != nil {
		t.Fatal(err)
	}
	if canon(payload.CWD) != canon(wantWorkspace) {
		t.Fatalf("workspace cwd = %q, want %q", payload.CWD, wantWorkspace)
	}
	if canon(payload.TTInvocationCWD) != canon(invocationCWD) {
		t.Fatalf("TT_INVOCATION_CWD = %q, want %q", payload.TTInvocationCWD, invocationCWD)
	}
	if canon(payload.TTWorkspaceCWD) != canon(wantWorkspace) {
		t.Fatalf("TT_WORKSPACE_CWD = %q, want %q", payload.TTWorkspaceCWD, wantWorkspace)
	}
	if payload.TTFormulaRunDir == "" {
		t.Fatal("TT_FORMULA_RUN_DIR missing")
	}
	if !payload.Tracked {
		t.Fatal("tracked file missing in worktree")
	}
	if payload.Docs {
		t.Fatal("docs file should be sparse-excluded")
	}
	if _, err := os.Stat(wantWorkspace); !os.IsNotExist(err) {
		t.Fatalf("workspace path still exists after cleanup: %v", err)
	}
}

func TestSeedFormulaRunDirRefreshesEnvironmentContext(t *testing.T) {
	wf := &ir.Workflow{ID: "demo", Name: "demo", Graph: ir.NewGraph()}
	exec := NewExecutor(wf, steps.Capabilities{})
	workspace := t.TempDir()
	runDir := filepath.Join(workspace, ".tt", "runs", "formula", "demo-run")
	exec.SeedEnvironment(workspace)
	exec.SeedFormulaRunDir(runDir)

	env, err := exec.environmentContext()
	if err != nil {
		t.Fatal(err)
	}
	if env.FormulaRunDir != runDir {
		t.Fatalf("FormulaRunDir = %q, want %q", env.FormulaRunDir, runDir)
	}
}

func TestExecutorWorkspaceWorktreeCanBeRetained(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	runGitCmd(t, root, "init", "repo")
	runGitCmd(t, repo, "config", "user.email", "test@example.com")
	runGitCmd(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, repo, "add", ".")
	runGitCmd(t, repo, "commit", "-m", "initial")

	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "report", Step: steps.ScriptStep{Base: steps.Base{Metadata: steps.Metadata{ID: "report", Kind: steps.KindScript}}, Command: []string{"python3", "-c", `import json, os, subprocess
print(json.dumps({"cwd": os.getcwd(), "branch": subprocess.check_output(["git", "branch", "--show-current"], text=True).strip(), "exists": os.path.exists("file.txt")}, sort_keys=True))`}}})
	wf := &ir.Workflow{ID: "demo", Name: "demo", Graph: g, Workspace: &ir.WorkspacePolicy{Kind: "worktree", Cleanup: false, Branch: "{{branch_name}}", BranchSlugFrom: "feature_request", BranchPrefix: "feature", Base: "{{base_branch}}"}}
	exec := NewExecutor(wf, steps.Capabilities{Scripts: ScriptCapability{DenyUnsafe: true}})
	exec.SeedEnvironment(repo)
	exec.SeedVars(map[string]string{"feature_request": "Add Demo Feature", "base_branch": "HEAD"})
	exec.SeedRunID("keep-me")
	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if result.Status != steps.StatusCompleted {
		t.Fatalf("status = %s", result.Status)
	}
	wantWorkspace, err := filepath.Abs(filepath.Join(repo, ".tt", "worktrees", "keep-me"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wantWorkspace); err != nil {
		t.Fatalf("workspace path should be retained: %v", err)
	}
	var out scriptCapabilityOutput
	if err := json.Unmarshal(result.Nodes["report"].Output.Raw, &out); err != nil {
		t.Fatal(err)
	}
	var payload struct{ Branch string }
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.Stdout)), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Branch != "feature/add-demo-feature" {
		t.Fatalf("branch = %q, want feature/add-demo-feature", payload.Branch)
	}
	runGitCmd(t, repo, "worktree", "remove", "--force", wantWorkspace)
}

func TestExecutorWorkspaceBranchAvoidsExistingPrefixRef(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	runGitCmd(t, root, "init", "repo")
	runGitCmd(t, repo, "config", "user.email", "test@example.com")
	runGitCmd(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, repo, "add", ".")
	runGitCmd(t, repo, "commit", "-m", "initial")
	runGitCmd(t, repo, "branch", "feature")

	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "report", Step: steps.ScriptStep{Base: steps.Base{Metadata: steps.Metadata{ID: "report", Kind: steps.KindScript}}, Command: []string{"python3", "-c", `import json, subprocess
print(json.dumps({"branch": subprocess.check_output(["git", "branch", "--show-current"], text=True).strip()}, sort_keys=True))`}}})
	wf := &ir.Workflow{ID: "demo", Name: "demo", Graph: g, Workspace: &ir.WorkspacePolicy{Kind: "worktree", Cleanup: false, Branch: "{{branch_name}}", BranchSlugFrom: "feature_request", BranchPrefix: "feature", Base: "{{base_branch}}"}}
	exec := NewExecutor(wf, steps.Capabilities{Scripts: ScriptCapability{DenyUnsafe: true}})
	exec.SeedEnvironment(repo)
	exec.SeedVars(map[string]string{"feature_request": "Add Demo Feature", "base_branch": "HEAD"})
	exec.SeedRunID("prefix-conflict")
	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if result.Status != steps.StatusCompleted {
		t.Fatalf("status = %s", result.Status)
	}
	var out scriptCapabilityOutput
	if err := json.Unmarshal(result.Nodes["report"].Output.Raw, &out); err != nil {
		t.Fatal(err)
	}
	var payload struct{ Branch string }
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.Stdout)), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Branch != "feature-add-demo-feature" {
		t.Fatalf("branch = %q, want feature-add-demo-feature", payload.Branch)
	}
	wantWorkspace := filepath.Join(repo, ".tt", "worktrees", "prefix-conflict")
	runGitCmd(t, repo, "worktree", "remove", "--force", wantWorkspace)
}

func TestExecutorWorkspaceBranchDefaultsToInvocationHEAD(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	runGitCmd(t, root, "init", "repo")
	runGitCmd(t, repo, "config", "user.email", "test@example.com")
	runGitCmd(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("main"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, repo, "add", ".")
	runGitCmd(t, repo, "commit", "-m", "initial")
	runGitCmd(t, repo, "checkout", "-b", "topic")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("topic"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, repo, "commit", "-am", "topic change")

	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "report", Step: steps.ScriptStep{Base: steps.Base{Metadata: steps.Metadata{ID: "report", Kind: steps.KindScript}}, Command: []string{"python3", "-c", `import json, pathlib, subprocess
print(json.dumps({"branch": subprocess.check_output(["git", "branch", "--show-current"], text=True).strip(), "content": pathlib.Path("file.txt").read_text()}, sort_keys=True))`}}})
	wf := &ir.Workflow{ID: "demo", Name: "demo", Graph: g, Workspace: &ir.WorkspacePolicy{Kind: "worktree", Cleanup: false, Branch: "{{branch_name}}", BranchSlugFrom: "feature_request", BranchPrefix: "feature", Base: "{{base_branch}}"}}
	exec := NewExecutor(wf, steps.Capabilities{Scripts: ScriptCapability{DenyUnsafe: true}})
	exec.SeedEnvironment(repo)
	exec.SeedVars(map[string]string{"feature_request": "Add Demo Feature"})
	exec.SeedRunID("default-head")
	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if result.Status != steps.StatusCompleted {
		t.Fatalf("status = %s", result.Status)
	}
	var out scriptCapabilityOutput
	if err := json.Unmarshal(result.Nodes["report"].Output.Raw, &out); err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Branch  string
		Content string
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.Stdout)), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Branch != "feature/add-demo-feature" {
		t.Fatalf("branch = %q, want feature/add-demo-feature", payload.Branch)
	}
	if payload.Content != "topic" {
		t.Fatalf("content = %q, want topic from invocation HEAD", payload.Content)
	}
	wantWorkspace := filepath.Join(repo, ".tt", "worktrees", "default-head")
	runGitCmd(t, repo, "worktree", "remove", "--force", wantWorkspace)
}

func TestExecutorWorkspaceBranchAvoidsBranchCheckedOutInAnotherWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	runGitCmd(t, root, "init", "repo")
	runGitCmd(t, repo, "config", "user.email", "test@example.com")
	runGitCmd(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, repo, "add", ".")
	runGitCmd(t, repo, "commit", "-m", "initial")
	occupied := filepath.Join(root, "occupied")
	runGitCmd(t, repo, "worktree", "add", "-b", "feature/add-demo-feature", occupied, "HEAD")

	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "report", Step: steps.ScriptStep{Base: steps.Base{Metadata: steps.Metadata{ID: "report", Kind: steps.KindScript}}, Command: []string{"python3", "-c", `import json, subprocess
print(json.dumps({"branch": subprocess.check_output(["git", "branch", "--show-current"], text=True).strip()}, sort_keys=True))`}}})
	wf := &ir.Workflow{ID: "demo", Name: "demo", Graph: g, Workspace: &ir.WorkspacePolicy{Kind: "worktree", Cleanup: false, Branch: "{{branch_name}}", BranchSlugFrom: "feature_request", BranchPrefix: "feature", Base: "{{base_branch}}"}}
	exec := NewExecutor(wf, steps.Capabilities{Scripts: ScriptCapability{DenyUnsafe: true}})
	exec.SeedEnvironment(repo)
	exec.SeedVars(map[string]string{"feature_request": "Add Demo Feature"})
	exec.SeedRunID("run-abcdef123456")
	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if result.Status != steps.StatusCompleted {
		t.Fatalf("status = %s", result.Status)
	}
	var out scriptCapabilityOutput
	if err := json.Unmarshal(result.Nodes["report"].Output.Raw, &out); err != nil {
		t.Fatal(err)
	}
	var payload struct{ Branch string }
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.Stdout)), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Branch != "feature/add-demo-feature-abcdef123456" {
		t.Fatalf("branch = %q, want feature/add-demo-feature-abcdef123456", payload.Branch)
	}
	wantWorkspace := filepath.Join(repo, ".tt", "worktrees", "run-abcdef123456")
	runGitCmd(t, repo, "worktree", "remove", "--force", wantWorkspace)
	runGitCmd(t, repo, "worktree", "remove", "--force", occupied)
}

func TestExecutorStartFromStepSkipsUpstreamSteps(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "a", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "a", Kind: steps.KindAgent}}}})
	g.AddNode(&ir.Node{ID: "b", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "b", Kind: steps.KindAgent}}}})
	g.AddNode(&ir.Node{ID: "c", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "c", Kind: steps.KindAgent}}}})
	g.AddEdge("a", "b", "blocks")
	g.AddEdge("b", "c", "blocks")
	wf := &ir.Workflow{ID: "demo", Graph: g}

	agentA := &countingAgent{}
	agentB := &countingAgent{}
	agentC := &countingAgent{}

	capabilities := steps.Capabilities{
		Agents: &multiAgent{agents: map[string]steps.AgentRunner{
			"a": agentA,
			"b": agentB,
			"c": agentC,
		}},
	}

	exec := NewExecutor(wf, capabilities)
	rawA, _ := json.Marshal("output-a")
	if err := exec.Store.SaveStep(StepState{WorkflowID: wf.ID, NodeID: "a", Status: steps.StatusCompleted, Result: &steps.RunResult{Status: steps.StatusCompleted, Output: steps.Value{Type: "json", Raw: rawA}}}); err != nil {
		t.Fatal(err)
	}
	exec.StartFromStep = "b"

	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != steps.StatusCompleted {
		t.Fatalf("status = %s", result.Status)
	}

	if agentA.calls != 0 {
		t.Fatalf("agent a calls = %d, want 0 (should be skipped)", agentA.calls)
	}
	if agentB.calls != 1 {
		t.Fatalf("agent b calls = %d, want 1", agentB.calls)
	}
	if agentC.calls != 1 {
		t.Fatalf("agent c calls = %d, want 1", agentC.calls)
	}
}

func TestExecutorRerunStepExecutesDownstreamSteps(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "a", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "a", Kind: steps.KindAgent}}}})
	g.AddNode(&ir.Node{ID: "b", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "b", Kind: steps.KindAgent}}}})
	g.AddNode(&ir.Node{ID: "c", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "c", Kind: steps.KindAgent}}}})
	g.AddEdge("a", "b", "blocks")
	g.AddEdge("b", "c", "blocks")
	wf := &ir.Workflow{ID: "demo", Graph: g}

	agentA := &countingAgent{}
	agentB := &countingAgent{}
	agentC := &countingAgent{}

	capabilities := steps.Capabilities{
		Agents: &multiAgent{agents: map[string]steps.AgentRunner{
			"a": agentA,
			"b": agentB,
			"c": agentC,
		}},
	}

	exec := NewExecutor(wf, capabilities)
	rawA, _ := json.Marshal("output-a")
	rawB, _ := json.Marshal("output-b")
	rawC, _ := json.Marshal("output-c")
	if err := exec.Store.SaveStep(StepState{WorkflowID: wf.ID, NodeID: "a", Status: steps.StatusCompleted, Result: &steps.RunResult{Status: steps.StatusCompleted, Output: steps.Value{Type: "json", Raw: rawA}}}); err != nil {
		t.Fatal(err)
	}
	if err := exec.Store.SaveStep(StepState{WorkflowID: wf.ID, NodeID: "b", Status: steps.StatusCompleted, Result: &steps.RunResult{Status: steps.StatusCompleted, Output: steps.Value{Type: "json", Raw: rawB}}}); err != nil {
		t.Fatal(err)
	}
	if err := exec.Store.SaveStep(StepState{WorkflowID: wf.ID, NodeID: "c", Status: steps.StatusCompleted, Result: &steps.RunResult{Status: steps.StatusCompleted, Output: steps.Value{Type: "json", Raw: rawC}}}); err != nil {
		t.Fatal(err)
	}
	exec.RerunSteps = map[ir.NodeID]bool{"b": true}

	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != steps.StatusCompleted {
		t.Fatalf("status = %s", result.Status)
	}

	if agentA.calls != 0 {
		t.Fatalf("agent a calls = %d, want 0 (should be skipped)", agentA.calls)
	}
	if agentB.calls != 1 {
		t.Fatalf("agent b calls = %d, want 1 (should be rerun)", agentB.calls)
	}
	if agentC.calls != 1 {
		t.Fatalf("agent c calls = %d, want 1 (should be rerun as downstream)", agentC.calls)
	}
}

func TestExecutorStartFromStepWithNoPriorState(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "a", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "a", Kind: steps.KindAgent}}}})
	g.AddNode(&ir.Node{ID: "b", Step: steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "b", Kind: steps.KindAgent}}}})
	g.AddEdge("a", "b", "blocks")
	wf := &ir.Workflow{ID: "demo", Graph: g}

	agent := &countingAgent{}
	exec := NewExecutor(wf, steps.Capabilities{Agents: agent})
	exec.StartFromStep = "b"

	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != steps.StatusCompleted {
		t.Fatalf("status = %s", result.Status)
	}

	if agent.calls != 1 {
		t.Fatalf("agent calls = %d, want 1 (only step b)", agent.calls)
	}
}

type multiAgent struct {
	agents map[string]steps.AgentRunner
}

func (m *multiAgent) RunAgent(ctx context.Context, req steps.AgentRequest) (steps.Value, error) {
	agent, ok := m.agents[req.NodeID]
	if !ok {
		return steps.Value{}, fmt.Errorf("no agent for step %s", req.NodeID)
	}
	return agent.RunAgent(ctx, req)
}

func runGitCmd(t *testing.T, cwd string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, strings.TrimSpace(string(out)))
	}
}
