package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/sjzsdu/tt/internal/formula"
)

func TestExecutorPausesForStaticHumanInputStep(t *testing.T) {
	recipe := &formula.Recipe{Name: "demo", Steps: []formula.RecipeStep{{
		ID:        "demo.profile",
		Title:     "Profile",
		Execution: HumanInputExecution,
		Form: &formula.FormSpec{Title: "Profile", Fields: []*formula.FormField{{
			Name: "level", Label: "Level", Type: "radio", Options: []string{"beginner", "advanced"}, Required: true,
		}}},
	}}}
	exec := New(recipe, RunOptions{})
	result, err := exec.Run(context.Background(), func(context.Context, *formula.RecipeStep, string) (string, error) {
		t.Fatal("runner should not be called for static human_input step")
		return "", nil
	})
	var waiting WaitingInputError
	if !errors.As(err, &waiting) {
		t.Fatalf("err = %v, want WaitingInputError", err)
	}
	if waiting.StepID != "demo.profile" || waiting.Request == nil || waiting.Request.Form == nil {
		t.Fatalf("waiting error mismatch: %+v", waiting)
	}
	if result == nil || result.WaitingInput != 1 || len(result.Steps) != 1 {
		t.Fatalf("result mismatch: %+v", result)
	}
	if result.Steps[0].Status != StatusWaitingInput || result.Steps[0].HumanInputRequest == nil {
		t.Fatalf("step result mismatch: %+v", result.Steps[0])
	}
}

func TestExecutorPausesForDynamicHumanInputBlock(t *testing.T) {
	recipe := &formula.Recipe{Name: "demo", Steps: []formula.RecipeStep{{ID: "demo.ask", Title: "Ask"}}}
	exec := New(recipe, RunOptions{})
	output := "before\n```tt-human-input\n{\"reason\":\"need context\",\"form\":{\"title\":\"Context\",\"fields\":[{\"name\":\"goal\",\"label\":\"Goal\",\"type\":\"textarea\",\"required\":true}]}}\n```\nafter"
	result, err := exec.Run(context.Background(), func(context.Context, *formula.RecipeStep, string) (string, error) {
		return output, nil
	})
	var waiting WaitingInputError
	if !errors.As(err, &waiting) {
		t.Fatalf("err = %v, want WaitingInputError", err)
	}
	if waiting.Request == nil || waiting.Request.Reason != "need context" || waiting.Request.Form == nil {
		t.Fatalf("request mismatch: %+v", waiting.Request)
	}
	if result == nil || result.WaitingInput != 1 || result.Steps[0].Status != StatusWaitingInput {
		t.Fatalf("result mismatch: %+v", result)
	}
}

func TestParseHumanInputRequestIgnoresInvalidBlocks(t *testing.T) {
	if req := ParseHumanInputRequest("```tt-human-input\nnot-json\n```"); req != nil {
		t.Fatalf("expected nil for invalid block, got %+v", req)
	}
}

func TestParseHumanInputRequestAcceptsUppercaseJSONInfo(t *testing.T) {
	output := "```tt-human-input JSON\n{\"reason\":\"need context\",\"form\":{\"title\":\"Context\",\"fields\":[{\"name\":\"goal\",\"label\":\"Goal\",\"type\":\"textarea\"}]}}\n```"
	req := ParseHumanInputRequest(output)
	if req == nil || req.Form == nil || req.Form.Title != "Context" {
		t.Fatalf("request mismatch: %+v", req)
	}
}
