package ui

import (
	"reflect"
	"testing"
	"time"
)

func TestParseExecutionAddressSupportsNestedLoops(t *testing.T) {
	parent, path, body := ParseExecutionAddress("outer.iter3.inner.iter2.summarize")
	if parent != "outer" || body != "summarize" || !reflect.DeepEqual(path, []int{3, 2}) {
		t.Fatalf("parsed = parent %q path %v body %q", parent, path, body)
	}
}

func TestRecordExecutionTransitionPreservesImmutableHistory(t *testing.T) {
	var snapshot Snapshot
	started := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	RecordExecutionTransition(&snapshot, ExecutionTransition{
		Address: "cycle.iter2.fetch", Title: "Fetch", Status: StatusRunning,
		Session: "session-2", Detail: "started", At: started,
	})
	RecordExecutionTransition(&snapshot, ExecutionTransition{
		Address: "cycle.iter2.fetch", Status: StatusCompleted,
		Detail: "done", Output: "ok", At: started.Add(1500 * time.Millisecond),
	})

	if len(snapshot.ExecutionInstances) != 1 || len(snapshot.ExecutionEvents) != 2 {
		t.Fatalf("instances=%d events=%d", len(snapshot.ExecutionInstances), len(snapshot.ExecutionEvents))
	}
	instance := snapshot.ExecutionInstances[0]
	if instance.Status != StatusCompleted || instance.Session != "session-2" || instance.DurationMS != 1500 {
		t.Fatalf("instance = %+v", instance)
	}
	if snapshot.ExecutionEvents[0].Status != StatusRunning || snapshot.ExecutionEvents[1].FromStatus != StatusRunning {
		t.Fatalf("events = %+v", snapshot.ExecutionEvents)
	}
	if len(instance.Path) != 3 || instance.Path[1].Kind != "iteration" || instance.Path[1].Index != 2 {
		t.Fatalf("structured path = %+v", instance.Path)
	}
}

func TestRecordExecutionTransitionIncrementsAttemptAfterTerminalState(t *testing.T) {
	var snapshot Snapshot
	started := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	RecordExecutionTransition(&snapshot, ExecutionTransition{Address: "work", Status: StatusRunning, At: started})
	RecordExecutionTransition(&snapshot, ExecutionTransition{Address: "work", Status: StatusFailed, At: started.Add(time.Second)})
	RecordExecutionTransition(&snapshot, ExecutionTransition{Address: "work", Status: StatusRunning, At: started.Add(2 * time.Second)})
	if snapshot.ExecutionInstances[0].Attempt != 2 || snapshot.ExecutionEvents[2].Attempt != 2 {
		t.Fatalf("instance=%+v events=%+v", snapshot.ExecutionInstances[0], snapshot.ExecutionEvents)
	}
}
