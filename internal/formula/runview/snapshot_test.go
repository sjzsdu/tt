package runview

import (
	"testing"

	"github.com/sjzsdu/tt/internal/formula/ui"
)

func TestNestedWaitingInputCanBeCompletedAndResumed(t *testing.T) {
	address := "invoke.formula(child).approve"
	snapshot := ui.Snapshot{
		Steps:              []ui.Step{{ID: "invoke", Status: ui.StatusWaitingInput}},
		ExecutionInstances: []ui.ExecutionInstance{{Address: address, DefinitionStepID: "approve", Status: ui.StatusWaitingInput}},
	}
	resolved, err := ResolveWaitingInputStepID(snapshot, address)
	if err != nil || resolved != address {
		t.Fatalf("resolved=%q err=%v", resolved, err)
	}
	if err := MarkStepCompletedWithOutput(&snapshot, address, `{"approved":true}`); err != nil {
		t.Fatal(err)
	}
	results, _ := BuildResumeState(snapshot)
	found := false
	for _, result := range results {
		if result.StepID == address && result.Status == ui.StatusCompleted {
			found = true
		}
	}
	if !found {
		t.Fatalf("resume results = %+v", results)
	}
}
