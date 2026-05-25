package cmd

import (
	"path/filepath"
	"testing"

	ttconfig "github.com/sjzsdu/tt/internal/ttconfig"
)

func TestFormulaSearchPathsIncludesProjectAndGlobalByDefault(t *testing.T) {
	projectRoot := t.TempDir()
	homeDir := t.TempDir()
	loaded := ttconfig.Loaded{
		Sources: ttconfig.Sources{ProjectPath: filepath.Join(projectRoot, ".tt", "config.json")},
		Merged: ttconfig.Config{
			Paths: ttconfig.PathsConfig{FormulaDir: ".tt/formulas"},
		},
	}

	paths := formulaSearchPaths(loaded, "", homeDir)
	want := []string{
		filepath.Join(projectRoot, ".tt", "formulas"),
		filepath.Join(homeDir, ".tt", "formulas"),
	}
	if len(paths) != len(want) {
		t.Fatalf("paths length = %d, want %d: %v", len(paths), len(want), paths)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("paths[%d] = %q, want %q; all paths: %v", i, paths[i], want[i], paths)
		}
	}
}

func TestFormulaSearchPathsExplicitDirOverridesDefaults(t *testing.T) {
	explicit := filepath.Join(t.TempDir(), "custom-formulas")
	paths := formulaSearchPaths(ttconfig.Loaded{}, explicit, t.TempDir())
	if len(paths) != 1 || paths[0] != explicit {
		t.Fatalf("paths = %v, want only %q", paths, explicit)
	}
}

func TestFormulaSearchPathsDeduplicatesGlobalWhenProjectIsHome(t *testing.T) {
	homeDir := t.TempDir()
	loaded := ttconfig.Loaded{
		Sources: ttconfig.Sources{ProjectPath: filepath.Join(homeDir, ".tt", "config.json")},
		Merged: ttconfig.Config{
			Paths: ttconfig.PathsConfig{FormulaDir: ".tt/formulas"},
		},
	}

	paths := formulaSearchPaths(loaded, "", homeDir)
	want := filepath.Join(homeDir, ".tt", "formulas")
	if len(paths) != 1 || paths[0] != want {
		t.Fatalf("paths = %v, want only %q", paths, want)
	}
}
