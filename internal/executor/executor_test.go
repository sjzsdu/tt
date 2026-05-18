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
