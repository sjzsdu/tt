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
	err := runFormulaPreflight(context.Background(), f, t.TempDir())
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
	if err := runFormulaPreflight(context.Background(), f, dir); err != nil {
		t.Fatalf("preflight should pass: %v", err)
	}
}

func TestRunFormulaPreflightEnv(t *testing.T) {
	t.Setenv("TT_PREFLIGHT_TEST_ENV", "ok")
	f := &formula.Formula{Preflight: &formula.PreflightSpec{Checks: []*formula.PreflightCheck{{Type: "env", Env: "TT_PREFLIGHT_TEST_ENV"}}}}
	if err := runFormulaPreflight(context.Background(), f, t.TempDir()); err != nil {
		t.Fatalf("preflight should pass: %v", err)
	}
}
