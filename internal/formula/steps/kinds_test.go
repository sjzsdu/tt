package steps

import (
	"context"
	"encoding/json"
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
