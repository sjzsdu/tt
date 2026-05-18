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
	if !strings.HasPrefix(store.Dir, filepath.Join(workspace, ".tt", "runs", "formula")) {
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
