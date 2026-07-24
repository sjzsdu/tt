package team

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validTeamTOML = `team = "review"
title = "Review Team"
version = 1
default_model = " team-default "

[coordination]
facilitator = "lead"
finalizer = "lead"
review_waves = 1
max_concurrency = 2

[limits]
max_agent_turns = 6
max_wall_time = "5m"

[memory]
enabled = true
maintainer = "lead"
max_chars = 5000

[[agents]]
id = "lead"
role = "Lead"
agent = "assistant"
model = " lead-model "

[[agents]]
id = "expert"
role = "Expert"
agent = "coder"
`

func TestParseDefinition(t *testing.T) {
	definition, err := Parse([]byte(validTeamTOML))
	if err != nil {
		t.Fatal(err)
	}
	if definition.Team != "review" {
		t.Fatalf("team = %q", definition.Team)
	}
	if definition.DefinitionHash == "" {
		t.Fatal("definition hash is empty")
	}
	if !definition.MemoryEnabled() {
		t.Fatal("memory should be enabled")
	}
	if definition.Coordination.ReviewWaves != 1 {
		t.Fatalf("review waves = %d", definition.Coordination.ReviewWaves)
	}
	if definition.DefaultModel != "team-default" || definition.Agents[0].Model != "lead-model" {
		t.Fatalf("models were not normalized: default=%q lead=%q", definition.DefaultModel, definition.Agents[0].Model)
	}
}

func TestDefinitionDefaultsAndValidation(t *testing.T) {
	definition, err := Parse([]byte(`team = "defaults"

[[agents]]
id = "first"

[[agents]]
id = "second"
`))
	if err != nil {
		t.Fatal(err)
	}
	if definition.Coordination.Facilitator != "first" ||
		definition.Coordination.Finalizer != "first" ||
		definition.Memory.Maintainer != "first" {
		t.Fatalf("unexpected defaults: %+v %+v", definition.Coordination, definition.Memory)
	}
	if definition.Coordination.MaxConcurrency != 4 {
		t.Fatalf("max concurrency = %d", definition.Coordination.MaxConcurrency)
	}
}

func TestLoadAndListDefinitions(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "review")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, DefinitionFilename)
	if err := os.WriteFile(path, []byte(validTeamTOML), 0o644); err != nil {
		t.Fatal(err)
	}
	definition, err := Load("review", root)
	if err != nil {
		t.Fatal(err)
	}
	if definition.Source != path {
		t.Fatalf("source = %q, want %q", definition.Source, path)
	}
	flatPath := filepath.Join(root, "flat.toml")
	flatTOML := strings.Replace(validTeamTOML, `team = "review"`, `team = "flat"`, 1)
	if err := os.WriteFile(flatPath, []byte(flatTOML), 0o644); err != nil {
		t.Fatal(err)
	}
	records, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 ||
		records[0].Name != "flat" ||
		records[1].Name != "review" ||
		records[1].Agents != 2 {
		t.Fatalf("records = %+v", records)
	}
}

func TestLoadFlatDefinitionBeforeBuiltin(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "product-review.toml")
	content := strings.Replace(validTeamTOML, `team = "review"`, `team = "product-review"`, 1)
	content = strings.Replace(content, `title = "Review Team"`, `title = "Custom Product Review"`, 1)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	definition, err := Load("product-review", root)
	if err != nil {
		t.Fatal(err)
	}
	if definition.Source != path || definition.Title != "Custom Product Review" {
		t.Fatalf("definition = %+v", definition)
	}
}

func TestLoadBuiltinDefinitionFallback(t *testing.T) {
	definition, err := Load("product-review", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if definition.Source != "builtin:product-review" || len(definition.Agents) != 3 {
		t.Fatalf("definition = %+v", definition)
	}
}

func TestDefinitionRejectsUnknownCoordinator(t *testing.T) {
	_, err := Parse([]byte(`team = "bad"

[coordination]
facilitator = "missing"

[[agents]]
id = "one"

[[agents]]
id = "two"
`))
	if err == nil {
		t.Fatal("expected validation error")
	}
}
