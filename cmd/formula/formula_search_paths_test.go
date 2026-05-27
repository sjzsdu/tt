package formulacmd

import (
	"os"
	"path/filepath"
	"strings"
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

func TestFormulaDefaultRunDirUsesCurrentWorkingDirByDefault(t *testing.T) {
	wd := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(wd); err != nil {
		t.Fatal(err)
	}
	wd, err = os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	loaded := ttconfig.Loaded{Sources: ttconfig.Sources{ProjectPath: filepath.Join(t.TempDir(), ".tt", "config.json")}}

	got := formulaDefaultRunDir(loaded)
	want := filepath.Join(wd, ".tt", "runs", "formula")
	if got != want {
		t.Fatalf("formulaDefaultRunDir = %q, want %q", got, want)
	}
}

func TestFormulaDefaultRunDirHonorsExplicitConfig(t *testing.T) {
	wd := t.TempDir()
	projectRoot := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(wd); err != nil {
		t.Fatal(err)
	}
	loaded := ttconfig.Loaded{
		Sources: ttconfig.Sources{ProjectPath: filepath.Join(projectRoot, ".tt", "config.json")},
		Merged:  ttconfig.Config{Paths: ttconfig.PathsConfig{FormulaRunDir: "custom-runs"}},
	}

	got := formulaDefaultRunDir(loaded)
	want := filepath.Join(projectRoot, "custom-runs")
	if got != want {
		t.Fatalf("formulaDefaultRunDir = %q, want %q", got, want)
	}
}

func TestSessionSlug(t *testing.T) {
	if got := sessionSlug("Flow360 Release!!"); got != "flow360-release" {
		t.Fatalf("sessionSlug = %q, want flow360-release", got)
	}
}

func TestUniqueFormulaRunSessionIncludesBaseFormulaAndUniqueSuffix(t *testing.T) {
	first := uniqueFormulaRunSession("cli:formula", "flow360-release")
	second := uniqueFormulaRunSession("cli:formula", "flow360-release")
	if first == second {
		t.Fatalf("uniqueFormulaRunSession returned duplicate %q", first)
	}
	prefix := "cli:formula:flow360-release:"
	if !strings.HasPrefix(first, prefix) || !strings.HasPrefix(second, prefix) {
		t.Fatalf("sessions = %q, %q; want prefix %q", first, second, prefix)
	}
}
