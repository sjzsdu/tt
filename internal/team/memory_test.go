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

func TestMemoryProposalProvenanceRollbackAndCorruptedHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.md")
	proposal, err := ProposeMemory(path, "review", "review/thread-1", 2, []int64{7, 8, 9}, "lead", "# Team Memory\n\n- Keep decisions.", 5000)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Status != "pending" || !strings.Contains(proposal.Diff, "+- Keep decisions.") {
		t.Fatalf("proposal = %+v", proposal)
	}
	first, err := PromoteMemory(path, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if first.Version != 1 || len(first.SourceEvents) != 3 {
		t.Fatalf("first = %+v", first)
	}
	if _, err := UpgradeMemory(path, "review", "review/thread-1", 3, "# Team Memory\n\n- Keep decisions.\n- Prefer tests.", 5000); err != nil {
		t.Fatal(err)
	}
	restored, err := RollbackMemory(path, "review", "review/thread-1", 4, 1)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Version != 3 || restored.RestoredFrom != 1 || strings.Contains(restored.Content, "Prefer tests") {
		t.Fatalf("restored = %+v", restored)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "versions", "000002.md"), []byte("---\nbroken"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := LoadMemory(path, "review")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RollbackMemory(path, "review", "review/thread-1", 5, 2); err == nil {
		t.Fatal("expected corrupted history error")
	}
	after, err := LoadMemory(path, "review")
	if err != nil {
		t.Fatal(err)
	}
	if before.Version != after.Version || before.Content != after.Content {
		t.Fatalf("current memory changed after failed rollback: before=%+v after=%+v", before, after)
	}
}

func TestMemoryProposalRejectsUnsafeContentWithoutPromotion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.md")
	proposal, err := ProposeMemory(path, "review", "thread", 1, []int64{1}, "lead", "api_key = super-secret-value", 1000)
	if err == nil || proposal.Status != "rejected" {
		t.Fatalf("proposal/err = %+v %v", proposal, err)
	}
	current, loadErr := LoadMemory(path, "review")
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if current.Version != 0 {
		t.Fatalf("unsafe proposal replaced current memory: %+v", current)
	}
}

func TestStableMemoryContextExcludesPriorActiveTasks(t *testing.T) {
	content := `# Team Memory

## 用户偏好与沟通约束
- 必须使用简体中文。
- 用户当前关注点是清理 provider。

## 仓库与架构稳定事实
- provider 与 manifest 分离。

## 未解决的关键问题
- 下一轮继续实现过滤器。

## 本次线程形成的稳定结论
- 当前交付没有完成。`
	filtered := stableMemoryContext(content)
	for _, unwanted := range []string{"当前关注点", "下一轮继续", "当前交付"} {
		if strings.Contains(filtered, unwanted) {
			t.Fatalf("filtered memory retained %q:\n%s", unwanted, filtered)
		}
	}
	for _, wanted := range []string{"必须使用简体中文", "provider 与 manifest 分离"} {
		if !strings.Contains(filtered, wanted) {
			t.Fatalf("filtered memory lost %q:\n%s", wanted, filtered)
		}
	}
}
