package cmd

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	formulasteps "github.com/sjzsdu/tt/internal/formula/steps"
	teamruntime "github.com/sjzsdu/tt/internal/team"
)

type capturingTeamExternalRunner struct {
	requests []formulasteps.ExternalAgentRequest
}

func (r *capturingTeamExternalRunner) RunExternalAgent(_ context.Context, req formulasteps.ExternalAgentRequest) (formulasteps.Value, error) {
	r.requests = append(r.requests, req)
	data, _ := json.Marshal(map[string]any{
		"session_id": "external-session",
		"text":       "external answer",
	})
	return formulasteps.Value{Type: "json", Raw: data}, nil
}

func TestTeamProcessorRoutesAndResumesExternalAgent(t *testing.T) {
	runner := &capturingTeamExternalRunner{}
	processor := &teamPicoclawProcessor{
		workspace:        t.TempDir(),
		defaultModel:     "default-model",
		externalRunner:   runner,
		externalSessions: map[string]string{},
		externalLocks:    map[string]*sync.Mutex{},
	}
	call := teamruntime.AgentCall{
		MemberID: "implementer",
		Model:    "codex-model",
		Prompt:   "implement it",
		External: &teamruntime.ExternalAgentConfig{
			Driver:    "codex",
			Resume:    "configured-session",
			Timeout:   "2m",
			ExtraArgs: []string{"--full-auto"},
		},
	}
	for range 2 {
		answer, err := processor.Process(context.Background(), call)
		if err != nil {
			t.Fatal(err)
		}
		if answer != "external answer" {
			t.Fatalf("answer = %q", answer)
		}
	}
	if len(runner.requests) != 2 {
		t.Fatalf("requests = %d", len(runner.requests))
	}
	if runner.requests[0].Resume != "configured-session" {
		t.Fatalf("initial resume = %q", runner.requests[0].Resume)
	}
	if runner.requests[1].Resume != "external-session" {
		t.Fatalf("continued resume = %q", runner.requests[1].Resume)
	}
	if runner.requests[0].Model != "codex-model" {
		t.Fatalf("model = %q", runner.requests[0].Model)
	}
}
