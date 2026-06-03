package formulaui

import "testing"

func TestAppendStepActivityPreservesSessionOnUpdate(t *testing.T) {
	step := &Step{ID: "loop", Title: "Loop"}

	AppendStepActivity(step, StepActivity{
		At:      "10:00:00",
		StepID:  "loop.iter1.agent-step",
		Title:   "Agent step",
		Status:  "running",
		Session: "cli:formula.loop.iter1.agent-step",
	})
	AppendStepActivity(step, StepActivity{
		At:     "10:01:00",
		StepID: "loop.iter1.agent-step",
		Status: "completed",
		Output: "{}",
	})

	if len(step.Activities) != 1 {
		t.Fatalf("activities = %d, want 1", len(step.Activities))
	}
	activity := step.Activities[0]
	if activity.Session != "cli:formula.loop.iter1.agent-step" {
		t.Fatalf("session = %q, want preserved session", activity.Session)
	}
	if activity.Title != "Agent step" {
		t.Fatalf("title = %q, want preserved title", activity.Title)
	}
	if activity.Status != "completed" {
		t.Fatalf("status = %q, want completed", activity.Status)
	}
}
