package formulacmd

import (
	"strings"
	"testing"
)

func TestNewBuildsIndependentFormulaCommandTrees(t *testing.T) {
	first := New(Dependencies{})
	second := New(Dependencies{})

	if first == second {
		t.Fatal("New returned the same command instance")
	}
	if err := first.PersistentFlags().Set("dir", "/tmp/first-formulas"); err != nil {
		t.Fatalf("set first --dir: %v", err)
	}
	firstDir, err := first.PersistentFlags().GetString("dir")
	if err != nil {
		t.Fatalf("get first --dir: %v", err)
	}
	secondDir, err := second.PersistentFlags().GetString("dir")
	if err != nil {
		t.Fatalf("get second --dir: %v", err)
	}
	if firstDir != "/tmp/first-formulas" {
		t.Fatalf("first --dir = %q", firstDir)
	}
	if secondDir != "" {
		t.Fatalf("second --dir = %q, want independent default", secondDir)
	}
}

func TestFormulaRunCommandAcceptsFileWithoutName(t *testing.T) {
	cmd := New(Dependencies{})
	runCmd, _, err := cmd.Find([]string{"run", "--file", "run.toml", "--dry-run"})
	if err != nil {
		t.Fatalf("Find(run) error = %v", err)
	}
	if err := runCmd.Flags().Set("file", "run.toml"); err != nil {
		t.Fatalf("set --file: %v", err)
	}
	if err := runCmd.Args(runCmd, nil); err != nil {
		t.Fatalf("run Args with --file and no name error = %v", err)
	}
}

func TestFormulaRunCommandRequiresNameOrFile(t *testing.T) {
	cmd := New(Dependencies{})
	runCmd, _, err := cmd.Find([]string{"run"})
	if err != nil {
		t.Fatalf("Find(run) error = %v", err)
	}
	err = runCmd.Args(runCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "requires a formula name or --file") {
		t.Fatalf("run Args error = %v, want name/file requirement", err)
	}
}
