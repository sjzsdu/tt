package steps

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type mapContextView map[string]Value

func (m mapContextView) Get(path string) (Value, bool) {
	if v, ok := m[path]; ok {
		return v, true
	}
	root, rest, ok := strings.Cut(path, ".")
	if !ok {
		return Value{}, false
	}
	v, ok := m[root]
	if !ok {
		return Value{}, false
	}
	var data any
	if err := json.Unmarshal(v.Raw, &data); err != nil {
		return Value{}, false
	}
	current := data
	for _, part := range strings.Split(rest, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return Value{}, false
		}
		current, ok = object[part]
		if !ok {
			return Value{}, false
		}
	}
	raw, _ := json.Marshal(current)
	return Value{Raw: raw}, true
}

type mapContextStore map[string]Value

func (m mapContextStore) Get(path string) (Value, bool) { return mapContextView(m).Get(path) }
func (m mapContextStore) Set(path string, value Value) error {
	m[path] = value
	return nil
}

type contextEchoStep struct{ Base }

func (s contextEchoStep) Run(_ context.Context, req RunRequest) (*RunResult, error) {
	article, ok := req.Context.Get("article")
	if !ok {
		return failedRun(errMissingArticle{})
	}
	return &RunResult{Status: StatusCompleted, Output: article}, nil
}

type errMissingArticle struct{}

func (errMissingArticle) Error() string { return "missing article" }

type concurrencyProbeStep struct {
	Base
	current *int32
	maxSeen *int32
	delay   time.Duration
}

func (s concurrencyProbeStep) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	now := atomic.AddInt32(s.current, 1)
	for {
		max := atomic.LoadInt32(s.maxSeen)
		if now <= max || atomic.CompareAndSwapInt32(s.maxSeen, max, now) {
			break
		}
	}
	defer atomic.AddInt32(s.current, -1)
	timer := time.NewTimer(s.delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return failedRun(ctx.Err())
	case <-timer.C:
	}
	article, ok := req.Context.Get("article")
	if !ok {
		return failedRun(errMissingArticle{})
	}
	return &RunResult{Status: StatusCompleted, Output: article}, nil
}

func TestStepErrorUnmarshalAcceptsObjectCause(t *testing.T) {
	var result RunResult
	data := []byte(`{"Status":"failed","Error":{"Message":"step output validation failed","Cause":{"message":"output.articles is required"}}}`)
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.Error == nil || result.Error.Cause == nil {
		t.Fatalf("missing error cause: %+v", result.Error)
	}
	if got := result.Error.Error(); got != "step output validation failed: output.articles is required" {
		t.Fatalf("error = %q", got)
	}
}

func TestStepErrorMarshalStoresStringCause(t *testing.T) {
	data, err := json.Marshal(&RunResult{Status: StatusFailed, Error: &StepError{Message: "failed", Cause: errMissingArticle{}}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"Cause":{}`) || !strings.Contains(string(data), `"Cause":"missing article"`) {
		t.Fatalf("encoded result = %s", data)
	}
}

type capturingExternalAgentRunner struct{ req ExternalAgentRequest }

func (r *capturingExternalAgentRunner) RunExternalAgent(_ context.Context, req ExternalAgentRequest) (Value, error) {
	r.req = req
	return Value{Type: "json", Raw: []byte(`{"text":"ok"}`)}, nil
}

func TestExternalAgentStepPassesTimeoutAndConfig(t *testing.T) {
	runner := &capturingExternalAgentRunner{}
	step := ExternalAgentStep{
		Base:      Base{Metadata: Metadata{ID: "ask", Kind: KindExternalAgent}},
		Driver:    "{{driver}}",
		Model:     "{{model}}",
		Mode:      "exec",
		Resume:    "sess",
		Cwd:       ".",
		Timeout:   "2s",
		Prompt:    "review {{input}}",
		ExtraArgs: []string{"--sandbox", "{{sandbox}}"},
	}
	res, err := step.Run(context.Background(), RunRequest{
		NodeID:       "ask",
		Context:      mapContextView{"input": {Raw: []byte(`"code"`)}, "driver": {Raw: []byte(`"codex"`)}, "model": {Raw: []byte(`"gpt-5"`)}, "sandbox": {Raw: []byte(`"read-only"`)}},
		Capabilities: Capabilities{ExternalAgents: runner},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusCompleted {
		t.Fatalf("status = %s", res.Status)
	}
	if runner.req.Driver != "codex" || runner.req.Model != "gpt-5" || runner.req.Mode != "exec" || runner.req.Resume != "sess" {
		t.Fatalf("req = %+v", runner.req)
	}
	if runner.req.Timeout != 2*time.Second {
		t.Fatalf("timeout = %s", runner.req.Timeout)
	}
	if !strings.Contains(runner.req.Prompt, "review code") || !strings.Contains(runner.req.Prompt, "Formula step execution guard") {
		t.Fatalf("prompt = %q", runner.req.Prompt)
	}
	if got := strings.Join(runner.req.ExtraArgs, " "); got != "--sandbox read-only" {
		t.Fatalf("extra args = %q", got)
	}
}

func TestParseDynamicHumanInputRepairsMissingFieldsArrayCloser(t *testing.T) {
	raw, err := json.Marshal("```tt-human-input json\n" + `{"reason":"need scope","form":{"title":"Scope","fields":[{"name":"audience","label":"Audience","type":"select","options":["dev","ops"]},{"name":"constraints","label":"Constraints","type":"textarea"}}}` + "\n```")
	if err != nil {
		t.Fatal(err)
	}
	req, found, err := parseDynamicHumanInputRequest(Value{Type: "json", Raw: raw})
	if err != nil {
		t.Fatal(err)
	}
	if !found || req == nil {
		t.Fatalf("request not found: found=%v req=%+v", found, req)
	}
	form, ok := req.Form.(map[string]any)
	if !ok {
		t.Fatalf("form = %#v", req.Form)
	}
	fields, ok := form["fields"].([]any)
	if !ok || len(fields) != 2 {
		t.Fatalf("fields = %#v", form["fields"])
	}
}

func TestLoopIterationStoreGetUsesLongestStoredPrefixForDottedStepIDs(t *testing.T) {
	store := newLoopIterationStore(nil)
	raw, err := json.Marshal(map[string]any{"stdout": map[string]any{"ok": true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("run-pr-rebase.prepare-rebase-worktree", Value{Type: "json", Raw: raw}); err != nil {
		t.Fatal(err)
	}
	value, ok := store.Get("run-pr-rebase.prepare-rebase-worktree.stdout.ok")
	if !ok {
		t.Fatal("dotted loop step output not found")
	}
	var got bool
	if err := json.Unmarshal(value.Raw, &got); err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatal("ok = false, want true")
	}
}

func TestLoopIterationStoreGetNormalizesJSONStringOutputs(t *testing.T) {
	store := newLoopIterationStore(nil)
	raw, err := json.Marshal(map[string]any{"stdout": `{"ok":true,"nested":{"done":true}}`})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("run-pr-rebase.prepare-rebase-worktree", Value{Type: "json", Raw: raw}); err != nil {
		t.Fatal(err)
	}
	value, ok := store.Get("run-pr-rebase.prepare-rebase-worktree.stdout.ok")
	if !ok {
		t.Fatal("nested JSON string output not found")
	}
	var got bool
	if err := json.Unmarshal(value.Raw, &got); err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatal("ok = false, want true")
	}
}

func TestLoopStepEmitsSkippedForConditionFalseBodyStep(t *testing.T) {
	loop := LoopStep{
		Base: Base{Metadata: Metadata{ID: "monitor", Kind: KindLoop}},
		Max:  1,
		Body: []Step{
			NoopStep{Base: Base{Metadata: Metadata{ID: "repair", Kind: KindNoop, Condition: "action == repair_conflict"}}},
			NoopStep{Base: Base{Metadata: Metadata{ID: "wait", Kind: KindNoop, Condition: "action == wait"}}},
		},
	}
	var events []struct {
		nodeID string
		typ    string
		status Status
	}
	_, err := loop.Run(context.Background(), RunRequest{
		NodeID:  "monitor",
		Context: mapContextView{"action": {Raw: []byte(`"wait"`)}},
		Emit: func(nodeID string, eventType string, payload any) {
			status := Status("")
			if res, ok := payload.(*RunResult); ok && res != nil {
				status = res.Status
			}
			events = append(events, struct {
				nodeID string
				typ    string
				status Status
			}{nodeID: nodeID, typ: eventType, status: status})
		},
	})
	if err != nil {
		t.Fatalf("run loop: %v", err)
	}
	assertLoopEvent(t, events, "monitor.iter1.repair", "step.skipped", StatusSkipped)
	assertLoopEvent(t, events, "monitor.iter1.wait", "step.started", "")
	assertLoopEvent(t, events, "monitor.iter1.wait", "step.completed", StatusCompleted)
}

func TestLoopStepResolvesTemplatedMax(t *testing.T) {
	loop := LoopStep{
		Base:    Base{Metadata: Metadata{ID: "monitor", Kind: KindLoop}},
		MaxExpr: "{{max_cycles}}",
		Until:   "done == true",
		Body: []Step{
			NoopStep{Base: Base{Metadata: Metadata{ID: "tick", Kind: KindNoop}}},
		},
	}
	started := 0
	_, err := loop.Run(context.Background(), RunRequest{
		NodeID:  "monitor",
		Context: mapContextView{"max_cycles": {Raw: []byte(`3`)}},
		Emit: func(nodeID string, eventType string, payload any) {
			if eventType == "step.started" && strings.HasSuffix(nodeID, ".tick") {
				started++
			}
		},
	})
	if err != nil {
		t.Fatalf("run loop: %v", err)
	}
	if started != 3 {
		t.Fatalf("started ticks = %d, want 3", started)
	}
}

func TestLoopStepHonorsMaxWithoutUntil(t *testing.T) {
	loop := LoopStep{
		Base: Base{Metadata: Metadata{ID: "monitor", Kind: KindLoop}},
		Max:  3,
		Body: []Step{
			NoopStep{Base: Base{Metadata: Metadata{ID: "tick", Kind: KindNoop}}},
		},
	}
	started := 0
	_, err := loop.Run(context.Background(), RunRequest{
		NodeID: "monitor",
		Emit: func(nodeID string, eventType string, payload any) {
			if eventType == "step.started" && strings.HasSuffix(nodeID, ".tick") {
				started++
			}
		},
	})
	if err != nil {
		t.Fatalf("run loop: %v", err)
	}
	if started != 3 {
		t.Fatalf("started ticks = %d, want 3", started)
	}
}

func TestLoopStepExposesScriptJSONStdoutForLaterConditions(t *testing.T) {
	agent := &countingStepAgentRunner{}
	loop := LoopStep{
		Base: Base{Metadata: Metadata{ID: "monitor", Kind: KindLoop}},
		Max:  1,
		Body: []Step{
			ScriptStep{
				Base:       Base{Metadata: Metadata{ID: "prepare", Kind: KindScript}},
				Command:    []string{"demo"},
				Validation: &OutputValidationSpec{Format: "json", Required: []string{"stdout"}},
			},
			AgentStep{Base: Base{Metadata: Metadata{ID: "resolve", Kind: KindAgent, Condition: "prepare.stdout.ready_for_agent == true"}}},
		},
	}
	store := newLoopIterationStore(nil)
	res, err := loop.Run(context.Background(), RunRequest{
		NodeID:       "monitor",
		Context:      store,
		Outputs:      store,
		Capabilities: Capabilities{Scripts: jsonStdoutScriptRunner{}, Agents: agent},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusCompleted {
		t.Fatalf("status = %s", res.Status)
	}
	if agent.calls != 1 {
		t.Fatalf("resolve calls = %d, want 1", agent.calls)
	}
	value, ok := store.Get("prepare.stdout.current_branch")
	if !ok {
		t.Fatal("missing normalized stdout current_branch")
	}
	var branch string
	if err := json.Unmarshal(value.Raw, &branch); err != nil {
		t.Fatal(err)
	}
	if branch != "feature/a" {
		t.Fatalf("branch = %q", branch)
	}
}

func TestScriptStepMarkdownFormatReturnsStdoutText(t *testing.T) {
	step := ScriptStep{
		Base:    Base{Metadata: Metadata{ID: "report", Kind: KindScript}},
		Command: []string{"demo"},
		Format:  "markdown",
	}
	res, err := step.Run(context.Background(), RunRequest{
		Capabilities: Capabilities{Scripts: markdownStdoutScriptRunner{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Output.Type != "text" {
		t.Fatalf("output type = %q, want text", res.Output.Type)
	}
	got := strings.TrimSpace(string(res.Output.Raw))
	if got != "## Report\n\n- status: ok" {
		t.Fatalf("output = %q", got)
	}
}

func assertLoopEvent(t *testing.T, events []struct {
	nodeID string
	typ    string
	status Status
}, nodeID, typ string, status Status) {
	t.Helper()
	for _, event := range events {
		if event.nodeID == nodeID && event.typ == typ && event.status == status {
			return
		}
	}
	t.Fatalf("missing event %s %s %s in %#v", nodeID, typ, status, events)
}

type recordingAgentRunner struct {
	prompt string
	req    AgentRequest
}

func (r *recordingAgentRunner) RunAgent(_ context.Context, req AgentRequest) (Value, error) {
	r.prompt = req.Prompt
	r.req = req
	return Value{Raw: []byte(`"ok"`)}, nil
}

type recordingScriptRunner struct {
	req ScriptRequest
}

func (r *recordingScriptRunner) RunScript(_ context.Context, req ScriptRequest) (Value, error) {
	r.req = req
	return Value{Raw: []byte(`"ok"`)}, nil
}

type jsonStdoutScriptRunner struct{}

func (jsonStdoutScriptRunner) RunScript(_ context.Context, _ ScriptRequest) (Value, error) {
	stdout := `{"ready_for_agent":true,"current_branch":"feature/a"}` + "\n"
	return Value{Type: "json", Raw: []byte(`{"command":["demo"],"exit_code":0,"stdout":` + strconv.Quote(stdout) + `}`)}, nil
}

type markdownStdoutScriptRunner struct{}

func (markdownStdoutScriptRunner) RunScript(_ context.Context, _ ScriptRequest) (Value, error) {
	stdout := "## Report\n\n- status: ok\n"
	return Value{Type: "json", Raw: []byte(`{"command":["demo"],"exit_code":0,"stdout":` + strconv.Quote(stdout) + `}`)}, nil
}

type countingStepAgentRunner struct{ calls int }

func (r *countingStepAgentRunner) RunAgent(_ context.Context, _ AgentRequest) (Value, error) {
	r.calls++
	return Value{Type: "json", Raw: []byte(`{"ok":true}`)}, nil
}

func TestAgentStepInjectsWholeInputContextJSON(t *testing.T) {
	agent := &recordingAgentRunner{}
	step := AgentStep{
		Base:     Base{Metadata: Metadata{ID: "consume", Kind: KindAgent}},
		Prompt:   "Use upstream result.",
		InputCtx: []string{"producer"},
	}

	_, err := step.Run(context.Background(), RunRequest{
		NodeID:  "consume",
		Context: mapContextView{"producer": {Raw: []byte(`{"issue_summary":"crash","research_brief":{"files":["main.go"]}}`)}},
		Capabilities: Capabilities{
			Agents: agent,
		},
	})
	if err != nil {
		t.Fatalf("run agent step: %v", err)
	}

	for _, want := range []string{
		"## Input context",
		"### producer",
		`"issue_summary": "crash"`,
		`"research_brief": {`,
		`"files": [`,
	} {
		if !strings.Contains(agent.prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, agent.prompt)
		}
	}
}

func TestAgentStepAppendsFormulaExecutionGuard(t *testing.T) {
	agent := &recordingAgentRunner{}
	step := AgentStep{
		Base:   Base{Metadata: Metadata{ID: "analyze", Kind: KindAgent}},
		Prompt: "Return JSON.",
	}

	_, err := step.Run(context.Background(), RunRequest{
		NodeID:       "analyze",
		Capabilities: Capabilities{Agents: agent},
	})
	if err != nil {
		t.Fatalf("run agent step: %v", err)
	}
	for _, want := range []string{
		"## Formula step execution guard",
		"Do not merely acknowledge project rules",
		"return only that JSON",
	} {
		if !strings.Contains(agent.prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, agent.prompt)
		}
	}
}

func TestAgentStepRendersRuntimeContextTemplates(t *testing.T) {
	agent := &recordingAgentRunner{}
	step := AgentStep{
		Base:   Base{Metadata: Metadata{ID: "work", Kind: KindAgent}},
		Cwd:    "{{env.cwd}}/worktree",
		Prompt: "cwd={{env.cwd}} branch={{env.git.branch}} missing={{env.missing}}",
	}

	_, err := step.Run(context.Background(), RunRequest{
		Context: mapContextView{
			"env": {Raw: []byte(`{"cwd":"/repo","git":{"branch":"main"}}`)},
		},
		Capabilities: Capabilities{Agents: agent},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(agent.prompt, "cwd=/repo") || !strings.Contains(agent.prompt, "branch=main") {
		t.Fatalf("prompt did not render env templates: %s", agent.prompt)
	}
	if !strings.Contains(agent.prompt, "missing={{env.missing}}") {
		t.Fatalf("missing template should remain unchanged: %s", agent.prompt)
	}
	if agent.req.Workspace != "/repo/worktree" {
		t.Fatalf("agent workspace = %q, want /repo/worktree", agent.req.Workspace)
	}
}

func TestScriptStepRendersRuntimeContextTemplates(t *testing.T) {
	script := &recordingScriptRunner{}
	step := ScriptStep{
		Base:    Base{Metadata: Metadata{ID: "script", Kind: KindScript}},
		Command: []string{"echo", "{{env.cwd}}", "{{env.git.branch}}"},
		Cwd:     "{{env.cwd}}",
		Env:     map[string]string{"BRANCH": "{{env.git.branch}}"},
	}

	_, err := step.Run(context.Background(), RunRequest{
		Context: mapContextView{
			"env": {Raw: []byte(`{"cwd":"/repo","git":{"branch":"main"}}`)},
		},
		Capabilities: Capabilities{Scripts: script},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(script.req.Command, " "); got != "echo /repo main" {
		t.Fatalf("command = %q", got)
	}
	if script.req.Cwd != "/repo" || script.req.Env["BRANCH"] != "main" {
		t.Fatalf("script req = %+v", script.req)
	}
}

func TestScriptStepRendersArrayIndexRuntimeContextTemplate(t *testing.T) {
	script := &recordingScriptRunner{}
	step := ScriptStep{
		Base:    Base{Metadata: Metadata{ID: "script", Kind: KindScript}},
		Command: []string{"echo", "{{classify.conflicted_branches.0}}"},
		Env:     map[string]string{"BRANCH": "{{classify.conflicted_branches.0}}"},
	}

	store := newLoopIterationStore(nil)
	_ = store.Set("classify", Value{Raw: []byte(`{"conflicted_branches":["feature/a"]}`)})
	_, err := step.Run(context.Background(), RunRequest{
		Context:      store,
		Capabilities: Capabilities{Scripts: script},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(script.req.Command, " "); got != "echo feature/a" {
		t.Fatalf("command = %q", got)
	}
	if script.req.Env["BRANCH"] != "feature/a" {
		t.Fatalf("env BRANCH = %q", script.req.Env["BRANCH"])
	}
}

func TestAggregateStepCollectsAndProjectsObjects(t *testing.T) {
	step := AggregateStep{
		Base:    Base{Metadata: Metadata{ID: "manifest", Kind: KindAggregate}},
		Source:  "write-articles",
		As:      "articles",
		Require: []string{"filename", "title", "summary", "content"},
		Exclude: []string{"content"},
	}
	res, err := step.Run(context.Background(), RunRequest{Context: mapContextView{
		"write-articles": {Raw: []byte(`{"nested":[{"filename":"01.md","title":"One","summary":"S","content":"large"},{"skip":true}]}`)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string][]map[string]any
	if err := json.Unmarshal(res.Output.Raw, &got); err != nil {
		t.Fatal(err)
	}
	articles := got["articles"]
	if len(articles) != 1 {
		t.Fatalf("articles = %+v", articles)
	}
	if _, ok := articles[0]["content"]; ok {
		t.Fatalf("content should be excluded: %+v", articles[0])
	}
	if articles[0]["filename"] != "01.md" || articles[0]["title"] != "One" {
		t.Fatalf("article projection = %+v", articles[0])
	}
}

func TestWriteFilesStepCreatesFilesAndManifest(t *testing.T) {
	tmp := t.TempDir()
	step := WriteFilesStep{
		Base:    Base{Metadata: Metadata{ID: "write", Kind: KindWriteFiles}},
		Source:  "write-articles",
		Root:    tmp,
		DirName: "{{topic_name}}",
	}
	res, err := step.Run(context.Background(), RunRequest{Context: mapContextView{
		"topic_name":     {Raw: []byte(`"demo"`)},
		"write-articles": {Raw: []byte(`[{"filename":"01.md","title":"One","summary":"S","content":"# One\nBody"}]`)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(tmp, "demo", "01.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# One\nBody\n" {
		t.Fatalf("content = %q", string(data))
	}
	var manifest struct {
		Directory string              `json:"directory"`
		Files     []map[string]string `json:"files"`
	}
	if err := json.Unmarshal(res.Output.Raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Directory != filepath.Join(tmp, "demo") || len(manifest.Files) != 1 {
		t.Fatalf("manifest = %+v", manifest)
	}
}

func TestWriteFilesStepAcceptsJSONStringObjects(t *testing.T) {
	tmp := t.TempDir()
	article, err := json.Marshal(`{"filename":"01.md","title":"One","summary":"S","content":"# One\nBody"}`)
	if err != nil {
		t.Fatal(err)
	}
	articles, err := json.Marshal([]string{string(article)})
	if err != nil {
		t.Fatal(err)
	}
	step := WriteFilesStep{Base: Base{Metadata: Metadata{ID: "write", Kind: KindWriteFiles}}, Source: "write-articles", Root: tmp, DirName: "demo"}

	res, err := step.Run(context.Background(), RunRequest{Context: mapContextView{
		"write-articles": {Raw: articles},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "demo", "01.md")); err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Files []map[string]string `json:"files"`
	}
	if err := json.Unmarshal(res.Output.Raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Files) != 1 || manifest.Files[0]["filename"] != "01.md" {
		t.Fatalf("manifest = %+v", manifest)
	}
}

func TestToolStepDispatchesWriteFiles(t *testing.T) {
	tmp := t.TempDir()
	step := ToolStep{
		Base: Base{Metadata: Metadata{ID: "tool", Kind: KindTool}},
		Name: "write_files",
		WriteFiles: &WriteFilesStep{
			Source:  "write-articles",
			Root:    tmp,
			DirName: "docs",
		},
	}
	_, err := step.Run(context.Background(), RunRequest{Context: mapContextView{
		"write-articles": {Raw: []byte(`[{"filename":"01.md","content":"# One"}]`)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "docs", "01.md")); err != nil {
		t.Fatal(err)
	}
}

func TestToolStepDispatchesSleep(t *testing.T) {
	step := ToolStep{
		Base:  Base{Metadata: Metadata{ID: "pause", Kind: KindTool}},
		Name:  "sleep",
		Sleep: &SleepStep{Duration: "1ms"},
	}
	res, err := step.Run(context.Background(), RunRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(res.Output.Raw, &got); err != nil {
		t.Fatal(err)
	}
	if got["duration"] != "1ms" {
		t.Fatalf("sleep output = %+v", got)
	}
}

func TestGitToolCommandBuilders(t *testing.T) {
	cases := []struct {
		name string
		got  []string
		want string
	}{
		{name: "fetch", got: mustGitArgs(buildGitFetchCommand(GitFetchStep{Remote: "origin", Prune: true})), want: "git fetch origin --prune"},
		{name: "push", got: mustGitArgs(buildGitPushCommand(GitPushStep{Remote: "origin", Branch: "main", SetUpstream: true})), want: "git push --set-upstream origin main"},
		{name: "branch list", got: mustGitArgs(buildGitBranchCommand(GitBranchStep{All: true})), want: "git branch --all"},
		{name: "branch create", got: mustGitArgs(buildGitBranchCommand(GitBranchStep{Name: "feature", StartPoint: "origin/main"})), want: "git branch feature origin/main"},
		{name: "checkout", got: mustGitArgs(buildGitCheckoutCommand(GitCheckoutStep{Branch: "feature", Create: true, StartPoint: "main"})), want: "git checkout -b feature main"},
		{name: "worktree add new branch", got: mustGitArgs(buildGitWorktreeCommand(GitWorktreeStep{Path: "../repo-feature", Branch: "feature", Create: true, StartPoint: "main"})), want: "git worktree add -b feature ../repo-feature main"},
		{name: "worktree add existing branch", got: mustGitArgs(buildGitWorktreeCommand(GitWorktreeStep{Path: "../repo-feature", Branch: "feature"})), want: "git worktree add ../repo-feature feature"},
		{name: "worktree list porcelain", got: mustGitArgs(buildGitWorktreeCommand(GitWorktreeStep{List: true, Porcelain: true})), want: "git worktree list --porcelain"},
		{name: "worktree remove force", got: mustGitArgs(buildGitWorktreeCommand(GitWorktreeStep{Remove: "../repo-feature", Force: true})), want: "git worktree remove --force ../repo-feature"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := strings.Join(tc.got, " "); got != tc.want {
				t.Fatalf("command = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGitWorktreeSparseCommandBuilder(t *testing.T) {
	commands, err := buildGitWorktreeCommands(GitWorktreeStep{
		Path:        "../repo-feature",
		Branch:      "feature",
		Create:      true,
		StartPoint:  "main",
		SparsePaths: []string{"cmd", "internal/formula"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := joinCommands(commands)
	want := "git worktree add --no-checkout -b feature ../repo-feature main\n" +
		"git -C ../repo-feature sparse-checkout set --cone cmd internal/formula\n" +
		"git -C ../repo-feature checkout"
	if got != want {
		t.Fatalf("commands = %q, want %q", got, want)
	}
}

func TestGitWorktreeSparseRejectsInvalidMode(t *testing.T) {
	_, err := buildGitWorktreeCommands(GitWorktreeStep{Path: "../repo-feature", SparseMode: "invalid", SparsePaths: []string{"cmd"}})
	if err == nil || !strings.Contains(err.Error(), "sparse_mode") {
		t.Fatalf("err = %v, want sparse_mode error", err)
	}
}

func TestGitWorktreeSparseRunChecksOutOnlySparsePaths(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	worktree := filepath.Join(root, "wt")
	runTestCommand(t, root, "git", "init", "repo")
	runTestCommand(t, repo, "git", "config", "user.email", "test@example.com")
	runTestCommand(t, repo, "git", "config", "user.name", "Test User")
	mustWriteFile(t, filepath.Join(repo, "cmd", "a.txt"), "a\n")
	mustWriteFile(t, filepath.Join(repo, "internal", "formula", "b.txt"), "b\n")
	mustWriteFile(t, filepath.Join(repo, "huge", "c.txt"), "c\n")
	runTestCommand(t, repo, "git", "add", ".")
	runTestCommand(t, repo, "git", "commit", "-m", "init")
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	step := GitWorktreeStep{Path: worktree, Branch: "feature", Create: true, StartPoint: "HEAD", SparsePaths: []string{"cmd", "internal/formula"}}
	if _, err := step.Run(context.Background(), RunRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(worktree, "cmd", "a.txt")); err != nil {
		t.Fatalf("sparse file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(worktree, "internal", "formula", "b.txt")); err != nil {
		t.Fatalf("nested sparse file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(worktree, "huge", "c.txt")); !os.IsNotExist(err) {
		t.Fatalf("non-sparse file exists or unexpected error: %v", err)
	}
}

func mustGitArgs(args []string, err error) []string {
	if err != nil {
		panic(err)
	}
	return args
}

func joinCommands(commands [][]string) string {
	lines := make([]string, 0, len(commands))
	for _, command := range commands {
		lines = append(lines, strings.Join(command, " "))
	}
	return strings.Join(lines, "\n")
}

func runTestCommand(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSleepToolHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (SleepStep{Duration: "1s"}).Run(ctx, RunRequest{})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestLoopStepEmitsBodyActivityEvents(t *testing.T) {
	loop := LoopStep{
		Base: Base{Metadata: Metadata{ID: "loop", Kind: KindLoop}},
		Max:  1,
		Body: []Step{NoopStep{Base: Base{Metadata: Metadata{ID: "child", Kind: KindNoop}}}},
	}
	var events []string
	_, err := loop.Run(context.Background(), RunRequest{
		NodeID: "loop",
		Emit: func(nodeID string, eventType string, payload any) {
			events = append(events, nodeID+":"+eventType)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantStarted := "loop.iter1.child:step.started"
	wantCompleted := "loop.iter1.child:step.completed"
	joined := strings.Join(events, "\n")
	if !strings.Contains(joined, wantStarted) || !strings.Contains(joined, wantCompleted) {
		t.Fatalf("events = %v, want %s and %s", events, wantStarted, wantCompleted)
	}
}

func TestLoopStepForEachSetsVarAndAggregatesOutputs(t *testing.T) {
	ctx := mapContextStore{
		"article-plan": {Raw: []byte(`[{"filename":"01.md","content":"# One"},{"filename":"02.md","content":"# Two"}]`)},
	}
	loop := LoopStep{
		Base:    Base{Metadata: Metadata{ID: "write-articles", Kind: KindLoop}},
		ForEach: "article-plan",
		Var:     "article",
		Body: []Step{
			contextEchoStep{Base: Base{Metadata: Metadata{ID: "draft", Kind: KindAgent}}},
		},
	}

	res, err := loop.Run(context.Background(), RunRequest{NodeID: "write-articles", Context: ctx, Outputs: ctx})
	if err != nil {
		t.Fatal(err)
	}
	var got []map[string]string
	if err := json.Unmarshal(res.Output.Raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0]["filename"] != "01.md" || got[1]["content"] != "# Two" {
		t.Fatalf("loop output = %+v", got)
	}
}

func TestLoopStepForEachAcceptsJSONStringArray(t *testing.T) {
	encodedPlan, err := json.Marshal(`[
		{"filename":"01.md","content":"# One"},
		{"filename":"02.md","content":"# Two"}
	]`)
	if err != nil {
		t.Fatal(err)
	}
	ctx := mapContextStore{"article-plan": {Raw: encodedPlan}}
	loop := LoopStep{
		Base:    Base{Metadata: Metadata{ID: "write-articles", Kind: KindLoop}},
		ForEach: "article-plan",
		Var:     "article",
		Body: []Step{
			contextEchoStep{Base: Base{Metadata: Metadata{ID: "draft", Kind: KindAgent}}},
		},
	}

	res, err := loop.Run(context.Background(), RunRequest{NodeID: "write-articles", Context: ctx, Outputs: ctx})
	if err != nil {
		t.Fatal(err)
	}
	var got []map[string]string
	if err := json.Unmarshal(res.Output.Raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0]["filename"] != "01.md" || got[1]["content"] != "# Two" {
		t.Fatalf("loop output = %+v", got)
	}
}

func TestLoopStepForEachParallelHonorsMaxConcurrency(t *testing.T) {
	ctx := mapContextStore{
		"article-plan": {Raw: []byte(`[
			{"filename":"01.md"},
			{"filename":"02.md"},
			{"filename":"03.md"},
			{"filename":"04.md"}
		]`)},
	}
	var current int32
	var maxSeen int32
	loop := LoopStep{
		Base:           Base{Metadata: Metadata{ID: "write-articles", Kind: KindLoop}},
		ForEach:        "article-plan",
		Var:            "article",
		Parallel:       true,
		MaxConcurrency: 2,
		Body: []Step{
			concurrencyProbeStep{Base: Base{Metadata: Metadata{ID: "draft", Kind: KindAgent}}, current: &current, maxSeen: &maxSeen, delay: 40 * time.Millisecond},
		},
	}

	started := time.Now()
	res, err := loop.Run(context.Background(), RunRequest{NodeID: "write-articles", Context: ctx, Outputs: ctx})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed >= 150*time.Millisecond {
		t.Fatalf("parallel loop took too long: %s", elapsed)
	}
	if got := atomic.LoadInt32(&maxSeen); got != 2 {
		t.Fatalf("max concurrency = %d, want 2", got)
	}
	var got []map[string]string
	if err := json.Unmarshal(res.Output.Raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got[0]["filename"] != "01.md" || got[3]["filename"] != "04.md" {
		t.Fatalf("loop output order = %+v", got)
	}
}
