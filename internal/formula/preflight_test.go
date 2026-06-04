package formula

import "testing"

func TestParserDecodesPreflightChecks(t *testing.T) {
	p := NewParser()
	f, err := p.ParseTOML([]byte(`formula = "demo"
version = 1
type = "workflow"

[preflight]

[[preflight.checks]]
type = "command"
name = "gh"
command = "gh"
message = "install gh"
condition = "{{driver}} == codex"

[[preflight.checks]]
type = "git"
name = "git-repo"
require_repo = true
require_remote = true

[[steps]]
id = "done"
title = "Done"
`))
	if err != nil {
		t.Fatal(err)
	}
	if f.Preflight == nil || len(f.Preflight.Checks) != 2 {
		t.Fatalf("expected 2 preflight checks, got %#v", f.Preflight)
	}
	if f.Preflight.Checks[0].Command != "gh" || f.Preflight.Checks[0].Condition != "{{driver}} == codex" || f.Preflight.Checks[1].RequireRemote != true {
		t.Fatalf("unexpected preflight checks: %#v", f.Preflight.Checks)
	}
}

func TestFormulaValidatePreflight(t *testing.T) {
	f := &Formula{Formula: "demo", Version: 1, Type: TypeWorkflow, Preflight: &PreflightSpec{Checks: []*PreflightCheck{{Type: "command"}}}}
	if err := f.Validate(); err == nil {
		t.Fatal("expected invalid command preflight")
	}
}
