package schema

import (
	"testing"
)

func TestDecodeKindDiscriminatedFormula(t *testing.T) {
	data := []byte(`formula = "demo"
version = 1

[[steps]]
id = "research"
kind = "agent"
title = "Research"

[[steps]]
id = "test"
kind = "script"
title = "Run tests"
depends_on = ["research"]
command = ["go", "test", "./..."]
`)
	doc, err := Decode("demo.toml", data)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Name != "demo" {
		t.Fatalf("name = %q", doc.Name)
	}
	if len(doc.Steps) != 2 {
		t.Fatalf("steps = %d", len(doc.Steps))
	}
	if doc.Steps[1].Kind != "script" {
		t.Fatalf("kind = %q", doc.Steps[1].Kind)
	}
	if got := doc.Steps[1].DependsOn; len(got) != 1 || got[0] != "research" {
		t.Fatalf("depends = %#v", got)
	}
}

func TestDecodeStepIdempotent(t *testing.T) {
	data := []byte(`formula = "demo"
version = 1

[[steps]]
id = "x"
idempotent = true
`)
	doc, err := Decode("demo.toml", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Steps) != 1 {
		t.Fatalf("steps = %d", len(doc.Steps))
	}
	if !doc.Steps[0].Idempotent {
		t.Fatal("idempotent = false, want true")
	}
}

func TestDecodeFormulaOutputs(t *testing.T) {
	data := []byte(`formula = "demo"
version = 1

[outputs.report]
from = "final-report"
type = "markdown"
required = true
description = "Public report"

[[steps]]
id = "final-report"
kind = "agent"
`)
	doc, err := Decode("demo.toml", data)
	if err != nil {
		t.Fatal(err)
	}
	output, ok := doc.Outputs["report"]
	if !ok || output.From != "final-report" || output.Type != "markdown" || !output.Required || output.Description == "" {
		t.Fatalf("output = %+v, ok = %v", output, ok)
	}
}

func TestDecodeFormulaOutputRequiresSource(t *testing.T) {
	data := []byte(`formula = "demo"
version = 1

[outputs.report]
required = true
`)
	if _, err := Decode("demo.toml", data); err == nil {
		t.Fatal("expected output source validation error")
	}
}
