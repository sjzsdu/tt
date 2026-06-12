package formula

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sjzsdu/tt/internal/formula/ir"
	formularuntime "github.com/sjzsdu/tt/internal/formula/runtime"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

func TestBuiltinFormulasParseAndCompile(t *testing.T) {
	entries, err := BuiltinFormulas()
	if err != nil {
		t.Fatalf("BuiltinFormulas() error = %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected at least one builtin formula")
	}

	// Find fresh-topic-docs among the entries
	var entry *BuiltinEntry
	for i := range entries {
		if entries[i].Name == "fresh-topic-docs" {
			entry = &entries[i]
			break
		}
	}
	if entry == nil {
		t.Fatalf("fresh-topic-docs not found in builtin formulas: %v", entries)
	}
	if entry.Description == "" || entry.Title == "" || entry.Category == "" {
		t.Fatalf("builtin metadata incomplete: %+v", entry)
	}

	p := NewParser()
	f, err := p.LoadByName(entry.Name)
	if err != nil {
		t.Fatalf("LoadByName(%q) error = %v", entry.Name, err)
	}
	if f.Source != "builtin:"+entry.Name {
		t.Fatalf("Source = %q, want builtin:%s", f.Source, entry.Name)
	}
	if len(f.Steps) < 3 {
		t.Fatalf("expected multi-step document workflow, got %d steps", len(f.Steps))
	}
	stepIDs := make(map[string]bool)
	var idList []string
	for _, s := range f.Steps {
		stepIDs[s.ID] = true
		idList = append(idList, s.ID)
	}
	for _, want := range []string{"scope-analysis", "write-articles", "series-package"} {
		if !stepIDs[want] {
			t.Fatalf("expected step %q in formula, got steps: %v", want, idList)
		}
	}
	if err := f.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	workflow, err := CompileWorkflowByName(context.Background(), entry.Name, nil, map[string]string{"topic": "空间计算"})
	if err != nil {
		t.Fatalf("Compile(%q) error = %v", entry.Name, err)
	}
	if workflow.Name != entry.Name {
		t.Fatalf("workflow.Name = %q, want %q", workflow.Name, entry.Name)
	}
}

func TestBuiltinFormulaContent(t *testing.T) {
	data, ok, err := BuiltinFormulaContent("fresh-topic-docs")
	if err != nil {
		t.Fatalf("BuiltinFormulaContent error = %v", err)
	}
	if !ok || len(data) == 0 {
		t.Fatalf("expected fresh-topic-docs content")
	}
}

func TestBuiltinAtomicFormulasAreHiddenButLoadable(t *testing.T) {
	regular, err := BuiltinFormulas()
	if err != nil {
		t.Fatalf("BuiltinFormulas() error = %v", err)
	}
	for _, entry := range regular {
		if slices.Contains([]string{"git-run-validation", "github-fetch-pr", "github-list-my-prs", "github-fetch-pr-files", "github-fetch-pr-diff", "github-build-pr-context", "git-auto-detect-validation"}, entry.Name) {
			t.Fatalf("atomic formula %q should not appear in regular builtin list", entry.Name)
		}
	}

	atomics, err := BuiltinAtomicFormulas()
	if err != nil {
		t.Fatalf("BuiltinAtomicFormulas() error = %v", err)
	}
	want := []string{"git-run-validation", "github-fetch-pr", "github-list-my-prs", "github-fetch-pr-files", "github-fetch-pr-diff", "github-build-pr-context", "git-auto-detect-validation"}
	got := make(map[string]bool)
	for _, entry := range atomics {
		got[entry.Name] = true
		if entry.Category == "" || entry.Description == "" {
			t.Fatalf("atomic builtin metadata incomplete: %+v", entry)
		}
	}
	for _, name := range want {
		if !got[name] {
			t.Fatalf("missing atomic builtin %q; got %+v", name, atomics)
		}

		data, ok, err := BuiltinFormulaContent(name)
		if err != nil {
			t.Fatalf("BuiltinFormulaContent(%q) error = %v", name, err)
		}
		if !ok || len(data) == 0 {
			t.Fatalf("expected atomic builtin content for %q", name)
		}

		p := NewParser()
		f, err := p.LoadByName(name)
		if err != nil {
			t.Fatalf("LoadByName(%q) error = %v", name, err)
		}
		if f.Type != "atomic" {
			t.Fatalf("atomic formula %q type = %q, want atomic", name, f.Type)
		}
		if err := f.Validate(); err != nil {
			t.Fatalf("Validate(%q) error = %v", name, err)
		}
	}
}

func TestBuiltinAtomicFormulasCompile(t *testing.T) {
	varsByName := map[string]map[string]string{
		"github-fetch-pr":         {"pr_ref": "1"},
		"github-fetch-pr-files":   {"pr_ref": "1"},
		"github-fetch-pr-diff":    {"pr_ref": "1"},
		"github-build-pr-context": {"meta_json": `{"number":1,"title":"t"}`},
	}
	atomics, err := BuiltinAtomicFormulas()
	if err != nil {
		t.Fatalf("BuiltinAtomicFormulas() error = %v", err)
	}
	for _, entry := range atomics {
		workflow, err := CompileWorkflowByName(context.Background(), entry.Name, nil, varsByName[entry.Name])
		if err != nil {
			t.Fatalf("CompileWorkflowByName(%q) error = %v", entry.Name, err)
		}
		if workflow.Name != entry.Name {
			t.Fatalf("workflow.Name = %q, want %q", workflow.Name, entry.Name)
		}
	}
}

func TestBuiltinAtomicRuntimeContracts(t *testing.T) {
	t.Run("git-run-validation", func(t *testing.T) {
		repo := t.TempDir()
		out := runAtomicForTest(t, "git-run-validation", map[string]string{"repo_path": repo, "command": "printf ok"}, "validation")
		if out["requested"] != true || out["success"] != true || !strings.Contains(asString(out["stdout"]), "ok") {
			t.Fatalf("unexpected validation output: %#v", out)
		}
	})

	t.Run("git-auto-detect-validation", func(t *testing.T) {
		repo := t.TempDir()
		oldwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chdir(repo); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chdir(oldwd) })
		out := runAtomicForTest(t, "git-auto-detect-validation", map[string]string{"command": "printf auto-ok"}, "validation")
		if out["attempted"] != true || out["success"] != true || !strings.Contains(asString(out["stdout"]), "auto-ok") {
			t.Fatalf("unexpected auto validation output: %#v", out)
		}
	})

	t.Run("github atomics with fake gh", func(t *testing.T) {
		installFakeGH(t)

		pr := runAtomicForTest(t, "github-fetch-pr", map[string]string{"pr_ref": "1"}, "pr")
		if pr["ok"] != true || int(asFloat(pr["number"])) != 1 || asString(pr["title"]) != "Test PR" {
			t.Fatalf("unexpected pr output: %#v", pr)
		}

		files := runAtomicForTest(t, "github-fetch-pr-files", map[string]string{"pr_ref": "1"}, "files")
		if files["ok"] != true || len(asSlice(files["files"])) != 1 {
			t.Fatalf("unexpected files output: %#v", files)
		}

		diff := runAtomicForTest(t, "github-fetch-pr-diff", map[string]string{"pr_ref": "1"}, "diff")
		if diff["ok"] != true || !strings.Contains(asString(diff["patch"]), "diff --git") || asFloat(diff["patch_chars"]) == 0 {
			t.Fatalf("unexpected diff output: %#v", diff)
		}

		contextOut := runAtomicForTest(t, "github-build-pr-context", map[string]string{
			"meta_json":   mustJSON(t, pr),
			"files_json":  mustJSON(t, files),
			"patch_chars": "123",
		}, "context")
		if contextOut["ready"] != true || int(asFloat(contextOut["number"])) != 1 || len(asSlice(contextOut["changed_files"])) != 1 {
			t.Fatalf("unexpected context output: %#v", contextOut)
		}

		prs := runAtomicForTest(t, "github-list-my-prs", map[string]string{"author": "me", "limit": "5"}, "prs")
		if prs["ok"] != true || len(asSlice(prs["items"])) != 1 {
			t.Fatalf("unexpected prs output: %#v", prs)
		}
	})
}

func TestAllBuiltinFormulasCompile(t *testing.T) {
	entries, err := BuiltinFormulas()
	if err != nil {
		t.Fatalf("BuiltinFormulas() error = %v", err)
	}
	for _, entry := range entries {
		vars := builtinCompileSmokeVars(entry.Name)
		workflow, err := CompileWorkflowByName(context.Background(), entry.Name, nil, vars)
		if err != nil {
			t.Fatalf("CompileWorkflowByName(%q) error = %v", entry.Name, err)
		}
		if workflow.Name != entry.Name {
			t.Fatalf("workflow.Name = %q, want %q", workflow.Name, entry.Name)
		}
	}
}

func builtinCompileSmokeVars(name string) map[string]string {
	switch name {
	case "bug-fix":
		return map[string]string{"issue_summary": "smoke bug"}
	case "fresh-topic-docs":
		return map[string]string{"topic": "smoke topic"}
	case "feature":
		return map[string]string{"feature_request": "smoke feature"}
	case "github-pr-review", "github-pr-fix-comments", "github-pr-rebase-main":
		return map[string]string{"pr_ref": "1"}
	case "code-docs":
		return map[string]string{"repo": "."}
	case "gongbu":
		return map[string]string{"feature_request": "smoke feature"}
	case "jira-bug-fix":
		return map[string]string{"ticket_key": "SMOKE-1"}
	case "shan-yi-zhe":
		return map[string]string{"question": "smoke question"}
	default:
		return nil
	}
}

func runAtomicForTest(t *testing.T, name string, vars map[string]string, stepID string) map[string]any {
	t.Helper()
	workflow, err := CompileWorkflowByName(context.Background(), name, nil, vars)
	if err != nil {
		t.Fatalf("CompileWorkflowByName(%q) error = %v", name, err)
	}
	exec := formularuntime.NewExecutor(workflow, steps.Capabilities{Scripts: formularuntime.ScriptCapability{DefaultTimeout: 5 * time.Second}})
	exec.SeedWorkflowVars(workflow)
	exec.SeedVars(vars)
	result, err := exec.Run(context.Background())
	if err != nil {
		if result != nil {
			for id, node := range result.Nodes {
				if node != nil && node.Error != nil {
					t.Logf("node %s error: %+v raw=%s", id, node.Error, string(node.Output.Raw))
				}
			}
		}
		t.Fatalf("Run(%q) error = %v", name, err)
	}
	if result.Status != steps.StatusCompleted {
		t.Fatalf("Run(%q) status = %s", name, result.Status)
	}
	node := result.Nodes[workflow.Graph.Nodes[ir.NodeID(stepID)].ID]
	if node == nil {
		t.Fatalf("missing node result %q in %#v", stepID, result.Nodes)
	}
	var out map[string]any
	if err := json.Unmarshal(node.Output.Raw, &out); err != nil {
		t.Fatalf("unmarshal output for %s.%s: %v; raw=%s", name, stepID, err, string(node.Output.Raw))
	}
	return unwrapScriptStdout(t, out)
}

func unwrapScriptStdout(t *testing.T, out map[string]any) map[string]any {
	t.Helper()
	if parsed, ok := out["stdout"].(map[string]any); ok {
		return parsed
	}
	stdout, ok := out["stdout"].(string)
	if !ok {
		return out
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		return out
	}
	return parsed
}

func installFakeGH(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "gh")
	script := `#!/bin/sh
if [ "$1" = "auth" ]; then exit 0; fi
if [ "$1" = "pr" ] && [ "$2" = "view" ]; then
  case "$*" in
    *files*) printf '{"files":[{"path":"src/a.ts","additions":2,"deletions":1,"changeType":"MODIFIED"}]}' ;;
    *) printf '{"number":1,"title":"Test PR","body":"Body","author":{"login":"octo"},"url":"https://github.com/o/r/pull/1","state":"OPEN","isDraft":false,"baseRefName":"main","headRefName":"feat","headRefOid":"abc123","changedFiles":1,"additions":2,"deletions":1,"commits":[{"messageHeadline":"feat: test"}]}' ;;
  esac
  exit 0
fi
if [ "$1" = "pr" ] && [ "$2" = "diff" ]; then
  printf 'diff --git a/src/a.ts b/src/a.ts\n+new line\n'
  exit 0
fi
if [ "$1" = "pr" ] && [ "$2" = "list" ]; then
  printf '[{"number":1,"title":"Test PR","url":"https://github.com/o/r/pull/1","headRefName":"feat","baseRefName":"main","author":{"login":"me"}}]'
  exit 0
fi
printf 'unexpected gh args: %s\n' "$*" >&2
exit 1
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func asString(value any) string {
	s, _ := value.(string)
	return s
}

func asFloat(value any) float64 {
	f, _ := value.(float64)
	return f
}

func asSlice(value any) []any {
	s, _ := value.([]any)
	return s
}

func TestShanYiZheBuiltinFormula(t *testing.T) {
	p := NewParser()
	f, err := p.LoadByName("shan-yi-zhe")
	if err != nil {
		t.Fatalf("LoadByName(shan-yi-zhe) error = %v", err)
	}
	if f.Source != "builtin:shan-yi-zhe" {
		t.Fatalf("Source = %q, want builtin:shan-yi-zhe", f.Source)
	}
	stepIDs := make(map[string]bool)
	for _, s := range f.Steps {
		stepIDs[s.ID] = true
	}
	for _, want := range []string{"discern-situation", "cast-frame", "line-plan", "interpret-lines", "change-reading", "life-guidance"} {
		if !stepIDs[want] {
			t.Fatalf("expected step %q in shan-yi-zhe", want)
		}
	}
	if err := f.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if _, err := CompileWorkflowByName(context.Background(), "shan-yi-zhe", nil, map[string]string{"question": "我是否应该离开现在的工作去创业？"}); err != nil {
		t.Fatalf("Compile(shan-yi-zhe) error = %v", err)
	}
}
