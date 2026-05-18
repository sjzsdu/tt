package formularun

import (
	"os"
	"path/filepath"
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
}
