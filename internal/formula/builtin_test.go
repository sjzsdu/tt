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
		t.Fatal("expected builtin formulas")
	}
	for _, entry := range entries {
		t.Run(entry.Name, func(t *testing.T) {
			if entry.Description == "" {
				t.Fatalf("builtin %s missing description", entry.Name)
			}
			p := NewParser()
			f, err := p.LoadByName(entry.Name)
			if err != nil {
				t.Fatalf("LoadByName(%q) error = %v", entry.Name, err)
			}
			if f.Source != "builtin:"+entry.Name {
				t.Fatalf("Source = %q, want builtin:%s", f.Source, entry.Name)
			}
			if err := f.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			recipe, err := Compile(context.Background(), entry.Name, nil, nil)
			if err != nil {
				t.Fatalf("Compile(%q) error = %v", entry.Name, err)
			}
			if recipe.Name != entry.Name {
				t.Fatalf("recipe.Name = %q, want %q", recipe.Name, entry.Name)
			}
		})
	}
}

func TestBuiltinFormulaContent(t *testing.T) {
	data, ok, err := BuiltinFormulaContent("daily-plan")
	if err != nil {
		t.Fatalf("BuiltinFormulaContent error = %v", err)
	}
	if !ok || len(data) == 0 {
		t.Fatalf("expected daily-plan content")
	}
}
