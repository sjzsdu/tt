package cmd

import (
	"testing"

	teamruntime "github.com/sjzsdu/tt/internal/team"
)

func TestTeamCommandRegistersMVPSubcommands(t *testing.T) {
	want := map[string]bool{
		"run":    false,
		"ask":    false,
		"resume": false,
		"show":   false,
		"open":   false,
		"list":   false,
		"init":   false,
	}
	for _, command := range teamCmd.Commands() {
		if _, ok := want[command.Name()]; ok {
			want[command.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("tt team %s is not registered", name)
		}
	}
}

func TestStarterTeamDefinitionParses(t *testing.T) {
	definition, err := teamruntime.Parse([]byte(starterTeamDefinition("product-review")))
	if err != nil {
		t.Fatal(err)
	}
	if definition.Team != "product-review" || len(definition.Agents) != 3 {
		t.Fatalf("definition = %+v", definition)
	}
	if !definition.MemoryEnabled() {
		t.Fatal("starter team memory should be enabled")
	}
}
