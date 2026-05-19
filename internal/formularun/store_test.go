package formularun

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sjzsdu/tt/internal/formula"
)

func TestStorePersistsRunArtifacts(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	recipe := &formula.Recipe{
		Name:        "demo formula",
		Description: "demo",
		Steps:       []formula.RecipeStep{{ID: "demo", Title: "Demo", IsRoot: true}},
	}
	store, err := New(filepath.Join(root, "runs"), recipe, map[string]string{"topic": "test"}, "coder", "model", "session", workspace)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(store.Meta.RunID, "demo-formula/") {
		t.Fatalf("run id should include formula slug directory, got %q", store.Meta.RunID)
	}
	if !strings.Contains(filepath.ToSlash(store.Dir), "/runs/demo-formula/") {
		t.Fatalf("run dir should be nested under formula slug, got %s", store.Dir)
	}
	if strings.Contains(filepath.Base(store.Dir), "demo-formula") {
		t.Fatalf("leaf run dir should not repeat formula slug, got %s", filepath.Base(store.Dir))
	}
	if err := store.SaveState(map[string]string{"status": "running"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveStepPrompt("demo.step", "prompt"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveStepOutput("demo.step", "output"); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(Event{Type: "step_completed", StepID: "demo.step", Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Finish(StatusCompleted, ""); err != nil {
		t.Fatal(err)
	}

	record, err := Resolve(filepath.Join(root, "runs"), "latest")
	if err != nil {
		t.Fatal(err)
	}
	if record.Metadata.Formula != "demo formula" || record.Metadata.Status != StatusCompleted {
		t.Fatalf("unexpected metadata: %+v", record.Metadata)
	}
	if record.ID != store.Meta.RunID {
		t.Fatalf("record id = %q, want %q", record.ID, store.Meta.RunID)
	}
	for _, rel := range []string{"run.json", "recipe.json", "state.json", filepath.Join("steps", "demo.step.prompt.md"), filepath.Join("steps", "demo.step.output.md")} {
		if _, err := os.Stat(filepath.Join(store.Dir, rel)); err != nil {
			t.Fatalf("expected artifact %s: %v", rel, err)
		}
	}
	logs, err := os.ReadFile(filepath.Join(store.Dir, "logs.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range [][]byte{[]byte(`"type":"run_started"`), []byte(`"type":"step_completed"`), []byte(`"type":"run_finished"`)} {
		if !bytes.Contains(logs, want) {
			t.Fatalf("expected logs to contain %s; got %s", want, logs)
		}
	}
}

func TestNewDefaultRootEnsuresTTAndGitIgnore(t *testing.T) {
	workspace := t.TempDir()
	recipe := &formula.Recipe{Name: "demo", Steps: []formula.RecipeStep{{ID: "demo", Title: "Demo", IsRoot: true}}}
	store, err := New("", recipe, nil, "", "", "", workspace)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(store.Dir, filepath.Join(workspace, ".tt", "runs", "formula", "demo")) {
		t.Fatalf("run dir should be under workspace .tt, got %s", store.Dir)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".tt")); err != nil {
		t.Fatalf("expected .tt directory: %v", err)
	}
	gitignore, err := os.ReadFile(filepath.Join(workspace, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(gitignore, []byte(".tt/")) {
		t.Fatalf("expected .gitignore to contain .tt/: %s", gitignore)
	}
}

func TestLoadMetadataMarksDeadRunningRunStale(t *testing.T) {
	dir := t.TempDir()
	meta := Metadata{RunID: "run-stale", Formula: "demo", Status: StatusRunning, StartedAt: "2026-01-01T00:00:00Z", PID: -1}
	if err := writeJSON(filepath.Join(dir, "run.json"), meta); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadMetadata(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != StatusStale {
		t.Fatalf("expected stale status, got %+v", loaded)
	}
	logs, err := os.ReadFile(filepath.Join(dir, "logs.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(logs, []byte(`"type":"run_stale"`)) {
		t.Fatalf("expected run_stale event, got %s", logs)
	}
}

func TestDeleteRemovesRunDirectory(t *testing.T) {
	root := t.TempDir()
	recipe := &formula.Recipe{Name: "delete-demo", Steps: []formula.RecipeStep{{ID: "delete-demo", Title: "Demo", IsRoot: true}}}
	store, err := New(root, recipe, nil, "", "", "", root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := Delete(root, store.Meta.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if record.ID != store.Meta.RunID {
		t.Fatalf("deleted wrong run: %+v", record)
	}
	if _, err := os.Stat(store.Dir); !os.IsNotExist(err) {
		t.Fatalf("expected run dir removed, stat err=%v", err)
	}
}

func TestListIncludesLegacyFlatRunDirectories(t *testing.T) {
	root := t.TempDir()
	legacyDir := filepath.Join(root, "20260101-000000-demo-abc123")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := Metadata{RunID: "20260101-000000-demo-abc123", Formula: "demo", Status: StatusCompleted, StartedAt: "2026-01-01T00:00:00Z"}
	if err := writeJSON(filepath.Join(legacyDir, "run.json"), meta); err != nil {
		t.Fatal(err)
	}
	records, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ID != meta.RunID {
		t.Fatalf("records = %+v, want legacy flat run", records)
	}
}
