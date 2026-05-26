package steps

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

type recordingAgentRunner struct {
	prompt string
}

func (r *recordingAgentRunner) RunAgent(_ context.Context, req AgentRequest) (Value, error) {
	r.prompt = req.Prompt
	return Value{Raw: []byte(`"ok"`)}, nil
}

type recordingScriptRunner struct {
	req ScriptRequest
}

func (r *recordingScriptRunner) RunScript(_ context.Context, req ScriptRequest) (Value, error) {
	r.req = req
	return Value{Raw: []byte(`"ok"`)}, nil
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

func TestAgentStepRendersRuntimeContextTemplates(t *testing.T) {
	agent := &recordingAgentRunner{}
	step := AgentStep{
		Base:   Base{Metadata: Metadata{ID: "work", Kind: KindAgent}},
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
