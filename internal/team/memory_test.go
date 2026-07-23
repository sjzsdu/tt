package team

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpgradeMemoryMaintainsCurrentAndHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.md")
	first, err := UpgradeMemory(path, "review", "review/thread-1", 1, "# Team Memory\n\n- Prefer incremental delivery.", 5000)
	if err != nil {
		t.Fatal(err)
	}
	if first.Version != 1 {
		t.Fatalf("version = %d", first.Version)
	}
	second, err := UpgradeMemory(path, "review", "review/thread-1", 2, "# Team Memory\n\n- Prefer incremental delivery.\n- Keep public decisions.", 5000)
	if err != nil {
		t.Fatal(err)
	}
	if second.Version != 2 {
		t.Fatalf("version = %d", second.Version)
	}
	loaded, err := LoadMemory(path, "review")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != 2 || !strings.Contains(loaded.Content, "public decisions") {
		t.Fatalf("loaded = %+v", loaded)
	}
	for _, version := range []string{"000001.md", "000002.md"} {
		if _, err := os.Stat(filepath.Join(filepath.Dir(path), "versions", version)); err != nil {
			t.Fatalf("history %s: %v", version, err)
		}
	}
}

func TestUpgradeMemoryRejectsOversizedContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.md")
	_, err := UpgradeMemory(path, "review", "thread", 1, strings.Repeat("x", 1001), 1000)
	if err == nil {
		t.Fatal("expected max_chars error")
	}
}
