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
language = " 简体中文 "
default_model = " team-default "

[coordination]
facilitator = "lead"
finalizer = "lead"
review_waves = 1
max_concurrency = 2

[limits]
max_agent_turns = 6
max_review_turns_per_agent = 3
max_response_chars = 2000
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
	if definition.Limits.MaxReviewTurnsPerAgent != 3 ||
		definition.Limits.MaxResponseChars != 2000 {
		t.Fatalf("coordination/limits not parsed: %+v %+v", definition.Coordination, definition.Limits)
	}
	if definition.DefaultModel != "team-default" || definition.Agents[0].Model != "lead-model" {
		t.Fatalf("models were not normalized: default=%q lead=%q", definition.DefaultModel, definition.Agents[0].Model)
	}
	if definition.Language != "简体中文" {
		t.Fatalf("language = %q", definition.Language)
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

func TestDefinitionRejectsUnknownDeliveryOwner(t *testing.T) {
	definition := testDefinition(t)
	definition.Coordination.DeliveryOwner = "missing"
	if err := definition.Validate(); err == nil ||
		!strings.Contains(err.Error(), "coordination.delivery_owner") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestDefinitionNormalizesDeliveryCoordination(t *testing.T) {
	definition := testDefinition(t)
	definition.Coordination.InitialHandoff = " lead "
	definition.Coordination.DeliveryOwner = " expert "
	definition.Normalize()
	if definition.Coordination.InitialHandoff != "lead" ||
		definition.Coordination.DeliveryOwner != "expert" {
		t.Fatalf("delivery coordination was not normalized: %+v", definition.Coordination)
	}
	if err := definition.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestParseExternalAgentDefinition(t *testing.T) {
	definition, err := Parse([]byte(`
team = "external-review"
version = 1

[coordination]
facilitator = "lead"
finalizer = "lead"
review_waves = 0
max_concurrency = 2

[limits]
max_agent_turns = 3

[memory]
maintainer = "lead"
max_chars = 1000

[[agents]]
id = "lead"
model = "gpt-5"
[agents.external]
driver = " Codex "
mode = "normal"
timeout = "10m"
extra_args = ["--full-auto"]

[[agents]]
id = "reviewer"
agent = "assistant"
`))
	if err != nil {
		t.Fatal(err)
	}
	external := definition.Agents[0].External
	if external == nil || external.Driver != "codex" || external.Timeout != "10m" {
		t.Fatalf("unexpected external config: %+v", external)
	}
}

func TestExternalAgentDefinitionValidation(t *testing.T) {
	tests := []struct {
		name     string
		agent    string
		driver   string
		timeout  string
		wantText string
	}{
		{name: "conflicting embedded agent", agent: "assistant", driver: "codex", wantText: "cannot configure both"},
		{name: "unsupported driver", driver: "unknown", wantText: "is not supported"},
		{name: "invalid timeout", driver: "jcode", timeout: "soon", wantText: "positive duration"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			definition := testDefinition(t)
			definition.Agents[0].Agent = tt.agent
			definition.Agents[0].External = &ExternalAgentConfig{Driver: tt.driver, Timeout: tt.timeout}
			if err := definition.Validate(); err == nil || !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("Validate() error = %v, want %q", err, tt.wantText)
			}
		})
	}
}
