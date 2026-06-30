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
	runGit(t, repo, "checkout", "-b", "right", "main")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("right\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "commit", "-am", "right")
	merge := exec.Command("git", "merge", "left")
	merge.Dir = repo
	if err := merge.Run(); err == nil {
		t.Fatalf("expected merge conflict")
	}

	out, err := (formularuntime.ScriptCapability{DefaultTimeout: 5 * time.Second}).RunScript(context.Background(), steps.ScriptRequest{
		Command: listStep.Script.Command,
		Env:     map[string]string{"TT_REPO_ROOT": repo},
		Timeout: 5 * time.Second,
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
		if child.Meta().ID == "run-bead-coding.select-bead" {
			script, ok := child.(steps.ScriptStep)
			if !ok {
				t.Fatalf("run-bead-coding.select-bead = %T, want ScriptStep", child)
			}
			if strings.Contains(script.Env["EXCLUDE_BEAD_FILE"], "keep-coding-skip.txt") {
				sawExclude = true
			}
		}
	}
	if !sawAppend {
		t.Fatalf("keep-coding loop body should append partial beads to a skip list")
	}
	if !sawExclude {
		t.Fatalf("embedded bead-coding select-bead should receive the skip file")
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
	clarify := workflow.Graph.Nodes[ir.NodeID("clarify-situation")]
	if clarify == nil {
		t.Fatalf("missing clarify-situation node")
	}
	clarifyAgent, ok := clarify.Step.(steps.AgentStep)
	if !ok {
		t.Fatalf("clarify-situation step = %T, want AgentStep", clarify.Step)
	}
	if !clarifyAgent.DynamicForm {
		t.Fatalf("clarify-situation should use dynamic_form so forms are derived from the user's question")
	}
	if !slices.Contains(clarifyAgent.InputCtx, "discern-situation") {
		t.Fatalf("clarify-situation input_context = %v, want discern-situation", clarifyAgent.InputCtx)
	}
	for _, want := range []string{"只针对当前问题", "不能使用千篇一律", "radio / checkbox / select", "最多 1 个 textarea", "字段数量 3-5 个", "用户可见文案必须是产品文案"} {
		if !strings.Contains(clarifyAgent.Prompt, want) {
			t.Fatalf("clarify-situation prompt missing %q:\n%s", want, clarifyAgent.Prompt)
		}
	}
	for _, notWant := range []string{"career_direction", "postgraduate_readiness", "resources_and_pressure", "main_choice", "current_status"} {
		if strings.Contains(clarifyAgent.Prompt, notWant) {
			t.Fatalf("shan-yi-zhe should remain universal and not hard-code prompt field %q:\n%s", notWant, clarifyAgent.Prompt)
		}
	}

	merge := workflow.Graph.Nodes[ir.NodeID("merge-situation")]
	if merge == nil {
		t.Fatalf("missing merge-situation node")
	}
	if !slices.Contains(merge.Step.Meta().DependsOn, steps.ID("discern-situation")) || !slices.Contains(merge.Step.Meta().DependsOn, steps.ID("clarify-situation")) {
		t.Fatalf("merge-situation deps = %v, want discern-situation and clarify-situation", merge.Step.Meta().DependsOn)
	}
	mergeAgent, ok := merge.Step.(steps.AgentStep)
	if !ok {
		t.Fatalf("merge-situation step = %T, want AgentStep", merge.Step)
	}
	if !strings.Contains(mergeAgent.Prompt, "advice_confidence") || !strings.Contains(mergeAgent.Prompt, "conclusion_mode") {
		t.Fatalf("merge-situation prompt should require confidence and conclusion mode:\n%s", mergeAgent.Prompt)
	}

	for _, check := range []struct {
		id      ir.NodeID
		wantCtx string
	}{
		{"cast-frame", "merge-situation"},
		{"line-plan", "merge-situation"},
		{"change-reading", "merge-situation"},
		{"life-guidance", "merge-situation"},
	} {
		node := workflow.Graph.Nodes[check.id]
		if node == nil {
			t.Fatalf("missing %s node", check.id)
		}
		agent, ok := node.Step.(steps.AgentStep)
		if !ok {
			t.Fatalf("%s step = %T, want AgentStep", check.id, node.Step)
		}
		if !slices.Contains(agent.InputCtx, check.wantCtx) {
			t.Fatalf("%s input_context = %v, want %s", check.id, agent.InputCtx, check.wantCtx)
		}
	}
	cast := workflow.Graph.Nodes[ir.NodeID("cast-frame")]
	if !slices.Contains(cast.Step.Meta().DependsOn, steps.ID("merge-situation")) {
		t.Fatalf("cast-frame deps = %v, want merge-situation", cast.Step.Meta().DependsOn)
	}
	life := workflow.Graph.Nodes[ir.NodeID("life-guidance")].Step.(steps.AgentStep)
	for _, want := range []string{"已知事实与信息缺口", "结论置信度", "决策阈值", "行动方案", "不要编造用户未提供", "依据链", "本卦义理", "卦辞与彖传讲解", "易学概念", "复盘问题清单"} {
		if !strings.Contains(life.Prompt, want) {
			t.Fatalf("life-guidance prompt missing %q:\n%s", want, life.Prompt)
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
	for _, want := range []string{"init-scan-state.stdout", "load-final-state.stdout", "create-bug-beads"} {
		if !slices.Contains(finalAgent.InputCtx, want) {
			t.Fatalf("final-report context = %v, want %s", finalAgent.InputCtx, want)
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
