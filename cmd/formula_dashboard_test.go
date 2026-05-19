package cmd

import (
	"testing"

	"github.com/sjzsdu/tt/internal/formula"
)

func TestBuildFormulaDashboardGraphHidesGeneratedNoopBoundaries(t *testing.T) {
	recipe := &formula.Recipe{
		Name: "demo",
		Steps: []formula.RecipeStep{
			{ID: "demo", IsRoot: true},
			{ID: "demo.start", Title: "start", Execution: "noop", Metadata: map[string]string{"formula_boundary": "start"}},
			{ID: "demo.work", Title: "Work"},
			{ID: "demo.end", Title: "end", Execution: "noop", Metadata: map[string]string{"formula_boundary": "end"}},
		},
		Deps: []formula.RecipeDep{
			{StepID: "demo.start", DependsOnID: "demo", Type: "blocks"},
			{StepID: "demo.work", DependsOnID: "demo.start", Type: "blocks"},
			{StepID: "demo.end", DependsOnID: "demo.work", Type: "blocks"},
		},
	}

	steps, edges := buildFormulaDashboardGraph(recipe)
	if len(steps) != 1 || steps[0].ID != "demo.work" {
		t.Fatalf("dashboard steps = %+v, want only real work step", steps)
	}
	if len(edges) != 0 {
		t.Fatalf("dashboard edges = %+v, want hidden boundary edges omitted", edges)
	}
}

func TestBuildFormulaDashboardGraphKeepsExplicitBoundaryWork(t *testing.T) {
	recipe := &formula.Recipe{
		Name: "demo",
		Steps: []formula.RecipeStep{
			{ID: "demo", IsRoot: true},
			{ID: "demo.start", Title: "Custom start", Metadata: map[string]string{"formula_boundary": "start"}},
			{ID: "demo.work", Title: "Work"},
		},
		Deps: []formula.RecipeDep{{StepID: "demo.work", DependsOnID: "demo.start", Type: "blocks"}},
	}

	steps, edges := buildFormulaDashboardGraph(recipe)
	if len(steps) != 2 {
		t.Fatalf("dashboard should keep explicit non-noop boundary, got %+v", steps)
	}
	if len(edges) != 1 || edges[0].From != "demo.start" || edges[0].To != "demo.work" {
		t.Fatalf("dashboard edges = %+v, want explicit start -> work", edges)
	}
}
