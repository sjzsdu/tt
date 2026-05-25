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
	workflow, err := CompileWorkflowByName(context.Background(), entry.Name, nil, map[string]string{"topic": "空间计算"})
	if err != nil {
		t.Fatalf("Compile(%q) error = %v", entry.Name, err)
	}
	if workflow.Name != entry.Name {
		t.Fatalf("workflow.Name = %q, want %q", workflow.Name, entry.Name)
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

func TestShanYiZheBuiltinFormula(t *testing.T) {
	p := NewParser()
	f, err := p.LoadByName("shan-yi-zhe")
	if err != nil {
		t.Fatalf("LoadByName(shan-yi-zhe) error = %v", err)
	}
	if f.Source != "builtin:shan-yi-zhe" {
		t.Fatalf("Source = %q, want builtin:shan-yi-zhe", f.Source)
	}
	stepIDs := make(map[string]bool)
	for _, s := range f.Steps {
		stepIDs[s.ID] = true
	}
	for _, want := range []string{"discern-situation", "cast-frame", "line-plan", "interpret-lines", "change-reading", "life-guidance"} {
		if !stepIDs[want] {
			t.Fatalf("expected step %q in shan-yi-zhe", want)
		}
	}
	if err := f.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if _, err := CompileWorkflowByName(context.Background(), "shan-yi-zhe", nil, map[string]string{"question": "我是否应该离开现在的工作去创业？"}); err != nil {
		t.Fatalf("Compile(shan-yi-zhe) error = %v", err)
	}
}
