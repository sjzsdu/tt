package formulacmd

import "testing"

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
