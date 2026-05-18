package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/sjzsdu/tt/internal/formula"
)

func TestExecutorSkipsInitialCompletedResults(t *testing.T) {
	recipe := &formula.Recipe{
		Name: "resume-demo",
		Steps: []formula.RecipeStep{
			{ID: "resume-demo", Title: "Root", IsRoot: true},
			{ID: "resume-demo.first", Title: "First", OutputKey: "first_out"},
			{ID: "resume-demo.second", Title: "Second", InputCtx: []string{"first_out"}},
		},
		Deps: []formula.RecipeDep{
			{StepID: "resume-demo.first", DependsOnID: "resume-demo", Type: "parent-child"},
			{StepID: "resume-demo.second", DependsOnID: "resume-demo", Type: "parent-child"},
			{StepID: "resume-demo.second", DependsOnID: "resume-demo.first", Type: "blocks"},
		},
	}
	exec := New(recipe, RunOptions{
		InitialContext: map[string]string{"first_out": "saved output"},
		InitialResults: []StepResult{{StepID: "resume-demo.first", Title: "First", Status: StatusCompleted, Output: "saved output"}},
	})
	called := []string{}
	result, err := exec.Run(context.Background(), func(ctx context.Context, step *formula.RecipeStep, prompt string) (string, error) {
		called = append(called, step.ID)
		if step.ID == "resume-demo.first" {
			t.Fatalf("completed step should not rerun")
		}
		if step.ID == "resume-demo.second" && !strings.Contains(prompt, "saved output") {
			t.Fatalf("resume context missing from prompt: %s", prompt)
		}
		return "second output", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(called) != 1 || called[0] != "resume-demo.second" {
		t.Fatalf("called steps = %v", called)
	}
	if result.Completed != 3 {
		t.Fatalf("completed = %d, want 3", result.Completed)
	}
}

func TestEvaluateConditionSupportsJSONPath(t *testing.T) {
	ctx := map[string]string{"decision": `{"approved":true,"score": 9, "kind":"frontend"}`}
	if !EvaluateCondition("decision.approved == true", ctx) {
		t.Fatalf("expected boolean JSON path condition to pass")
	}
	if !EvaluateCondition("decision.kind == frontend", ctx) {
		t.Fatalf("expected string JSON path condition to pass")
	}
	if !EvaluateCondition("decision.score == 9", ctx) {
		t.Fatalf("expected numeric JSON path condition to pass")
	}
}

func TestExecutorRuntimeUntilLoopUsesAgentOutput(t *testing.T) {
	recipe := &formula.Recipe{
		Name: "runtime-loop",
		Steps: []formula.RecipeStep{
			{ID: "runtime-loop", Title: "Root", IsRoot: true},
			{
				ID:    "runtime-loop.improve",
				Title: "Improve until approved",
				Loop:  &formula.LoopSpec{Until: "review.approved == true", Max: 3, Body: []*formula.Step{{ID: "review", Title: "Review {{iteration}}", OutputKey: "review"}}},
			},
		},
		Deps: []formula.RecipeDep{{StepID: "runtime-loop.improve", DependsOnID: "runtime-loop", Type: "parent-child"}},
	}
	calls := 0
	result, err := New(recipe, RunOptions{}).Run(context.Background(), func(ctx context.Context, step *formula.RecipeStep, prompt string) (string, error) {
		calls++
		if calls == 1 {
			return `{"approved":false}`, nil
		}
		return `{"approved":true}`, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	if result.Completed != 4 { // root + loop + two loop-body iterations
		t.Fatalf("completed = %d, want 4; result=%+v", result.Completed, result)
	}
}
