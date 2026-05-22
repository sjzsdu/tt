package formula

import (
	"context"
	"testing"
)

func TestBuiltinFormulasParseAndCompile(t *testing.T) {
	entries, err := BuiltinFormulas()
	if err != nil {
		t.Fatalf("BuiltinFormulas() error = %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected at least one builtin formula")
	}

	// Find fresh-topic-docs among the entries
	var entry *BuiltinEntry
	for i := range entries {
		if entries[i].Name == "fresh-topic-docs" {
			entry = &entries[i]
			break
		}
	}
	if entry == nil {
		t.Fatalf("fresh-topic-docs not found in builtin formulas: %v", entries)
	}
	if entry.Description == "" || entry.Title == "" || entry.Category == "" {
		t.Fatalf("builtin metadata incomplete: %+v", entry)
	}

	p := NewParser()
	f, err := p.LoadByName(entry.Name)
	if err != nil {
		t.Fatalf("LoadByName(%q) error = %v", entry.Name, err)
	}
	if f.Source != "builtin:"+entry.Name {
		t.Fatalf("Source = %q, want builtin:%s", f.Source, entry.Name)
	}
	if len(f.Steps) < 3 {
		t.Fatalf("expected multi-step document workflow, got %d steps", len(f.Steps))
	}
	stepIDs := make(map[string]bool)
	var idList []string
	for _, s := range f.Steps {
		stepIDs[s.ID] = true
		idList = append(idList, s.ID)
	}
	for _, want := range []string{"scope-analysis", "write-articles", "series-package"} {
		if !stepIDs[want] {
			t.Fatalf("expected step %q in formula, got steps: %v", want, idList)
		}
	}
	if err := f.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	recipe, err := Compile(context.Background(), entry.Name, nil, map[string]string{"topic": "空间计算"})
	if err != nil {
		t.Fatalf("Compile(%q) error = %v", entry.Name, err)
	}
	if recipe.Name != entry.Name {
		t.Fatalf("recipe.Name = %q, want %q", recipe.Name, entry.Name)
	}
}

func TestBuiltinFormulaContent(t *testing.T) {
	data, ok, err := BuiltinFormulaContent("fresh-topic-docs")
	if err != nil {
		t.Fatalf("BuiltinFormulaContent error = %v", err)
	}
	if !ok || len(data) == 0 {
		t.Fatalf("expected fresh-topic-docs content")
	}
}
