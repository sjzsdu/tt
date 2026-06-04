package formulacmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sjzsdu/tt/internal/formula"
)

func TestRunFormulaPreflightCommandMissing(t *testing.T) {
	f := &formula.Formula{Preflight: &formula.PreflightSpec{Checks: []*formula.PreflightCheck{{Type: "command", Name: "missing-tool", Command: "tt-definitely-missing-tool", Message: "install missing tool"}}}}
	err := runFormulaPreflight(context.Background(), f, t.TempDir(), nil)
	if err == nil {
		t.Fatal("expected preflight error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "missing-tool") || !strings.Contains(msg, "install missing tool") {
		t.Fatalf("unexpected error: %s", msg)
	}
}

func TestRunFormulaPreflightExecAndPathPass(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &formula.Formula{Preflight: &formula.PreflightSpec{Checks: []*formula.PreflightCheck{
		{Type: "exec", Name: "shell", Command: "test -f marker.txt"},
		{Type: "path", Name: "marker", Path: "marker.txt"},
	}}}
	if err := runFormulaPreflight(context.Background(), f, dir, nil); err != nil {
		t.Fatalf("preflight should pass: %v", err)
	}
}

func TestRunFormulaPreflightEnv(t *testing.T) {
	t.Setenv("TT_PREFLIGHT_TEST_ENV", "ok")
	f := &formula.Formula{Preflight: &formula.PreflightSpec{Checks: []*formula.PreflightCheck{{Type: "env", Env: "TT_PREFLIGHT_TEST_ENV"}}}}
	if err := runFormulaPreflight(context.Background(), f, t.TempDir(), nil); err != nil {
		t.Fatalf("preflight should pass: %v", err)
	}
}

func TestRunFormulaPreflightSkipsFalseCondition(t *testing.T) {
	f := &formula.Formula{Vars: map[string]*formula.VarDef{"driver": &formula.VarDef{Default: stringPtr("jcode")}}, Preflight: &formula.PreflightSpec{Checks: []*formula.PreflightCheck{
		{Type: "command", Name: "codex", Command: "tt-definitely-missing-tool", Condition: "{{driver}} == codex"},
	}}}
	if err := runFormulaPreflight(context.Background(), f, t.TempDir(), nil); err != nil {
		t.Fatalf("preflight should skip false condition: %v", err)
	}
}

func TestRunFormulaPreflightRunsTrueConditionWithOverride(t *testing.T) {
	f := &formula.Formula{Vars: map[string]*formula.VarDef{"driver": &formula.VarDef{Default: stringPtr("jcode")}}, Preflight: &formula.PreflightSpec{Checks: []*formula.PreflightCheck{
		{Type: "command", Name: "codex", Command: "tt-definitely-missing-tool", Condition: "{{driver}} == codex"},
	}}}
	err := runFormulaPreflight(context.Background(), f, t.TempDir(), map[string]string{"driver": "codex"})
	if err == nil || !strings.Contains(err.Error(), "codex") {
		t.Fatalf("expected conditional preflight failure, got %v", err)
	}
}

func stringPtr(v string) *string { return &v }
