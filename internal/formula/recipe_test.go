package formula

import "testing"

func TestToRecipeAddsRealStartAndEndBoundarySteps(t *testing.T) {
	f := &Formula{
		Formula: "demo",
		Steps: []*Step{
			{ID: "decide", Title: "Decide"},
			{ID: "improve", Title: "Improve", DependsOn: []string{"decide"}},
		},
	}
	recipe, err := toRecipe(f)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"demo.start", "demo.end"} {
		step := recipe.StepByID(id)
		if step == nil {
			t.Fatalf("missing boundary step %s", id)
		}
		if step.Execution != "noop" || step.Type != "boundary" {
			t.Fatalf("boundary step %+v should be noop boundary", step)
		}
	}
	wantDeps := map[[2]string]bool{
		{"demo.start", "demo"}:          true,
		{"demo.decide", "demo.start"}:   true,
		{"demo.improve", "demo.decide"}: true,
		{"demo.end", "demo.improve"}:    true,
	}
	for _, dep := range recipe.Deps {
		delete(wantDeps, [2]string{dep.StepID, dep.DependsOnID})
	}
	if len(wantDeps) != 0 {
		t.Fatalf("missing boundary deps: %+v\nall deps: %+v", wantDeps, recipe.Deps)
	}
}

func TestToRecipeRootOnlyConvergesStartToEnd(t *testing.T) {
	recipe, err := toRecipe(&Formula{Formula: "empty"})
	if err != nil {
		t.Fatal(err)
	}
	if recipe.StepByID("empty.start") == nil || recipe.StepByID("empty.end") == nil {
		t.Fatalf("missing start/end steps: %+v", recipe.Steps)
	}
	found := false
	for _, dep := range recipe.Deps {
		if dep.StepID == "empty.end" && dep.DependsOnID == "empty.start" && dep.Type == "blocks" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing start -> end dependency: %+v", recipe.Deps)
	}
}

func TestToRecipeReusesExplicitStartAndEndStepIDs(t *testing.T) {
	recipe, err := toRecipe(&Formula{Formula: "custom", Steps: []*Step{
		{ID: "start", Title: "Custom start", OutputKey: "started"},
		{ID: "work", Title: "Work", DependsOn: []string{"start"}},
		{ID: "end", Title: "Custom end", DependsOn: []string{"work"}, OutputKey: "summary"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, step := range recipe.Steps {
		counts[step.ID]++
	}
	if counts["custom.start"] != 1 || counts["custom.end"] != 1 {
		t.Fatalf("expected one start/end step, counts=%+v steps=%+v", counts, recipe.Steps)
	}
	if start := recipe.StepByID("custom.start"); start == nil || start.Execution == "noop" || start.OutputKey != "started" {
		t.Fatalf("explicit start step should be preserved and marked as boundary: %+v", start)
	}
	if end := recipe.StepByID("custom.end"); end == nil || end.Execution == "noop" || end.OutputKey != "summary" {
		t.Fatalf("explicit end step should be preserved and marked as boundary: %+v", end)
	}
}

func TestToRecipeDefaultsOutputKeyToStepID(t *testing.T) {
	recipe, err := toRecipe(&Formula{Formula: "demo", Steps: []*Step{
		{ID: "first", Title: "First"},
		{ID: "second", Title: "Second", OutputKey: "custom"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if first := recipe.StepByID("demo.first"); first == nil || first.OutputKey != "first" {
		t.Fatalf("default output key mismatch: %+v", first)
	}
	if second := recipe.StepByID("demo.second"); second == nil || second.OutputKey != "custom" {
		t.Fatalf("explicit output key should be preserved: %+v", second)
	}
}
