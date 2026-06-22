package formulacmd

import (
	"testing"
	"time"
)

func TestParseFormulaSchedulePlanEvery(t *testing.T) {
	plan, err := parseFormulaSchedulePlan("2m", "")
	if err != nil {
		t.Fatalf("parse every: %v", err)
	}
	if plan.Every != 2*time.Minute {
		t.Fatalf("Every = %s, want 2m", plan.Every)
	}
	if plan.Cron != "" {
		t.Fatalf("Cron = %q, want empty", plan.Cron)
	}
}

func TestParseFormulaSchedulePlanCron(t *testing.T) {
	plan, err := parseFormulaSchedulePlan("", "*/2 * * * *")
	if err != nil {
		t.Fatalf("parse cron: %v", err)
	}
	if plan.Cron != "*/2 * * * *" {
		t.Fatalf("Cron = %q", plan.Cron)
	}
}

func TestParseFormulaSchedulePlanRequiresOneSource(t *testing.T) {
	if _, err := parseFormulaSchedulePlan("", ""); err == nil {
		t.Fatal("expected missing schedule source error")
	}
	if _, err := parseFormulaSchedulePlan("1m", "*/2 * * * *"); err == nil {
		t.Fatal("expected mutually exclusive schedule source error")
	}
}

func TestParseFormulaSchedulePlanRejectsInvalidValues(t *testing.T) {
	if _, err := parseFormulaSchedulePlan("0s", ""); err == nil {
		t.Fatal("expected non-positive interval error")
	}
	if _, err := parseFormulaSchedulePlan("", "not cron"); err == nil {
		t.Fatal("expected invalid cron error")
	}
}

func TestNextFormulaScheduleTickEvery(t *testing.T) {
	now := time.Date(2026, 6, 22, 7, 30, 0, 0, time.UTC)
	next, err := nextFormulaScheduleTick(formulaSchedulePlan{Every: 90 * time.Second}, now)
	if err != nil {
		t.Fatalf("next tick: %v", err)
	}
	want := now.Add(90 * time.Second)
	if !next.Equal(want) {
		t.Fatalf("next = %s, want %s", next, want)
	}
}

func TestNextFormulaScheduleTickCron(t *testing.T) {
	now := time.Date(2026, 6, 22, 7, 30, 12, 0, time.UTC)
	next, err := nextFormulaScheduleTick(formulaSchedulePlan{Cron: "*/2 * * * *"}, now)
	if err != nil {
		t.Fatalf("next tick: %v", err)
	}
	want := time.Date(2026, 6, 22, 7, 32, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("next = %s, want %s", next, want)
	}
}

func TestReachedFormulaScheduleMaxRuns(t *testing.T) {
	if reachedFormulaScheduleMaxRuns(10, 0) {
		t.Fatal("maxRuns=0 should mean unlimited")
	}
	if !reachedFormulaScheduleMaxRuns(2, 2) {
		t.Fatal("expected max runs reached")
	}
}
