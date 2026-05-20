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

func TestFormulaRunWebDashboardEnabledByDefault(t *testing.T) {
	webFlag := formulaRunCmd.Flags().Lookup("web")
	if webFlag == nil {
		t.Fatal("missing --web flag")
	}
	if webFlag.DefValue != "true" {
		t.Fatalf("--web default = %q, want true", webFlag.DefValue)
	}
	if noWebFlag := formulaRunCmd.Flags().Lookup("no-web"); noWebFlag == nil {
		t.Fatal("missing --no-web opt-out flag")
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

func TestFormulaDashboardLoopBodyActivityRollsUpToParentStep(t *testing.T) {
	recipe := &formula.Recipe{
		Name: "demo",
		Steps: []formula.RecipeStep{
			{ID: "demo", IsRoot: true},
			{ID: "demo.review", Title: "Review loop"},
		},
	}
	dashboard := newFormulaDashboardServer(recipe)

	dashboard.markStepRunning("demo.review.iter1.check", "Check iteration 1", "agent", "model", "session")
	dashboard.markStepCompleted("demo.review.iter1.check", "approved=false")

	snapshot := dashboard.snapshot()
	if got := snapshot.Steps[0].Status; got != "running" {
		t.Fatalf("parent status = %q, want running while loop body is recorded", got)
	}
	if len(snapshot.Steps[0].Activities) != 1 {
		t.Fatalf("activities = %+v, want one rolled-up loop activity", snapshot.Steps[0].Activities)
	}
	activity := snapshot.Steps[0].Activities[0]
	if activity.StepID != "demo.review.iter1.check" || activity.Status != "completed" || activity.Output != "approved=false" {
		t.Fatalf("activity = %+v, want completed loop body output", activity)
	}
}

func TestBuildFormulaDashboardGraphIncludesLoopPlan(t *testing.T) {
	recipe := &formula.Recipe{
		Name: "demo",
		Steps: []formula.RecipeStep{
			{ID: "demo", IsRoot: true},
			{ID: "demo.loop", Title: "Loop", Loop: &formula.LoopSpec{Until: "review.approved == true", Max: 3, Body: []*formula.Step{{ID: "review", Title: "Review", OutputKey: "review"}}}},
		},
	}

	steps, _ := buildFormulaDashboardGraph(recipe)
	if len(steps) != 1 || steps[0].Loop == nil {
		t.Fatalf("dashboard step loop = %+v, want loop plan", steps)
	}
	if steps[0].Loop.Summary != "until review.approved == true · max 3" {
		t.Fatalf("loop summary = %q", steps[0].Loop.Summary)
	}
	if len(steps[0].Loop.Body) != 1 || steps[0].Loop.Body[0].ID != "review" || steps[0].Loop.Body[0].OutputKey != "review" {
		t.Fatalf("loop body = %+v, want review body", steps[0].Loop.Body)
	}
}
