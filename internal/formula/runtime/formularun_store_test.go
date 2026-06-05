package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/run"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

func TestFormulaRunStateStoreMirrorsRuntimeArtifacts(t *testing.T) {
	root := t.TempDir()
	workflow := &ir.Workflow{ID: "demo", Name: "demo", Graph: ir.NewGraph()}
	store, err := run.New(root, workflow, nil, "main", "", "session", root)
	if err != nil {
		t.Fatal(err)
	}
	bridge := NewFormulaRunStateStore(store)
	g := ir.NewGraph()
	g.AddNode(&ir.Node{ID: "a", Step: steps.NoopStep{Base: steps.Base{Metadata: steps.Metadata{ID: "a", Kind: steps.KindNoop}}}})
	wf := &ir.Workflow{ID: "demo", Graph: g}
	exec := NewExecutor(wf, steps.Capabilities{})
	exec.Store = bridge
	result, err := exec.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != steps.StatusCompleted {
		t.Fatalf("status = %s", result.Status)
	}
	meta, err := run.LoadMetadata(store.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Status != run.StatusCompleted {
		t.Fatalf("metadata status = %s", meta.Status)
	}
	var snapshot Snapshot
	if err := run.LoadState(store.Dir, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Status != steps.StatusCompleted {
		t.Fatalf("snapshot status = %s", snapshot.Status)
	}
	if _, ok := snapshot.Steps["a"]; !ok {
		t.Fatal("missing mirrored step state")
	}
	logs, err := os.ReadFile(filepath.Join(store.Dir, "logs.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) == 0 {
		t.Fatal("logs should be mirrored")
	}
}

func TestFormulaRunStateStoreMirrorsWaitingInput(t *testing.T) {
	root := t.TempDir()
	workflow := &ir.Workflow{ID: "demo", Name: "demo", Graph: ir.NewGraph()}
	store, err := run.New(root, workflow, nil, "main", "", "session", root)
	if err != nil {
		t.Fatal(err)
	}
	bridge := NewFormulaRunStateStore(store)
	await := &steps.AwaitRequest{Type: "human_input", Reason: "need input"}
	if err := bridge.StartWorkflow("demo"); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal("ignored")
	if err := bridge.SaveStep(StepState{WorkflowID: "demo", NodeID: "ask", Status: steps.StatusWaiting, Result: &steps.RunResult{Status: steps.StatusWaiting, Output: steps.Value{Type: "json", Raw: raw}, Await: await}}); err != nil {
		t.Fatal(err)
	}
	if err := bridge.FinishWorkflow("demo", steps.StatusWaiting); err != nil {
		t.Fatal(err)
	}
	meta, err := run.LoadMetadata(store.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Status != run.StatusWaitingInput {
		t.Fatalf("metadata status = %s", meta.Status)
	}
	var got steps.AwaitRequest
	if err := store.LoadStepHumanInputRequest("ask", &got); err != nil {
		t.Fatal(err)
	}
	if got.Reason != "need input" {
		t.Fatalf("await reason = %q", got.Reason)
	}
}

func TestFormulaRunStateStorePersistsRepairsArtifact(t *testing.T) {
	root := t.TempDir()
	workflow := &ir.Workflow{ID: "demo", Name: "demo", Graph: ir.NewGraph()}
	store, err := run.New(root, workflow, nil, "main", "", "session", root)
	if err != nil {
		t.Fatal(err)
	}
	bridge := NewFormulaRunStateStore(store)
	if err := bridge.StartWorkflow("demo"); err != nil {
		t.Fatal(err)
	}
	repair := RepairRecord{StepID: "script", Kind: string(steps.KindScript), Attempt: 1, Status: "succeeded", FormulaUpdateHint: "replace bad with fixed", FixedCommand: []string{"fixed"}}
	if err := bridge.SaveRepair("demo", repair); err != nil {
		t.Fatal(err)
	}
	var snapshot Snapshot
	if err := run.LoadState(store.Dir, &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Repairs) != 1 || snapshot.Repairs[0].StepID != "script" {
		t.Fatalf("snapshot repairs = %+v", snapshot.Repairs)
	}
	matches, err := filepath.Glob(filepath.Join(store.Dir, "patches", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("patch artifacts = %v, want 1 file", matches)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	var got []RepairRecord
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].FormulaUpdateHint != "replace bad with fixed" {
		t.Fatalf("persisted repairs = %+v", got)
	}
}
