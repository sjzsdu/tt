package runtime

import (
	"context"
	"testing"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

func TestExecutorPersistsStateAndEvents(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "a", Step: steps.NoopStep{Base: steps.Base{Metadata: steps.Metadata{ID: "a", Kind: steps.KindNoop}}}})
	wf := &ir.Workflow{ID: "state-demo", Graph: g}
	store := NewMemoryStateStore()
	exec := NewExecutor(wf, steps.Capabilities{})
	exec.Store = store
	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != steps.StatusCompleted {
		t.Fatalf("status = %s", result.Status)
	}
	snapshot, err := store.Snapshot(wf.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Status != steps.StatusCompleted {
		t.Fatalf("snapshot status = %s", snapshot.Status)
	}
	state, ok := snapshot.Steps["a"]
	if !ok {
		t.Fatal("missing step state")
	}
	if state.Status != steps.StatusCompleted {
		t.Fatalf("step status = %s", state.Status)
	}
	if len(snapshot.Events) < 3 {
		t.Fatalf("events = %d, want workflow/step events", len(snapshot.Events))
	}
}

func TestExecutorPersistsWaitingState(t *testing.T) {
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "ask", Step: steps.HumanInputStep{Base: steps.Base{Metadata: steps.Metadata{ID: "ask", Kind: steps.KindHumanInput}}, Reason: "need input"}})
	wf := &ir.Workflow{ID: "waiting-demo", Graph: g}
	store := NewMemoryStateStore()
	exec := NewExecutor(wf, steps.Capabilities{})
	exec.Store = store
	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != steps.StatusWaiting {
		t.Fatalf("status = %s", result.Status)
	}
	snapshot, err := store.Snapshot(wf.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Status != steps.StatusWaiting {
		t.Fatalf("snapshot status = %s", snapshot.Status)
	}
	state := snapshot.Steps["ask"]
	if state.Status != steps.StatusWaiting || state.Result == nil || state.Result.Await == nil {
		t.Fatalf("bad waiting state: %+v", state)
	}
}
