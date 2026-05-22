package cmd

import (
	"testing"

	"github.com/sjzsdu/tt/internal/executor"
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

func TestBuildResumeStateExcludingRetriedStep(t *testing.T) {
	recipe := &formula.Recipe{
		Name: "demo",
		Steps: []formula.RecipeStep{
			{ID: "demo.done", Title: "Done", OutputKey: "done"},
			{ID: "demo.retry", Title: "Retry", OutputKey: "retry"},
		},
	}
	snapshot := formulaDashboardSnapshot{Steps: []formulaDashboardStep{
		{ID: "demo.done", Title: "Done", Status: string(executor.StatusCompleted), Output: "done-output"},
		{ID: "demo.retry", Title: "Retry", Status: string(executor.StatusCompleted), Output: "old-output"},
	}}
	results, ctx := buildResumeStateExcluding(recipe, snapshot, map[string]bool{"demo.retry": true})
	if len(results) != 1 || results[0].StepID != "demo.done" {
		t.Fatalf("results = %+v, want only non-retried completed step", results)
	}
	if ctx["done"] != "done-output" {
		t.Fatalf("ctx[done] = %q, want done-output", ctx["done"])
	}
	if _, ok := ctx["retry"]; ok {
		t.Fatalf("ctx contains retried output: %+v", ctx)
	}
}

func TestResolveFormulaDashboardStepIDAllowsFailedRetryTargets(t *testing.T) {
	snapshot := formulaDashboardSnapshot{Steps: []formulaDashboardStep{
		{ID: "fresh-topic-docs.write-articles", Title: "Write articles", Status: string(executor.StatusFailed)},
	}}

	got, err := resolveFormulaDashboardStepID(snapshot, "write-articles")
	if err != nil {
		t.Fatalf("resolveFormulaDashboardStepID returned error: %v", err)
	}
	if got != "fresh-topic-docs.write-articles" {
		t.Fatalf("resolved step = %q, want fresh-topic-docs.write-articles", got)
	}
}

func TestResolveFormulaRunStepIDStillRequiresWaitingInput(t *testing.T) {
	snapshot := formulaDashboardSnapshot{Steps: []formulaDashboardStep{
		{ID: "fresh-topic-docs.write-articles", Title: "Write articles", Status: string(executor.StatusFailed)},
	}}

	if _, err := resolveFormulaRunStepID(snapshot, "write-articles"); err == nil {
		t.Fatal("resolveFormulaRunStepID succeeded for failed step, want waiting-input error")
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
