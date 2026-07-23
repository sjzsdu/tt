package team

import (
	"os"
	"path/filepath"
	"testing"
)

const validTeamTOML = `team = "review"
title = "Review Team"
version = 1

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
	records, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Name != "review" || records[0].Agents != 2 {
		t.Fatalf("records = %+v", records)
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
