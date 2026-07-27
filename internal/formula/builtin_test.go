package formula

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sjzsdu/tt/internal/agents"
	"github.com/sjzsdu/tt/internal/formula/ir"
	formularuntime "github.com/sjzsdu/tt/internal/formula/runtime"
	"github.com/sjzsdu/tt/internal/formula/spec"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

func TestBuiltinFormulasParseAndCompile(t *testing.T) {
	paths, err := builtinFormulaPathsInDir("builtin/formulas")
	if err != nil {
		t.Fatalf("builtinFormulaPathsInDir(formulas) error = %v", err)
	}
	if !slices.Contains(paths, "builtin/formulas/docs/fresh-topic-docs.toml") {
		t.Fatalf("builtin formula paths should include nested docs/fresh-topic-docs.toml; got %v", paths)
	}

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

func TestBuiltinFormulaAgentReferencesExist(t *testing.T) {
	agentList, err := agents.List()
	if err != nil {
		t.Fatalf("agents.List() error = %v", err)
	}
	known := map[string]struct{}{}
	for _, agent := range agentList {
		known[agent.ID] = struct{}{}
	}

	entries, err := BuiltinFormulas()
	if err != nil {
		t.Fatalf("BuiltinFormulas() error = %v", err)
	}
	atomics, err := BuiltinAtomicFormulas()
	if err != nil {
		t.Fatalf("BuiltinAtomicFormulas() error = %v", err)
	}
	entries = append(entries, atomics...)

	p := NewParser()
	for _, entry := range entries {
		f, err := p.LoadByName(entry.Name)
		if err != nil {
			t.Fatalf("LoadByName(%q) error = %v", entry.Name, err)
		}
		for _, step := range f.Steps {
			assertFormulaStepAgentsExist(t, entry.Name, step, known)
		}
	}
}

func assertFormulaStepAgentsExist(t *testing.T, formulaName string, step *spec.Step, known map[string]struct{}) {
	t.Helper()
	if step == nil {
		return
	}
	if step.Agent != nil && strings.TrimSpace(step.Agent.Name) != "" {
		if _, ok := known[strings.TrimSpace(step.Agent.Name)]; !ok {
			t.Fatalf("builtin formula %s step %s references unknown agent %q", formulaName, step.ID, step.Agent.Name)
		}
	}
	for _, child := range step.Children {
		assertFormulaStepAgentsExist(t, formulaName, child, known)
	}
}

func TestGitResolveConflictsPrepareConflictContextScript(t *testing.T) {
	p := NewParser()
	f, err := p.LoadByName("git-resolve-conflicts")
	if err != nil {
		t.Fatalf("LoadByName(git-resolve-conflicts) error = %v", err)
	}
	var listStep *spec.Step
	for _, step := range f.Steps {
		if step.ID == "prepare-conflict-context" {
			listStep = step
			break
		}
	}
	if listStep == nil || listStep.Script == nil {
		t.Fatalf("prepare-conflict-context script step not found")
	}

	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "file.txt")
	runGit(t, repo, "commit", "-m", "base")
	runGit(t, repo, "checkout", "-b", "left")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("left\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "commit", "-am", "left")
	runGit(t, repo, "checkout", "-b", "right", "HEAD~1")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("right\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "commit", "-am", "right")
	merge := exec.Command("git", "merge", "left")
	merge.Dir = repo
	if err := merge.Run(); err == nil {
		t.Fatalf("expected merge conflict")
	}

	const conflictContextTimeout = 20 * time.Second
	out, err := (formularuntime.ScriptCapability{DefaultTimeout: conflictContextTimeout}).RunScript(context.Background(), steps.ScriptRequest{
		Command: listStep.Script.Command,
		Env:     map[string]string{"TT_REPO_ROOT": repo},
		Timeout: conflictContextTimeout,
	})
	if err != nil {
		t.Fatalf("prepare-conflict-context script failed: %v; raw=%s", err, string(out.Raw))
	}
	var wrapper map[string]any
	if err := json.Unmarshal(out.Raw, &wrapper); err != nil {
		t.Fatalf("unmarshal script wrapper: %v; raw=%s", err, string(out.Raw))
	}
	stdout, ok := wrapper["stdout"].(string)
	if !ok || strings.TrimSpace(stdout) == "" {
		t.Fatalf("script stdout missing in %#v", wrapper)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("unmarshal script stdout: %v; stdout=%s stderr=%v", err, stdout, wrapper["stderr"])
	}
	gotRepoRoot, err := filepath.EvalSymlinks(asString(payload["repo_root"]))
	if err != nil {
		t.Fatalf("resolve payload repo_root: %v; payload=%#v", err, payload)
	}
	wantRepoRoot, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatalf("resolve temporary repo root: %v", err)
	}
	if gotRepoRoot != wantRepoRoot {
		t.Fatalf("repo_root = %q, want %q", gotRepoRoot, wantRepoRoot)
	}
	files := asSlice(payload["files"])
	if len(files) != 1 || asString(files[0]) != "file.txt" {
		t.Fatalf("files = %#v, want [file.txt]; payload=%#v", files, payload)
	}
	if len(asSlice(payload["items"])) != 1 {
		t.Fatalf("items = %#v, want one item", payload["items"])
	}
	item, ok := asSlice(payload["items"])[0].(map[string]any)
	if !ok {
		t.Fatalf("item = %#v, want object", asSlice(payload["items"])[0])
	}
	regions := asSlice(item["conflict_regions"])
	if len(regions) != 1 {
		t.Fatalf("conflict_regions = %#v, want one region", item["conflict_regions"])
	}
	region, ok := regions[0].(map[string]any)
	if !ok || asFloat(region["start_line"]) <= 0 || asFloat(region["end_line"]) <= 0 {
		t.Fatalf("conflict region missing line range: %#v", regions[0])
	}
}

func TestGitResolveConflictsVerifyDoesNotStageFiles(t *testing.T) {
	p := NewParser()
	f, err := p.LoadByName("git-resolve-conflicts")
	if err != nil {
		t.Fatalf("LoadByName(git-resolve-conflicts) error = %v", err)
	}
	var finalizeStep *spec.Step
	for _, step := range f.Steps {
		if step.ID == "verify-conflict-resolution" {
			finalizeStep = step
			break
		}
	}
	if finalizeStep == nil || finalizeStep.Script == nil {
		t.Fatalf("verify-conflict-resolution script step not found")
	}

	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "conflict.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "unrelated.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "conflict.txt", "unrelated.txt")
	runGit(t, repo, "commit", "-m", "base")
	if err := os.WriteFile(filepath.Join(repo, "conflict.txt"), []byte("resolved\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "unrelated.txt"), []byte("user work in progress\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	prepare := `{"files":["conflict.txt"],"operation":"merge","started_at":"2026-01-01T00:00:00Z","started_epoch":1767225600}`
	out, err := (formularuntime.ScriptCapability{DefaultTimeout: 5 * time.Second}).RunScript(context.Background(), steps.ScriptRequest{
		Command: finalizeStep.Script.Command,
		Env: map[string]string{
			"TT_REPO_ROOT": repo,
			"TT_PREPARE":   prepare,
		},
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("verify-conflict-resolution script failed: %v; raw=%s", err, string(out.Raw))
	}
	staged := runGitOutput(t, repo, "diff", "--cached", "--name-only")
	if strings.TrimSpace(staged) != "" {
		t.Fatalf("staged files = %q, want none; formula should not git add", staged)
	}
	unstaged := strings.Fields(runGitOutput(t, repo, "diff", "--name-only"))
	if strings.Join(unstaged, ",") != "conflict.txt,unrelated.txt" {
		t.Fatalf("unstaged files = %#v, want conflict.txt and unrelated.txt to remain unstaged", unstaged)
	}
	var wrapper map[string]any
	if err := json.Unmarshal(out.Raw, &wrapper); err != nil {
		t.Fatalf("unmarshal script wrapper: %v; raw=%s", err, string(out.Raw))
	}
	stdout := asString(wrapper["stdout"])
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("unmarshal finalize stdout: %v; stdout=%s", err, stdout)
	}
	reported := asSlice(payload["needs_user_add"])
	if len(reported) != 1 || asString(reported[0]) != "conflict.txt" {
		t.Fatalf("reported needs_user_add = %#v, want [conflict.txt]", reported)
	}
	if stagedFiles := asSlice(payload["staged_files"]); len(stagedFiles) != 0 {
		t.Fatalf("staged_files should be omitted/empty, got %#v", stagedFiles)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return string(out)
}

func TestBuiltinAtomicFormulasAreHiddenButLoadable(t *testing.T) {
	paths, err := builtinFormulaPathsInDir("builtin/atomics")
	if err != nil {
		t.Fatalf("builtinFormulaPathsInDir(atomics) error = %v", err)
	}
	if !slices.Contains(paths, "builtin/atomics/validation/run-validation.toml") {
		t.Fatalf("builtin atomic paths should include nested validation/run-validation.toml; got %v", paths)
	}

	regular, err := BuiltinFormulas()
	if err != nil {
		t.Fatalf("BuiltinFormulas() error = %v", err)
	}
	for _, entry := range regular {
		if slices.Contains([]string{"run-validation", "github-fetch-pr", "github-list-my-prs"}, entry.Name) {
			t.Fatalf("atomic formula %q should not appear in regular builtin list", entry.Name)
		}
	}

	atomics, err := BuiltinAtomicFormulas()
	if err != nil {
		t.Fatalf("BuiltinAtomicFormulas() error = %v", err)
	}
	want := []string{"run-validation", "github-fetch-pr", "github-list-my-prs"}
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
		"github-fetch-pr": {"pr_ref": "1"},
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
	t.Run("run-validation", func(t *testing.T) {
		repo := t.TempDir()
		out := runAtomicForTest(t, "run-validation", map[string]string{"repo_path": repo, "command": "printf ok"}, "validation")
		if out["requested"] != true || out["success"] != true || !strings.Contains(asString(out["stdout"]), "ok") {
			t.Fatalf("unexpected validation output: %#v", out)
		}
	})

	t.Run("run-validation auto detect override", func(t *testing.T) {
		repo := t.TempDir()
		out := runAtomicForTest(t, "run-validation", map[string]string{"repo_path": repo, "command": "printf auto-ok", "auto_detect": "true"}, "validation")
		if out["attempted"] != true || out["success"] != true || out["auto_detect"] != true || !strings.Contains(asString(out["stdout"]), "auto-ok") {
			t.Fatalf("unexpected auto validation output: %#v", out)
		}
	})

	t.Run("github atomics with fake gh", func(t *testing.T) {
		installFakeGH(t)

		pr := runAtomicForTest(t, "github-fetch-pr", map[string]string{"pr_ref": "1"}, "pr")
		if pr["ok"] != true || int(asFloat(pr["number"])) != 1 || asString(pr["title"]) != "Test PR" {
			t.Fatalf("unexpected pr output: %#v", pr)
		}

		if pr["ready"] != true || len(asSlice(pr["files"])) != 1 || len(asSlice(pr["changed_files"])) != 1 {
			t.Fatalf("expected full PR files/context fields: %#v", pr)
		}
		if !strings.Contains(asString(pr["patch"]), "diff --git") || asFloat(pr["patch_chars"]) == 0 {
			t.Fatalf("expected full PR patch fields: %#v", pr)
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

func TestComplexBuiltinReportsUseCuratedSummaryPayload(t *testing.T) {
	tests := []struct {
		formula string
		finalID string
	}{
		{formula: "feature", finalID: "final-report"},
		{formula: "bug-fix", finalID: "final-report"},
		{formula: "gongbu", finalID: "final-report"},
		{formula: "requirement-grooming", finalID: "final-grooming-report"},
		{formula: "github-pr-review", finalID: "final-report"},
		{formula: "github-pr-fix-comments", finalID: "final-report"},
		{formula: "github-pr-rebase-main", finalID: "final-report"},
	}

	for _, tt := range tests {
		t.Run(tt.formula, func(t *testing.T) {
			workflow, err := CompileWorkflowByName(context.Background(), tt.formula, nil, builtinCompileSmokeVars(tt.formula))
			if err != nil {
				t.Fatalf("CompileWorkflowByName(%q) error = %v", tt.formula, err)
			}
			reportData := workflow.Graph.Nodes[ir.NodeID("report-data")]
			if reportData == nil {
				t.Fatal("missing report-data step")
			}
			aggregate, ok := reportData.Step.(steps.AggregateStep)
			if !ok {
				t.Fatalf("report-data step = %T, want steps.AggregateStep", reportData.Step)
			}
			if len(aggregate.Fields) == 0 {
				t.Fatal("report-data must select at least one named field")
			}

			final := workflow.Graph.Nodes[ir.NodeID(tt.finalID)]
			if final == nil {
				t.Fatalf("missing %s step", tt.finalID)
			}
			reporter, ok := final.Step.(steps.AgentStep)
			if !ok {
				t.Fatalf("%s step = %T, want steps.AgentStep", tt.finalID, final.Step)
			}
			if got, want := reporter.InputCtx, []string{"report-data"}; !slices.Equal(got, want) {
				t.Fatalf("%s input_context = %v, want %v", tt.finalID, got, want)
			}
		})
	}
}

func TestKeepCodingWorkflowHasStableCycleNode(t *testing.T) {
	workflow, err := CompileWorkflowByName(context.Background(), "keep-coding", nil, map[string]string{"goal": "smoke goal"})
	if err != nil {
		t.Fatalf("CompileWorkflowByName(keep-coding) error = %v", err)
	}
	cycle := workflow.Graph.Nodes[ir.NodeID("cycle")]
	if cycle == nil {
		t.Fatalf("keep-coding must keep outer cycle as a stable runtime loop node")
	}
	loop, ok := cycle.Step.(steps.LoopStep)
	if !ok {
		t.Fatalf("cycle step = %T, want steps.LoopStep", cycle.Step)
	}
	if loop.MaxExpr != "{{max_cycles}}" {
		t.Fatalf("cycle MaxExpr = %q, want templated max_cycles", loop.MaxExpr)
	}
	if loop.Until != "cycle-note.selected == false" {
		t.Fatalf("cycle Until = %q, want cycle-note.selected == false", loop.Until)
	}
	final := workflow.Graph.Nodes[ir.NodeID("final-report")]
	if final == nil {
		t.Fatalf("missing final-report node")
	}
	if _, err := formularuntime.PlanTopological(workflow.Graph); err != nil {
		t.Fatalf("keep-coding graph should be topologically valid: %v", err)
	}
}

func TestCodingWorkflowIsNonInteractive(t *testing.T) {
	workflow, err := CompileWorkflowByName(context.Background(), "coding", nil, map[string]string{"requirement": "smoke requirement"})
	if err != nil {
		t.Fatalf("CompileWorkflowByName(coding) error = %v", err)
	}
	if workflow.Graph.Nodes[ir.NodeID("plan")] == nil || workflow.Graph.Nodes[ir.NodeID("implement")] == nil {
		t.Fatalf("coding should orchestrate plan and implement formulas")
	}
	plan, ok := workflow.Graph.Nodes[ir.NodeID("plan")].Step.(steps.FormulaCallStep)
	if !ok || plan.Formula != "coding-requirement" {
		t.Fatalf("plan = %T (%q), want coding-requirement FormulaCallStep", workflow.Graph.Nodes[ir.NodeID("plan")].Step, plan.Formula)
	}
	implement, ok := workflow.Graph.Nodes[ir.NodeID("implement")].Step.(steps.FormulaCallStep)
	if !ok || implement.Formula != "coding-implementation" {
		t.Fatalf("implement = %T (%q), want coding-implementation FormulaCallStep", workflow.Graph.Nodes[ir.NodeID("implement")].Step, implement.Formula)
	}
	requirement, err := CompileWorkflowByName(context.Background(), "coding-requirement", nil, map[string]string{"requirement": "smoke requirement"})
	if err != nil {
		t.Fatalf("CompileWorkflowByName(coding-requirement) error = %v", err)
	}
	agent := requirement.Graph.Nodes[ir.NodeID("plan-requirement")].Step.(steps.ExternalAgentStep)
	if !strings.Contains(agent.Prompt, "不要使用动态表单") {
		t.Fatalf("coding plan prompt should explicitly forbid dynamic forms:\n%s", agent.Prompt)
	}
	for _, output := range []string{"plan", "plan_review", "implementation", "report"} {
		if _, ok := workflow.Outputs[output]; !ok {
			t.Fatalf("coding missing public output %q", output)
		}
	}
}

func TestBeadCodingCallsCodingFormula(t *testing.T) {
	workflow, err := CompileWorkflowByName(context.Background(), "bead-coding", nil, map[string]string{"goal": "smoke goal"})
	if err != nil {
		t.Fatalf("CompileWorkflowByName(bead-coding) error = %v", err)
	}
	call, ok := workflow.Graph.Nodes[ir.NodeID("implement-bead")].Step.(steps.FormulaCallStep)
	if !ok || call.Formula != "coding" {
		t.Fatalf("implement-bead = %T (%q), want coding FormulaCallStep", workflow.Graph.Nodes[ir.NodeID("implement-bead")].Step, call.Formula)
	}
	runValidation := workflow.Graph.Nodes[ir.NodeID("run-validation")]
	if runValidation == nil {
		t.Fatalf("missing run-validation node")
	}
	if !slices.Contains(runValidation.Step.Meta().DependsOn, steps.ID("implement-bead")) {
		t.Fatalf("run-validation should wait for coding FormulaCall, deps=%v", runValidation.Step.Meta().DependsOn)
	}
	if got := workflow.Outputs["cycle_summary"].From; got != "cycle-summary" {
		t.Fatalf("cycle_summary output source = %q", got)
	}
	if _, err := formularuntime.PlanTopological(workflow.Graph); err != nil {
		t.Fatalf("bead-coding graph should be topologically valid: %v", err)
	}
}

func TestJiraWrappersUseFormulaCallPublicReport(t *testing.T) {
	tests := []struct {
		formula string
		vars    map[string]string
		stepID  ir.NodeID
		child   string
	}{
		{formula: "jira-feature", vars: map[string]string{"ticket_key": "PROJ-1"}, stepID: "run-feature", child: "feature"},
		{formula: "jira-bug-fix", vars: map[string]string{"ticket_key": "PROJ-1"}, stepID: "run-bug-fix", child: "bug-fix"},
	}
	for _, tt := range tests {
		t.Run(tt.formula, func(t *testing.T) {
			workflow, err := CompileWorkflowByName(context.Background(), tt.formula, nil, tt.vars)
			if err != nil {
				t.Fatalf("CompileWorkflowByName(%s) error = %v", tt.formula, err)
			}
			call, ok := workflow.Graph.Nodes[tt.stepID].Step.(steps.FormulaCallStep)
			if !ok || call.Formula != tt.child {
				t.Fatalf("%s = %T (%q), want %s FormulaCallStep", tt.stepID, workflow.Graph.Nodes[tt.stepID].Step, call.Formula, tt.child)
			}
			final := workflow.Graph.Nodes["final-report"].Step.(steps.AgentStep)
			if !slices.Contains(final.InputCtx, string(tt.stepID)+".report") {
				t.Fatalf("final-report input_context = %v, want child public report", final.InputCtx)
			}
		})
	}
}

func TestKeepCodingPropagatesFormulaInputs(t *testing.T) {
	workflow, err := CompileWorkflowByName(context.Background(), "keep-coding", nil, nil)
	if err != nil {
		t.Fatalf("CompileWorkflowByName(keep-coding) error = %v", err)
	}
	cycle := workflow.Graph.Nodes[ir.NodeID("cycle")]
	if cycle == nil {
		t.Fatalf("missing cycle node")
	}
	loop, ok := cycle.Step.(steps.LoopStep)
	if !ok {
		t.Fatalf("cycle step = %T, want LoopStep", cycle.Step)
	}
	var call steps.FormulaCallStep
	var found bool
	for _, child := range loop.Body {
		if child.Meta().ID != "run-bead-coding" {
			continue
		}
		var ok bool
		call, ok = child.(steps.FormulaCallStep)
		if !ok {
			t.Fatalf("run-bead-coding = %T, want FormulaCallStep", child)
		}
		found = true
	}
	if !found {
		t.Fatalf("missing bead-coding FormulaCallStep")
	}
	if call.Formula != "bead-coding" || call.With["external_driver"] != "{{external_driver}}" || call.With["external_model"] != "{{external_model}}" {
		t.Fatalf("bead-coding FormulaCall should bind external agent inputs, got formula=%q with=%v", call.Formula, call.With)
	}
}

func TestCodingRequirementLoopBodyIDsAlignWithUntil(t *testing.T) {
	workflow, err := CompileWorkflowByName(context.Background(), "coding-requirement", nil, map[string]string{"requirement": "smoke requirement"})
	if err != nil {
		t.Fatalf("CompileWorkflowByName(coding) error = %v", err)
	}
	node := workflow.Graph.Nodes[ir.NodeID("plan-review-loop")]
	if node == nil {
		t.Fatalf("missing plan review loop")
	}
	loop, ok := node.Step.(steps.LoopStep)
	if !ok {
		t.Fatalf("plan.plan-review-loop = %T, want LoopStep", node.Step)
	}
	if loop.Until != "plan-review.approved == true" {
		t.Fatalf("plan review loop until = %q, want plan-review.approved == true", loop.Until)
	}
	var sawReview bool
	for _, child := range loop.Body {
		if child.Meta().ID == "plan-review-loop.plan-review" {
			t.Fatalf("loop body id %q is double-prefixed with loop id", child.Meta().ID)
		}
		if child.Meta().ID == "plan-review" {
			sawReview = true
		}
	}
	if !sawReview {
		t.Fatalf("plan review loop body should contain plan-review")
	}
}

func TestKeepCodingDefaultMaxCyclesIsHighEnoughForBacklog(t *testing.T) {
	workflow, err := CompileWorkflowByName(context.Background(), "keep-coding", nil, nil)
	if err != nil {
		t.Fatalf("CompileWorkflowByName(keep-coding) error = %v", err)
	}
	got := workflow.Vars["max_cycles"].Default
	if got == nil || *got != "100" {
		value := "<nil>"
		if got != nil {
			value = *got
		}
		t.Fatalf("keep-coding max_cycles default = %q, want 100", value)
	}
}

func TestKeepCodingSkipsPartialBeadsWithinRun(t *testing.T) {
	workflow, err := CompileWorkflowByName(context.Background(), "keep-coding", nil, nil)
	if err != nil {
		t.Fatalf("CompileWorkflowByName(keep-coding) error = %v", err)
	}
	if workflow.Graph.Nodes[ir.NodeID("prepare-skip-list")] == nil {
		t.Fatalf("missing prepare-skip-list node")
	}
	cycle := workflow.Graph.Nodes[ir.NodeID("cycle")]
	if cycle == nil {
		t.Fatalf("missing cycle node")
	}
	loop, ok := cycle.Step.(steps.LoopStep)
	if !ok {
		t.Fatalf("cycle step = %T, want steps.LoopStep", cycle.Step)
	}
	var sawAppend bool
	var sawExclude bool
	for _, child := range loop.Body {
		if child.Meta().ID == "append-skip-list" {
			sawAppend = true
		}
		if child.Meta().ID == "run-bead-coding" {
			call, ok := child.(steps.FormulaCallStep)
			if !ok {
				t.Fatalf("run-bead-coding = %T, want FormulaCallStep", child)
			}
			if call.With["exclude_bead_file"] == "{{skip_file}}" {
				sawExclude = true
			}
		}
	}
	if !sawAppend {
		t.Fatalf("keep-coding loop body should append partial beads to a skip list")
	}
	if !sawExclude {
		t.Fatalf("bead-coding FormulaCall should receive the skip file")
	}
}

func TestKeepCodingPersistsCycleSummariesForFinalReport(t *testing.T) {
	workflow, err := CompileWorkflowByName(context.Background(), "keep-coding", nil, nil)
	if err != nil {
		t.Fatalf("CompileWorkflowByName(keep-coding) error = %v", err)
	}
	if got := workflow.Vars["cycle_log_file"].Default; got == nil || *got == "" {
		t.Fatalf("keep-coding should define cycle_log_file var, got %#v", got)
	}
	cycle := workflow.Graph.Nodes[ir.NodeID("cycle")]
	if cycle == nil {
		t.Fatalf("missing cycle node")
	}
	loop, ok := cycle.Step.(steps.LoopStep)
	if !ok {
		t.Fatalf("cycle step = %T, want steps.LoopStep", cycle.Step)
	}
	var sawAppendCycleLog bool
	var appendSkipDependsOnLog bool
	for _, child := range loop.Body {
		meta := child.Meta()
		if meta.ID == "append-cycle-log" {
			sawAppendCycleLog = true
			script, ok := child.(steps.ScriptStep)
			if !ok {
				t.Fatalf("append-cycle-log = %T, want ScriptStep", child)
			}
			if !strings.Contains(script.Env["CYCLE_LOG_FILE"], "cycle_log_file") {
				t.Fatalf("append-cycle-log should write to cycle_log_file, env=%#v", script.Env)
			}
			if !strings.Contains(script.Command[2], "path.open('a'") || !strings.Contains(script.Command[2], "commit_hash") {
				t.Fatalf("append-cycle-log should append machine summaries including commits, script:\n%s", script.Command[2])
			}
		}
		if meta.ID == "append-skip-list" {
			for _, dep := range meta.DependsOn {
				if dep == "append-cycle-log" {
					appendSkipDependsOnLog = true
				}
			}
		}
	}
	if !sawAppendCycleLog {
		t.Fatalf("keep-coding loop should append every cycle summary to a cycle log")
	}
	if !appendSkipDependsOnLog {
		t.Fatalf("append-skip-list should depend on append-cycle-log so every iteration is logged before loop output can be overwritten")
	}

	summary := workflow.Graph.Nodes[ir.NodeID("summarize-cycle-log")]
	if summary == nil {
		t.Fatalf("missing summarize-cycle-log node")
	}
	final := workflow.Graph.Nodes[ir.NodeID("final-report")]
	if final == nil {
		t.Fatalf("missing final-report node")
	}
	if !slices.Contains(final.Step.Meta().DependsOn, steps.ID("summarize-cycle-log")) {
		t.Fatalf("final-report should depend on summarize-cycle-log, deps=%v", final.Step.Meta().DependsOn)
	}
	finalAgent, ok := final.Step.(steps.AgentStep)
	if !ok {
		t.Fatalf("final-report = %T, want AgentStep", final.Step)
	}
	if !containsString(finalAgent.InputCtx, "summarize-cycle-log.stdout") {
		t.Fatalf("final-report should receive summarize-cycle-log.stdout, input_context=%v", finalAgent.InputCtx)
	}
}

func TestKeepCodingAppendSkipListScriptIsValidPython(t *testing.T) {
	workflow, err := CompileWorkflowByName(context.Background(), "keep-coding", nil, nil)
	if err != nil {
		t.Fatalf("CompileWorkflowByName(keep-coding) error = %v", err)
	}
	cycle := workflow.Graph.Nodes[ir.NodeID("cycle")]
	if cycle == nil {
		t.Fatalf("missing cycle node")
	}
	loop, ok := cycle.Step.(steps.LoopStep)
	if !ok {
		t.Fatalf("cycle step = %T, want steps.LoopStep", cycle.Step)
	}
	var appendScript *steps.ScriptStep
	for _, child := range loop.Body {
		if child.Meta().ID != "append-skip-list" {
			continue
		}
		script, ok := child.(steps.ScriptStep)
		if !ok {
			t.Fatalf("append-skip-list = %T, want ScriptStep", child)
		}
		appendScript = &script
		break
	}
	if appendScript == nil {
		t.Fatalf("missing append-skip-list script")
	}
	if len(appendScript.Command) < 3 || appendScript.Command[0] != "python3" || appendScript.Command[1] != "-c" {
		t.Fatalf("append-skip-list command = %#v, want python3 -c <script>", appendScript.Command)
	}
	scriptPath := filepath.Join(t.TempDir(), "append_skip_list.py")
	if err := os.WriteFile(scriptPath, []byte(appendScript.Command[2]), 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}
	cmd := exec.Command("python3", "-m", "py_compile", scriptPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("append-skip-list script should compile as Python: %v\n%s\nscript:\n%s", err, out, appendScript.Command[2])
	}
}

func TestKeepCodingDoesNotSkipProgressedPartialBeads(t *testing.T) {
	workflow, err := CompileWorkflowByName(context.Background(), "keep-coding", nil, nil)
	if err != nil {
		t.Fatalf("CompileWorkflowByName(keep-coding) error = %v", err)
	}
	cycle := workflow.Graph.Nodes[ir.NodeID("cycle")]
	if cycle == nil {
		t.Fatalf("missing cycle node")
	}
	loop, ok := cycle.Step.(steps.LoopStep)
	if !ok {
		t.Fatalf("cycle step = %T, want steps.LoopStep", cycle.Step)
	}
	var script string
	for _, child := range loop.Body {
		if child.Meta().ID != "append-skip-list" {
			continue
		}
		scriptStep, ok := child.(steps.ScriptStep)
		if !ok {
			t.Fatalf("append-skip-list = %T, want ScriptStep", child)
		}
		if len(scriptStep.Command) >= 3 {
			script = scriptStep.Command[2]
		}
	}
	if script == "" {
		t.Fatalf("missing append-skip-list script")
	}
	if !strings.Contains(script, "status == 'partial' and not committed and not closed") {
		t.Fatalf("append-skip-list should skip only non-progressed partial beads, script:\n%s", script)
	}
	if !strings.Contains(script, "status in {'blocked', 'failed'}") {
		t.Fatalf("append-skip-list should still skip blocked/failed beads, script:\n%s", script)
	}
}

func TestBeadCodingCanCloseWithoutNewCommit(t *testing.T) {
	workflow, err := CompileWorkflowByName(context.Background(), "bead-coding", nil, nil)
	if err != nil {
		t.Fatalf("CompileWorkflowByName(bead-coding) error = %v", err)
	}
	closeNode := workflow.Graph.Nodes[ir.NodeID("close-bead")]
	if closeNode == nil {
		t.Fatalf("missing close-bead node")
	}
	condition := closeNode.Step.Meta().Condition
	if strings.Contains(condition, "commit-changes.stdout.committed") {
		t.Fatalf("close-bead condition should not require a new commit: %q", condition)
	}
	if !strings.Contains(condition, "final-check.ready_to_close == true") {
		t.Fatalf("close-bead condition should honor final-check.ready_to_close: %q", condition)
	}
	finalCheck := workflow.Graph.Nodes[ir.NodeID("final-check")]
	if finalCheck == nil {
		t.Fatalf("missing final-check node")
	}
	agentStep, ok := finalCheck.Step.(steps.AgentStep)
	if !ok {
		t.Fatalf("final-check step = %T, want steps.AgentStep", finalCheck.Step)
	}
	for _, want := range []string{"文档、调研、需求收敛类 bead 也是有效需求", "不要强制要求人工复核", "已有交付物已经满足验收"} {
		if !strings.Contains(agentStep.Prompt, want) {
			t.Fatalf("final-check prompt should contain %q", want)
		}
	}
}

func TestBeadCodingCycleSummaryIncludesStatusForNoWork(t *testing.T) {
	workflow, err := CompileWorkflowByName(context.Background(), "bead-coding", nil, nil)
	if err != nil {
		t.Fatalf("CompileWorkflowByName(bead-coding) error = %v", err)
	}
	cycleSummary := workflow.Graph.Nodes[ir.NodeID("cycle-summary")]
	if cycleSummary == nil {
		t.Fatalf("missing cycle-summary node")
	}
	agentStep, ok := cycleSummary.Step.(steps.AgentStep)
	if !ok {
		t.Fatalf("cycle-summary step = %T, want steps.AgentStep", cycleSummary.Step)
	}
	if agentStep.Validation == nil || !slices.Contains(agentStep.Validation.Required, "status") {
		t.Fatalf("cycle-summary validation = %#v, want required status", agentStep.Validation)
	}
	if !strings.Contains(agentStep.Prompt, "status=\"no-work\"") {
		t.Fatalf("cycle-summary prompt should document no-work status")
	}
}

func TestRequirementGroomingLoadsExistingBeadsBeforeResearchAndCreate(t *testing.T) {
	workflow, err := CompileWorkflowByName(context.Background(), "requirement-grooming", nil, map[string]string{"requirement_prompt": "smoke"})
	if err != nil {
		t.Fatalf("CompileWorkflowByName(requirement-grooming) error = %v", err)
	}
	ensure := workflow.Graph.Nodes[ir.NodeID("ensure-beads-workspace")]
	if ensure == nil {
		t.Fatalf("missing ensure-beads-workspace node")
	}
	load := workflow.Graph.Nodes[ir.NodeID("load-existing-beads")]
	if load == nil {
		t.Fatalf("missing load-existing-beads node")
	}
	if !slices.Contains(load.Step.Meta().DependsOn, steps.ID("ensure-beads-workspace")) {
		t.Fatalf("load-existing-beads dependencies = %v, want ensure-beads-workspace", load.Step.Meta().DependsOn)
	}
	research := workflow.Graph.Nodes[ir.NodeID("project-research")]
	if research == nil {
		t.Fatalf("missing project-research node")
	}
	if !slices.Contains(research.Step.Meta().DependsOn, steps.ID("load-existing-beads")) {
		t.Fatalf("project-research dependencies = %v, want load-existing-beads", research.Step.Meta().DependsOn)
	}
	design := workflow.Graph.Nodes[ir.NodeID("design-bead-backlog")]
	if design == nil {
		t.Fatalf("missing design-bead-backlog node")
	}
	designAgent, ok := design.Step.(steps.AgentStep)
	if !ok {
		t.Fatalf("design-bead-backlog step = %T, want steps.AgentStep", design.Step)
	}
	if !slices.Contains(designAgent.InputCtx, "load-existing-beads.stdout") {
		t.Fatalf("design-bead-backlog context = %v, want load-existing-beads.stdout", designAgent.InputCtx)
	}
	create := workflow.Graph.Nodes[ir.NodeID("create-or-plan-beads")]
	if create == nil {
		t.Fatalf("missing create-or-plan-beads node")
	}
	if !slices.Contains(create.Step.Meta().DependsOn, steps.ID("design-bead-backlog")) {
		t.Fatalf("create-or-plan-beads dependencies = %v, want design-bead-backlog", create.Step.Meta().DependsOn)
	}
	createAgent, ok := create.Step.(steps.AgentStep)
	if !ok {
		t.Fatalf("create-or-plan-beads step = %T, want steps.AgentStep", create.Step)
	}
	if !slices.Contains(createAgent.InputCtx, "load-existing-beads.stdout") {
		t.Fatalf("create-or-plan-beads context = %v, want load-existing-beads.stdout", createAgent.InputCtx)
	}
}

func TestShanYiZheMergesClarificationBeforeDivination(t *testing.T) {
	workflow, err := CompileWorkflowByName(context.Background(), "shan-yi-zhe", nil, map[string]string{"question": "毕业后找工作还是考研？"})
	if err != nil {
		t.Fatalf("CompileWorkflowByName(shan-yi-zhe) error = %v", err)
	}
	for _, nodeID := range []ir.NodeID{
		"intent-gate",
		"personal-discern-shi-wei", "personal-clarify-shi-wei", "personal-settle-situation", "personal-cast-hexagram", "teach-personal-guidance",
		"history-analyze-scene", "history-cast-hexagram", "teach-history-case",
	} {
		if workflow.Graph.Nodes[nodeID] == nil {
			t.Fatalf("missing shan-yi-zhe node %s", nodeID)
		}
	}
	for _, removed := range []ir.NodeID{"discern-shi-wei", "settle-situation", "cast-hexagram", "teach-and-guide", "discern-situation", "merge-situation", "cast-frame", "line-plan", "interpret-lines", "change-reading", "life-guidance"} {
		if workflow.Graph.Nodes[removed] != nil {
			t.Fatalf("old shan-yi-zhe node %s should be removed", removed)
		}
	}

	gate := workflow.Graph.Nodes[ir.NodeID("intent-gate")]
	if len(gate.Step.Meta().DependsOn) != 0 {
		t.Fatalf("intent-gate deps = %v, want none", gate.Step.Meta().DependsOn)
	}
	gateAgent, ok := gate.Step.(steps.AgentStep)
	if !ok {
		t.Fatalf("intent-gate step = %T, want AgentStep", gate.Step)
	}
	for _, want := range []string{"第一道 Gate", "personal_advice", "historical_case_study", "primary_route", "现实处境", "历史场景学习易经", "只做意图识别和路由选择"} {
		if !strings.Contains(gateAgent.Prompt, want) {
			t.Fatalf("intent-gate prompt missing %q:\n%s", want, gateAgent.Prompt)
		}
	}

	personalDiscern := workflow.Graph.Nodes[ir.NodeID("personal-discern-shi-wei")]
	if !slices.Contains(personalDiscern.Step.Meta().DependsOn, steps.ID("intent-gate")) {
		t.Fatalf("personal-discern-shi-wei deps = %v, want intent-gate", personalDiscern.Step.Meta().DependsOn)
	}
	personalDiscernAgent, ok := personalDiscern.Step.(steps.AgentStep)
	if !ok {
		t.Fatalf("personal-discern-shi-wei step = %T, want AgentStep", personalDiscern.Step)
	}
	for _, want := range []string{"只服务 personal_advice", "needs_clarification", "判断信息是否足以进入取卦定像", "变易、不易、简易", "不要给最终建议"} {
		if !strings.Contains(personalDiscernAgent.Prompt, want) {
			t.Fatalf("personal-discern-shi-wei prompt missing %q:\n%s", want, personalDiscernAgent.Prompt)
		}
	}

	clarify := workflow.Graph.Nodes[ir.NodeID("personal-clarify-shi-wei")]
	clarifyAgent, ok := clarify.Step.(steps.AgentStep)
	if !ok {
		t.Fatalf("personal-clarify-shi-wei step = %T, want AgentStep", clarify.Step)
	}
	if !clarifyAgent.DynamicForm {
		t.Fatalf("personal-clarify-shi-wei should use dynamic_form")
	}
	for _, want := range []string{"动态澄清表单", "只针对当前现实问题", "时、位、选项、代价、资源、关系结构", "不要限制 textarea 数量", "未经用户确认的推测"} {
		if !strings.Contains(clarifyAgent.Prompt, want) {
			t.Fatalf("personal-clarify-shi-wei prompt missing %q:\n%s", want, clarifyAgent.Prompt)
		}
	}

	personalSettle := workflow.Graph.Nodes[ir.NodeID("personal-settle-situation")]
	if !slices.Contains(personalSettle.Step.Meta().DependsOn, steps.ID("personal-discern-shi-wei")) || !slices.Contains(personalSettle.Step.Meta().DependsOn, steps.ID("personal-clarify-shi-wei")) {
		t.Fatalf("personal-settle-situation deps = %v, want personal discern and clarify", personalSettle.Step.Meta().DependsOn)
	}
	personalSettleAgent, ok := personalSettle.Step.(steps.AgentStep)
	if !ok {
		t.Fatalf("personal-settle-situation step = %T, want AgentStep", personalSettle.Step)
	}
	for _, want := range []string{"现实辨事结果", "最终时位画像", "现实决策画像", "advice_confidence", "conclusion_mode", "ten_wings_frame"} {
		if !strings.Contains(personalSettleAgent.Prompt, want) {
			t.Fatalf("personal-settle-situation prompt missing %q:\n%s", want, personalSettleAgent.Prompt)
		}
	}

	personalCast := workflow.Graph.Nodes[ir.NodeID("personal-cast-hexagram")]
	if !slices.Contains(personalCast.Step.Meta().DependsOn, steps.ID("personal-settle-situation")) {
		t.Fatalf("personal-cast-hexagram deps = %v, want personal-settle-situation", personalCast.Step.Meta().DependsOn)
	}
	personalCastAgent, ok := personalCast.Step.(steps.AgentStep)
	if !ok {
		t.Fatalf("personal-cast-hexagram step = %T, want AgentStep", personalCast.Step)
	}
	for _, want := range []string{"现实决策画像", "此人此事此时此位", "本卦", "关键爻", "趋势，不是宿命", "advice_confidence"} {
		if !strings.Contains(personalCastAgent.Prompt, want) {
			t.Fatalf("personal-cast-hexagram prompt missing %q:\n%s", want, personalCastAgent.Prompt)
		}
	}

	personalFinal := workflow.Graph.Nodes[ir.NodeID("teach-personal-guidance")]
	if !slices.Contains(personalFinal.Step.Meta().DependsOn, steps.ID("personal-cast-hexagram")) {
		t.Fatalf("teach-personal-guidance deps = %v, want personal-cast-hexagram", personalFinal.Step.Meta().DependsOn)
	}
	personalFinalAgent, ok := personalFinal.Step.(steps.AgentStep)
	if !ok {
		t.Fatalf("teach-personal-guidance step = %T, want AgentStep", personalFinal.Step)
	}
	for _, want := range []string{"personal_advice", "开篇判断", "用户之时与位", "现实行动", "收束卦训", "不要机械套用固定 7 段标题", "不要编造用户未提供"} {
		if !strings.Contains(personalFinalAgent.Prompt, want) {
			t.Fatalf("teach-personal-guidance prompt missing %q:\n%s", want, personalFinalAgent.Prompt)
		}
	}
	for _, notWant := range []string{"一句话断局", "他怎么选择，结果如何", "今人学到什么", "输出结构必须只有 7 个板块"} {
		if strings.Contains(personalFinalAgent.Prompt, notWant) {
			t.Fatalf("teach-personal-guidance prompt should not contain history section %q", notWant)
		}
	}

	historyAnalyze := workflow.Graph.Nodes[ir.NodeID("history-analyze-scene")]
	if !slices.Contains(historyAnalyze.Step.Meta().DependsOn, steps.ID("intent-gate")) {
		t.Fatalf("history-analyze-scene deps = %v, want intent-gate", historyAnalyze.Step.Meta().DependsOn)
	}
	historyAnalyzeAgent, ok := historyAnalyze.Step.(steps.AgentStep)
	if !ok {
		t.Fatalf("history-analyze-scene step = %T, want AgentStep", historyAnalyze.Step)
	}
	for _, want := range []string{"只服务 historical_case_study", "历史人物当时的时位", "不给现代用户个人行动建议", "不要输出澄清表单", "通行历史常识", "易理类比"} {
		if !strings.Contains(historyAnalyzeAgent.Prompt, want) {
			t.Fatalf("history-analyze-scene prompt missing %q:\n%s", want, historyAnalyzeAgent.Prompt)
		}
	}

	historyCast := workflow.Graph.Nodes[ir.NodeID("history-cast-hexagram")]
	if !slices.Contains(historyCast.Step.Meta().DependsOn, steps.ID("history-analyze-scene")) {
		t.Fatalf("history-cast-hexagram deps = %v, want history-analyze-scene", historyCast.Step.Meta().DependsOn)
	}
	historyCastAgent, ok := historyCast.Step.(steps.AgentStep)
	if !ok {
		t.Fatalf("history-cast-hexagram step = %T, want AgentStep", historyCast.Step)
	}
	for _, want := range []string{"历史人物当时", "站在当时而非后见之明", "最贴切的一卦一爻", "实际选择与结果", "不要堆卦"} {
		if !strings.Contains(historyCastAgent.Prompt, want) {
			t.Fatalf("history-cast-hexagram prompt missing %q:\n%s", want, historyCastAgent.Prompt)
		}
	}

	historyFinal := workflow.Graph.Nodes[ir.NodeID("teach-history-case")]
	if !slices.Contains(historyFinal.Step.Meta().DependsOn, steps.ID("history-cast-hexagram")) {
		t.Fatalf("teach-history-case deps = %v, want history-cast-hexagram", historyFinal.Step.Meta().DependsOn)
	}
	historyFinalAgent, ok := historyFinal.Step.(steps.AgentStep)
	if !ok {
		t.Fatalf("teach-history-case step = %T, want AgentStep", historyFinal.Step)
	}
	for _, want := range []string{"historical_case_study", "历史场景学会", "历史前夕的断局", "当事人之时与位", "选择与结果", "今人可学的易学智慧", "不要机械套用固定 7 段标题", "义理复盘"} {
		if !strings.Contains(historyFinalAgent.Prompt, want) {
			t.Fatalf("teach-history-case prompt missing %q:\n%s", want, historyFinalAgent.Prompt)
		}
	}
	for _, notWant := range []string{"立刻做 / 近期做 / 长期取舍", "你站在哪一爻", "怎么做", "输出结构必须只有 7 个板块"} {
		if strings.Contains(historyFinalAgent.Prompt, notWant) {
			t.Fatalf("teach-history-case prompt should not contain personal section %q", notWant)
		}
	}
}

func TestWebBugHuntUsesAgentBrowserAndOptionalBeads(t *testing.T) {
	workflow, err := CompileWorkflowByName(context.Background(), "web-bug-hunt", nil, map[string]string{"url": "https://example.com"})
	if err != nil {
		t.Fatalf("CompileWorkflowByName(web-bug-hunt) error = %v", err)
	}
	for _, nodeID := range []ir.NodeID{"normalize-scope", "init-scan-state", "hunt-loop", "load-final-state", "final-report"} {
		if workflow.Graph.Nodes[nodeID] == nil {
			t.Fatalf("missing web-bug-hunt node %s", nodeID)
		}
	}
	if workflow.Graph.Nodes[ir.NodeID("verify-fixed-loop")] != nil {
		t.Fatalf("verify-fixed-loop should be filtered out by default fixed=false")
	}
	if workflow.Graph.Nodes[ir.NodeID("create-bug-beads")] != nil {
		t.Fatalf("create-bug-beads should be filtered out by default create_beads=false")
	}
	hunt := workflow.Graph.Nodes[ir.NodeID("hunt-loop")]
	huntLoop, ok := hunt.Step.(steps.LoopStep)
	if !ok {
		t.Fatalf("hunt-loop step = %T, want LoopStep", hunt.Step)
	}
	if huntLoop.Until != "persist-hunt-step.stdout.continue_scan == false" {
		t.Fatalf("hunt-loop until = %q", huntLoop.Until)
	}
	var sawExploreAgent, sawPersist bool
	for _, child := range huntLoop.Body {
		switch child.Meta().ID {
		case "explore-target":
			agentStep, ok := child.(steps.AgentStep)
			if !ok {
				t.Fatalf("explore-target step = %T, want AgentStep", child)
			}
			if agentStep.Agent != "agent-browser" {
				t.Fatalf("explore-target agent = %q, want agent-browser", agentStep.Agent)
			}
			sawExploreAgent = true
		case "persist-hunt-step":
			scriptStep, ok := child.(steps.ScriptStep)
			if !ok {
				t.Fatalf("persist-hunt-step step = %T, want ScriptStep", child)
			}
			if !strings.Contains(scriptStep.Command[2], "confirmed_bugs") || !strings.Contains(scriptStep.Command[2], "pending_pages") {
				t.Fatalf("persist-hunt-step should persist bugs and pending pages")
			}
			sawPersist = true
		}
	}
	if !sawExploreAgent || !sawPersist {
		t.Fatalf("hunt-loop missing explore-target=%v or persist-hunt-step=%v", sawExploreAgent, sawPersist)
	}
	final := workflow.Graph.Nodes[ir.NodeID("final-report")]
	finalAgent, ok := final.Step.(steps.AgentStep)
	if !ok {
		t.Fatalf("final-report step = %T, want AgentStep", final.Step)
	}
	for _, want := range []string{"load-final-state.stdout", "create-bug-beads"} {
		if !slices.Contains(finalAgent.InputCtx, want) {
			t.Fatalf("final-report context = %v, want %s", finalAgent.InputCtx, want)
		}
	}
	for _, unwanted := range []string{"init-scan-state.stdout", "hunt-loop", "verify-fixed-loop"} {
		if slices.Contains(finalAgent.InputCtx, unwanted) {
			t.Fatalf("final-report context = %v, should not include broad upstream context %s", finalAgent.InputCtx, unwanted)
		}
	}

	workflowWithBeads, err := CompileWorkflowByName(context.Background(), "web-bug-hunt", nil, map[string]string{"url": "https://example.com", "create_beads": "true"})
	if err != nil {
		t.Fatalf("CompileWorkflowByName(web-bug-hunt create_beads=true) error = %v", err)
	}
	create := workflowWithBeads.Graph.Nodes[ir.NodeID("create-bug-beads")]
	if create == nil {
		t.Fatalf("missing create-bug-beads when create_beads=true")
	}
	createAgent, ok := create.Step.(steps.AgentStep)
	if !ok {
		t.Fatalf("create-bug-beads step = %T, want AgentStep", create.Step)
	}
	if createAgent.Agent != "bead-manager" {
		t.Fatalf("create-bug-beads agent = %q, want bead-manager", createAgent.Agent)
	}

	fixedWorkflow, err := CompileWorkflowByName(context.Background(), "web-bug-hunt", nil, map[string]string{"url": "https://example.com", "fixed": "true"})
	if err != nil {
		t.Fatalf("CompileWorkflowByName(web-bug-hunt fixed=true) error = %v", err)
	}
	if fixedWorkflow.Graph.Nodes[ir.NodeID("hunt-loop")] != nil {
		t.Fatalf("hunt-loop should be filtered out when fixed=true")
	}
	verify := fixedWorkflow.Graph.Nodes[ir.NodeID("verify-fixed-loop")]
	if verify == nil {
		t.Fatalf("missing verify-fixed-loop when fixed=true")
	}
	verifyLoop, ok := verify.Step.(steps.LoopStep)
	if !ok {
		t.Fatalf("verify-fixed-loop step = %T, want LoopStep", verify.Step)
	}
	var sawVerifyAgent bool
	for _, child := range verifyLoop.Body {
		if child.Meta().ID != "verify-bug" {
			continue
		}
		agentStep, ok := child.(steps.AgentStep)
		if !ok {
			t.Fatalf("verify-bug step = %T, want AgentStep", child)
		}
		if agentStep.Agent != "agent-browser" {
			t.Fatalf("verify-bug agent = %q, want agent-browser", agentStep.Agent)
		}
		sawVerifyAgent = true
	}
	if !sawVerifyAgent {
		t.Fatalf("verify-fixed-loop missing verify-bug agent step")
	}
}

func TestWebFeatureTestUsesProjectDocStateAndAgentBrowser(t *testing.T) {
	workflow, err := CompileWorkflowByName(context.Background(), "web-feature-test", nil, map[string]string{
		"url":    "https://example.com",
		"prompt": "测试登录后创建项目和编辑项目名称",
	})
	if err != nil {
		t.Fatalf("CompileWorkflowByName(web-feature-test) error = %v", err)
	}
	for _, nodeID := range []ir.NodeID{"load-project-context", "normalize-scope", "init-test-state", "plan-test-cases", "persist-test-plan", "prepare-runnable-cases", "prepare-browser-session", "gate-runnable-cases", "test-loop", "validate-test-results", "merge-case-results", "load-final-state", "cleanup-browser-session", "final-report"} {
		if workflow.Graph.Nodes[nodeID] == nil {
			t.Fatalf("missing web-feature-test node %s", nodeID)
		}
	}
	loadCtx := workflow.Graph.Nodes[ir.NodeID("load-project-context")]
	loadScript, ok := loadCtx.Step.(steps.ScriptStep)
	if !ok {
		t.Fatalf("load-project-context step = %T, want ScriptStep", loadCtx.Step)
	}
	if !strings.Contains(loadScript.Command[2], "CONTEXT_DOC") || !strings.Contains(loadScript.Command[2], "web-feature-test.md") {
		t.Fatalf("load-project-context should load the configured .tt context doc")
	}
	initState := workflow.Graph.Nodes[ir.NodeID("init-test-state")]
	initScript, ok := initState.Step.(steps.ScriptStep)
	if !ok {
		t.Fatalf("init-test-state step = %T, want ScriptStep", initState.Step)
	}
	if !initScript.Meta().Idempotent {
		t.Fatalf("init-test-state idempotent = false, want true")
	}
	if strings.Contains(initScript.Command[2], "+ '\\n'") || !strings.Contains(initScript.Command[2], "chr(10)") {
		t.Fatalf("init-test-state should avoid TOML-sensitive Python newline literals")
	}
	for _, want := range []string{"artifacts_dir", "screenshots_dir", "test-state.json", "cases.jsonl", "runs.jsonl", "sanitize_browser_session_name", "[^a-zA-Z0-9_-]+"} {
		if !strings.Contains(initScript.Command[2], want) {
			t.Fatalf("init-test-state script missing %q", want)
		}
	}
	prepareBrowserNode := workflow.Graph.Nodes[ir.NodeID("prepare-browser-session")]
	prepareBrowser, ok := prepareBrowserNode.Step.(steps.ScriptStep)
	if !ok {
		t.Fatalf("prepare-browser-session step = %T, want ScriptStep", prepareBrowserNode.Step)
	}
	if !prepareBrowser.Meta().Idempotent {
		t.Fatalf("prepare-browser-session idempotent = false, want true")
	}
	prepareBrowserSource := prepareBrowser.Command[2] + prepareBrowser.Env["BROWSER_SESSION_NAME"] + prepareBrowser.Env["INIT_TEST_STATE"]
	for _, want := range []string{"agent-browser", "open", "get", "url", "eval", "WebGL", "screenshot", "safe_to_run_cases", "AGENT_BROWSER_SESSION_NAME", "AGENT_BROWSER_SOCKET_DIR", "{{init-test-state.stdout.browser_session_name}}"} {
		if !strings.Contains(prepareBrowserSource, want) {
			t.Fatalf("prepare-browser-session script missing %q", want)
		}
	}
	gateNode := workflow.Graph.Nodes[ir.NodeID("gate-runnable-cases")]
	gateStep, ok := gateNode.Step.(steps.ScriptStep)
	if !ok {
		t.Fatalf("gate-runnable-cases step = %T, want ScriptStep", gateNode.Step)
	}
	if !slices.Contains(gateStep.Meta().DependsOn, steps.ID("prepare-browser-session")) {
		t.Fatalf("gate-runnable-cases dependencies = %v, want prepare-browser-session", gateStep.Meta().DependsOn)
	}
	for _, want := range []string{"safe_to_run_cases", "skipped_results", "Browser preflight did not produce a safe shared session", "PREPARE_BROWSER_SESSION", "PREPARE_RUNNABLE_CASES", "prepare-browser-session.stdout"} {
		if !strings.Contains(gateStep.Command[2]+gateStep.Env["PREPARE_BROWSER_SESSION"], want) {
			t.Fatalf("gate-runnable-cases script missing %q", want)
		}
	}
	testLoopNode := workflow.Graph.Nodes[ir.NodeID("test-loop")]
	testLoop, ok := testLoopNode.Step.(steps.ScriptStep)
	if !ok {
		t.Fatalf("test-loop step = %T, want ScriptStep", testLoopNode.Step)
	}
	if !testLoop.Meta().Idempotent {
		t.Fatalf("test-loop idempotent = false, want true")
	}
	if !slices.Contains(testLoop.Meta().DependsOn, steps.ID("gate-runnable-cases")) {
		t.Fatalf("test-loop dependencies = %v, want gate-runnable-cases", testLoop.Meta().DependsOn)
	}
	testLoopSource := testLoop.Command[2] + testLoop.Env["GATE_RUNNABLE_CASES"] + testLoop.Env["PREPARE_BROWSER_SESSION"] + testLoop.Env["VISION_VERIFY"] + testLoop.Env["VISION_MODEL"]
	for _, want := range []string{"agent-browser", "operation_path", "coverage_results", "extract_selectors", "data-testid", "aria-label", "ensure_target_ready", "try_login", "context_doc", "click", "fill", "select", "screenshot", "bl", "vision", "describe", "visual_verification", "vision_verify", "vision_verify_mode", "should_verify_with_vision", "auto skipped pure DOM passed case", "VISION_VERIFY", "VISION_VERIFY_MODE", "VISION_MODEL", "AGENT_BROWSER_ENGINE", "AGENT_BROWSER_SESSION_NAME", "{{gate-runnable-cases.stdout}}", "{{prepare-browser-session.stdout}}"} {
		if !strings.Contains(testLoopSource, want) {
			t.Fatalf("test-loop script missing %q", want)
		}
	}
	if strings.Contains(testLoop.Command[2], "{{init-test-state.stdout.browser_session_name}}-{{case.id}}") {
		t.Fatalf("test-loop prompt should not use per-case browser sessions")
	}
	if testLoop.Validation == nil || !slices.Contains(testLoop.Validation.Required, "stdout") {
		t.Fatalf("test-loop validation = %#v, want required stdout", testLoop.Validation)
	}
	validateNode := workflow.Graph.Nodes[ir.NodeID("validate-test-results")]
	validateStep, ok := validateNode.Step.(steps.ScriptStep)
	if !ok {
		t.Fatalf("validate-test-results step = %T, want ScriptStep", validateNode.Step)
	}
	if !slices.Contains(validateStep.Meta().DependsOn, steps.ID("test-loop")) || !slices.Contains(validateStep.Meta().DependsOn, steps.ID("gate-runnable-cases")) {
		t.Fatalf("validate-test-results dependencies = %v, want test-loop and gate-runnable-cases", validateStep.Meta().DependsOn)
	}
	for _, want := range []string{"all unknown", "empty operation_path", "missing arguments for:", "did not complete the full ui interaction sequence", "sys.exit(1)", "GATE_RUNNABLE_CASES", "RESULTS_JSON", "test-loop.stdout"} {
		if !strings.Contains(validateStep.Command[2]+validateStep.Env["GATE_RUNNABLE_CASES"]+validateStep.Env["RESULTS_JSON"], want) {
			t.Fatalf("validate-test-results script missing %q", want)
		}
	}
	mergeNode := workflow.Graph.Nodes[ir.NodeID("merge-case-results")]
	mergeStep, ok := mergeNode.Step.(steps.ScriptStep)
	if !ok {
		t.Fatalf("merge-case-results step = %T, want ScriptStep", mergeNode.Step)
	}
	if !slices.Contains(mergeStep.Meta().DependsOn, steps.ID("validate-test-results")) {
		t.Fatalf("merge-case-results dependencies = %v, want validate-test-results", mergeStep.Meta().DependsOn)
	}
	if !mergeStep.Meta().Idempotent {
		t.Fatalf("merge-case-results idempotent = false, want true")
	}
	for _, want := range []string{"coverage_results", "coverage_points", "history", "chr(10)", ".agent-browser/tmp/screenshots", "shutil.copy2", "archived_screenshots", "SKIPPED_RESULTS_JSON", "skipped_results"} {
		if !strings.Contains(mergeStep.Command[2], want) {
			t.Fatalf("merge-case-results script missing %q", want)
		}
	}
	cleanupNode := workflow.Graph.Nodes[ir.NodeID("cleanup-browser-session")]
	cleanupStep, ok := cleanupNode.Step.(steps.ScriptStep)
	if !ok {
		t.Fatalf("cleanup-browser-session step = %T, want ScriptStep", cleanupNode.Step)
	}
	if !slices.Contains(cleanupStep.Meta().DependsOn, steps.ID("load-final-state")) {
		t.Fatalf("cleanup-browser-session dependencies = %v, want load-final-state", cleanupStep.Meta().DependsOn)
	}
	cleanupSource := cleanupStep.Command[2] + cleanupStep.Env["CLOSE_BROWSER_AFTER_RUN"] + cleanupStep.Env["BROWSER_SESSION_NAME"]
	for _, want := range []string{"agent-browser", "close", "--session", "CLOSE_BROWSER_AFTER_RUN", "close_browser_after_run", "browser_session_name"} {
		if !strings.Contains(cleanupSource, want) {
			t.Fatalf("cleanup-browser-session script missing %q", want)
		}
	}
	final := workflow.Graph.Nodes[ir.NodeID("final-report")]
	finalAgent, ok := final.Step.(steps.AgentStep)
	if !ok {
		t.Fatalf("final-report step = %T, want AgentStep", final.Step)
	}
	if !slices.Contains(final.Step.Meta().DependsOn, steps.ID("cleanup-browser-session")) {
		t.Fatalf("final-report dependencies = %v, want cleanup-browser-session", final.Step.Meta().DependsOn)
	}
	for _, want := range []string{"state_path", "artifacts_dir", "screenshots_dir", "only_failed=true", "覆盖矩阵", "visual_verification", "视觉模型", "cleanup-browser-session", "agent-browser close --session"} {
		if !strings.Contains(finalAgent.Prompt, want) {
			t.Fatalf("final-report prompt missing %q", want)
		}
	}
}

func TestSlideGenerateFormulaPlansLoopsAndAssemblesDeck(t *testing.T) {
	workflow, err := CompileWorkflowByName(context.Background(), "slide-generate", nil, map[string]string{
		"topic": "用易经解释创业失败后的破局",
	})
	if err != nil {
		t.Fatalf("CompileWorkflowByName(slide-generate) error = %v", err)
	}
	if got := workflow.Vars["output_dir"].Default; got == nil || *got != ".tt/slide-generate" {
		value := "<nil>"
		if got != nil {
			value = *got
		}
		t.Fatalf("slide-generate output_dir default = %q, want .tt/slide-generate", value)
	}
	for _, nodeID := range []ir.NodeID{"scope-analysis", "deck-plan", "write-slides", "assemble-deck", "final-report"} {
		if workflow.Graph.Nodes[nodeID] == nil {
			t.Fatalf("missing slide-generate node %s", nodeID)
		}
	}
	deckPlan := workflow.Graph.Nodes[ir.NodeID("deck-plan")]
	deckAgent, ok := deckPlan.Step.(steps.AgentStep)
	if !ok {
		t.Fatalf("deck-plan step = %T, want AgentStep", deckPlan.Step)
	}
	for _, want := range []string{"deck_name", "deck_title", "slides", "layout_hint", "visual_hint", "{{slide_count_hint}}"} {
		if !strings.Contains(deckAgent.Prompt, want) {
			t.Fatalf("deck-plan prompt missing %q", want)
		}
	}
	writeSlides := workflow.Graph.Nodes[ir.NodeID("write-slides")]
	loop, ok := writeSlides.Step.(steps.LoopStep)
	if !ok {
		t.Fatalf("write-slides step = %T, want LoopStep", writeSlides.Step)
	}
	if loop.ForEach != "deck-plan.slides" || loop.Var != "slide" {
		t.Fatalf("write-slides loop = for_each %q var %q, want deck-plan.slides/slide", loop.ForEach, loop.Var)
	}
	if len(loop.Body) != 1 {
		t.Fatalf("write-slides body len = %d, want 1", len(loop.Body))
	}
	draft, ok := loop.Body[0].(steps.AgentStep)
	if !ok {
		t.Fatalf("write-slides body[0] = %T, want AgentStep", loop.Body[0])
	}
	if draft.Agent != "slide-writer" {
		t.Fatalf("write-slides draft agent = %#v, want slide-writer", draft.Agent)
	}
	for _, want := range []string{"Reveal fragments", "layout_hint", "content", "speaker_notes", "talk", "视频/讲者口播", "观众可见文案", "本页旨在", "不是把", "我们需要让观众", "不要包含 `---`"} {
		if !strings.Contains(draft.Prompt, want) {
			t.Fatalf("write-slides draft prompt missing %q", want)
		}
	}
	if draft.Validation == nil || !slices.Contains(draft.Validation.Required, "speaker_notes") || !slices.Contains(draft.Validation.Required, "talk") {
		t.Fatalf("write-slides draft validation = %#v, want required speaker_notes and talk", draft.Validation)
	}
	assemble := workflow.Graph.Nodes[ir.NodeID("assemble-deck")]
	assembleScript, ok := assemble.Step.(steps.ScriptStep)
	if !ok {
		t.Fatalf("assemble-deck step = %T, want ScriptStep", assemble.Step)
	}
	for _, want := range []string{"deck_path", ".slide", "README.md", "talk_path", "talk_dir", "talk_files", "chr(10)", "slide_separator", "OUTPUT_DIR", ".tt/slide-generate", "WRITE_SLIDES", "DECK_PLAN"} {
		if !strings.Contains(assembleScript.Command[2]+assembleScript.Env["WRITE_SLIDES"]+assembleScript.Env["DECK_PLAN"], want) {
			t.Fatalf("assemble-deck script missing %q", want)
		}
	}
}

func TestSlideWriterAgentUsesJudgmentDrivenVisuals(t *testing.T) {
	agent, err := agents.Get("slide-writer")
	if err != nil {
		t.Fatalf("agents.Get(slide-writer) error = %v", err)
	}
	prompt := agent.Prompt
	for _, want := range []string{"用户提示是最高优先级", "最小必要改动", "现有 slide 源码为基底", "用户没有要求改的内容默认保留", "先判断是否需要视觉表达", "图表必须服务表达", "流程、层级、因果", "Mermaid/D2", "不要为了“显得丰富”", "而新增图示", "不要借机重写整页"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("slide-writer prompt missing %q", want)
		}
	}
	for _, banned := range []string{"默认不新增图表", "不要默认新增 Mermaid", "增强图文关系或简化 Mermaid"} {
		if strings.Contains(prompt, banned) {
			t.Fatalf("slide-writer prompt should not contain overcorrected diagram rule %q", banned)
		}
	}
}

func TestBuiltinFormulaAliasesAreCataloged(t *testing.T) {
	entries, err := BuiltinFormulas()
	if err != nil {
		t.Fatalf("BuiltinFormulas() error = %v", err)
	}
	want := map[string]string{
		"keep-coding":          "keep-coding",
		"bead-coding":          "bead-coding",
		"requirement-grooming": "requirement-grooming",
		"web-bug-hunt":         "web-bug-hunt",
		"web-feature-test":     "web-feature-test",
	}
	for _, entry := range entries {
		alias, ok := want[entry.Name]
		if !ok {
			continue
		}
		if !slices.Contains(entry.Aliases, alias) {
			t.Fatalf("builtin formula %s aliases = %v, want %q", entry.Name, entry.Aliases, alias)
		}
		delete(want, entry.Name)
	}
	if len(want) > 0 {
		t.Fatalf("missing builtin formulas with aliases: %v", want)
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
	case "web-bug-hunt":
		return map[string]string{"url": "https://example.com"}
	case "web-feature-test":
		return map[string]string{"url": "https://example.com", "prompt": "smoke feature test"}
	case "slide-generate":
		return map[string]string{"topic": "smoke slide deck"}
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
	for _, want := range []string{"intent-gate", "personal-discern-shi-wei", "personal-clarify-shi-wei", "personal-settle-situation", "personal-cast-hexagram", "teach-personal-guidance", "history-analyze-scene", "history-cast-hexagram", "teach-history-case"} {
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
