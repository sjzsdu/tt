package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

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
		"memory": false,
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

func TestTeamInitCopiesBuiltinDefinition(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TT_CONFIG", filepath.Join(root, "global-config.json"))
	t.Setenv("TT_PROJECT_CONFIG", filepath.Join(root, ".tt", "config.json"))

	var output bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&output)
	if err := runTeamInit(command, []string{"product-review"}); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root, ".tt", "teams", "product-review.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := teamruntime.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if definition.Team != "product-review" || len(definition.Agents) != 3 {
		t.Fatalf("definition = %+v", definition)
	}
	if !strings.Contains(output.String(), `Copied builtin team "product-review"`) {
		t.Fatalf("output = %q", output.String())
	}
}
