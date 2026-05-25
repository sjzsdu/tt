package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sjzsdu/tt/internal/formula"
	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/steps"
	"github.com/sjzsdu/tt/internal/formularun"
)

func TestFormulaRunStateStoreMirrorsRuntimeArtifacts(t *testing.T) {
	root := t.TempDir()
	recipe := &formula.Recipe{Name: "demo", Vars: map[string]*formula.VarDef{}, Steps: []formula.RecipeStep{{ID: "a", Title: "A"}}}
	store, err := formularun.New(root, recipe, nil, "main", "", "session", root)
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
	meta, err := formularun.LoadMetadata(store.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Status != formularun.StatusCompleted {
		t.Fatalf("metadata status = %s", meta.Status)
	}
	var snapshot Snapshot
	if err := formularun.LoadState(store.Dir, &snapshot); err != nil {
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
	recipe := &formula.Recipe{Name: "demo", Vars: map[string]*formula.VarDef{}, Steps: []formula.RecipeStep{{ID: "ask", Title: "Ask", Execution: "human_input"}}}
	store, err := formularun.New(root, recipe, nil, "main", "", "session", root)
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
	meta, err := formularun.LoadMetadata(store.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Status != formularun.StatusWaitingInput {
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
