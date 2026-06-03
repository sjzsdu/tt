package runtime

import (
	"encoding/json"
	"testing"

	"github.com/sjzsdu/tt/internal/formula/steps"
)

func TestContextStoreGetReadsFieldsFromJSONStringOutputs(t *testing.T) {
	store := NewContextStore()
	raw, err := json.Marshal(`{"needs_strategy_choice":true,"nested":{"value":"ok"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("plan-feature", steps.Value{Type: "json", Raw: raw}); err != nil {
		t.Fatal(err)
	}
	value, ok := store.Get("plan-feature.needs_strategy_choice")
	if !ok {
		t.Fatal("needs_strategy_choice not found")
	}
	var got bool
	if err := json.Unmarshal(value.Raw, &got); err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatal("needs_strategy_choice = false, want true")
	}
	if actual, ok := lookupConditionValue(store, "plan-feature.nested.value"); !ok || actual != "ok" {
		t.Fatalf("nested condition value = %q, %v; want ok, true", actual, ok)
	}
}

func TestContextStoreGetUsesLongestStoredPrefixForDottedStepIDs(t *testing.T) {
	store := NewContextStore()
	raw, err := json.Marshal(map[string]any{"stdout": map[string]any{"ok": true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("run-pr-rebase.prepare-rebase-worktree", steps.Value{Type: "json", Raw: raw}); err != nil {
		t.Fatal(err)
	}
	value, ok := store.Get("run-pr-rebase.prepare-rebase-worktree.stdout.ok")
	if !ok {
		t.Fatal("dotted step output not found")
	}
	var got bool
	if err := json.Unmarshal(value.Raw, &got); err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatal("ok = false, want true")
	}
}

func TestContextStoreGetReadsArrayIndexPath(t *testing.T) {
	store := NewContextStore()
	raw, err := json.Marshal(map[string]any{"conflicted_branches": []any{"feature/a", "feature/b"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("classify-run-state", steps.Value{Type: "json", Raw: raw}); err != nil {
		t.Fatal(err)
	}
	value, ok := store.Get("classify-run-state.conflicted_branches.0")
	if !ok {
		t.Fatal("array index path not found")
	}
	var got string
	if err := json.Unmarshal(value.Raw, &got); err != nil {
		t.Fatal(err)
	}
	if got != "feature/a" {
		t.Fatalf("branch = %q, want feature/a", got)
	}
}
