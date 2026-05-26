package steps

import (
	"context"
	"strings"
	"testing"
)

type mapContextView map[string]Value

func (m mapContextView) Get(path string) (Value, bool) {
	v, ok := m[path]
	return v, ok
}

type recordingAgentRunner struct {
	prompt string
}

func (r *recordingAgentRunner) RunAgent(_ context.Context, req AgentRequest) (Value, error) {
	r.prompt = req.Prompt
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
