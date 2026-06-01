package agentopt

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	repo2skillpkg "github.com/sjzsdu/tt/internal/repo2skill"
)

type fakeCollector struct{ profile *repo2skillpkg.RepoProfile }

func (f fakeCollector) Collect(string, repo2skillpkg.Options) (*repo2skillpkg.RepoProfile, func(), error) {
	return f.profile, func() {}, nil
}

type fakeProcessor struct{ resp string }

func (f fakeProcessor) ProcessDirect(string) (string, error) { return f.resp, nil }

func TestRenderMarkdownRoundTripShape(t *testing.T) {
	md := RenderMarkdown(toEmbeddedAgent(parseAgent(t)))
	if !strings.Contains(md, "id: demo-opt") || !strings.Contains(md, "You are optimized.") {
		t.Fatalf("unexpected markdown: %s", md)
	}
}

func TestOptimizeWritesOutput(t *testing.T) {
	tmp := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	_ = os.Chdir(tmp)
	_ = os.MkdirAll(".tt/agents", 0o755)
	_ = os.WriteFile(filepath.Join(".tt/agents", "base.md"), []byte("---\nid: base\nname: Base\nsoul: Test\n---\nBase prompt\n"), 0o644)

	o := Optimizer{Collector: fakeCollector{profile: &repo2skillpkg.RepoProfile{Name: "repo", Source: "./repo"}}, Processor: fakeProcessor{resp: "```json\n{\"id\":\"base-repo\",\"name\":\"Base Repo\",\"prompt\":" + jsonString(longPrompt("repo")) + ",\"skills\":[\"repo2skill\"]}\n```"}}
	result, err := o.Optimize(Options{Target: ".", BaseAgent: filepath.Join(".tt/agents", "base.md"), OutputPath: filepath.Join(tmp, "out")})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output == "" {
		t.Fatal("expected output path")
	}
	data, err := os.ReadFile(result.Output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "You are optimized for repo") {
		t.Fatalf("unexpected output: %s", string(data))
	}
}

func TestOptimizeUpdatesSourceAgentInPlaceByDefault(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "base.md")
	_ = os.WriteFile(path, []byte("---\nid: base\nname: Base\nsoul: Test\n---\nBase prompt\n"), 0o644)
	o := Optimizer{Collector: fakeCollector{profile: &repo2skillpkg.RepoProfile{Name: "repo", Source: "./repo"}}, Processor: fakeProcessor{resp: "{\"id\":\"base-repo\",\"name\":\"Base Repo\",\"prompt\":" + jsonString(longPrompt("repo")) + "}"}}
	result, err := o.Optimize(Options{Target: ".", BaseAgent: path})
	if err != nil {
		t.Fatal(err)
	}
	if !result.InPlace || result.Output != path || result.Generated.ID != "base" {
		t.Fatalf("expected in-place update preserving id, got %#v", result)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "Base prompt") || !strings.Contains(text, "Repository Knowledge Distillation") || !strings.Contains(text, "You are optimized for repo") {
		t.Fatalf("expected in-place optimization to preserve base prompt and append repo knowledge, got:\n%s", text)
	}
}

func TestMergeOptimizedPromptPreservesPreviousRepositories(t *testing.T) {
	existing := "Base prompt\n\n## Repository Knowledge Distillation\n\n### repo-a\nKnowledge for repo-a."
	merged := mergeOptimizedPrompt(existing, "Knowledge for repo-b.", &repo2skillpkg.RepoProfile{Name: "repo-b"})
	for _, want := range []string{"Base prompt", "repo-a", "Knowledge for repo-a", "repo-b", "Knowledge for repo-b"} {
		if !strings.Contains(merged, want) {
			t.Fatalf("merged prompt missing %q:\n%s", want, merged)
		}
	}
}

func TestOptimizeCopyCreatesSiblingAgent(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "base.md")
	_ = os.WriteFile(path, []byte("---\nid: base\nname: Base\nsoul: Test\n---\nBase prompt\n"), 0o644)
	o := Optimizer{Collector: fakeCollector{profile: &repo2skillpkg.RepoProfile{Name: "repo", Source: "./repo"}}, Processor: fakeProcessor{resp: "{\"id\":\"base-repo\",\"name\":\"Base Repo\",\"prompt\":" + jsonString(longPrompt("repo")) + "}"}}
	result, err := o.Optimize(Options{Target: ".", BaseAgent: path, Copy: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.InPlace || filepath.Base(result.Output) != "base-repo.md" {
		t.Fatalf("expected sibling copy, got output=%s inPlace=%v", result.Output, result.InPlace)
	}
}

func TestOptimizeRejectsWebsiteTarget(t *testing.T) {
	o := Optimizer{Collector: fakeCollector{profile: &repo2skillpkg.RepoProfile{Name: "repo"}}, Processor: fakeProcessor{resp: "{}"}}
	_, err := o.Optimize(Options{Target: "https://example.com/docs", BaseAgent: "base"})
	if err == nil || !strings.Contains(err.Error(), "Website") && !strings.Contains(err.Error(), "website") {
		t.Fatalf("expected website error, got %v", err)
	}
}

func TestOptimizeRejectsPromptOverBudget(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "base.md")
	_ = os.WriteFile(path, []byte("---\nid: base\nname: Base\nsoul: Test\n---\nBase prompt\n"), 0o644)
	o := Optimizer{Collector: fakeCollector{profile: &repo2skillpkg.RepoProfile{Name: "repo", Source: "./repo"}}, Processor: fakeProcessor{resp: "{\"id\":\"base-repo\",\"name\":\"Base Repo\",\"prompt\":" + jsonString(longPrompt("repo")) + "}"}}
	_, err := o.Optimize(Options{Target: ".", BaseAgent: path, MaxPromptChars: 220})
	if err == nil || !strings.Contains(err.Error(), "exceeds budget") {
		t.Fatalf("expected budget error, got %v", err)
	}
}

func parseAgent(t *testing.T) agentOutput {
	t.Helper()
	return agentOutput{ID: "demo-opt", Name: "Demo Opt", Soul: "Stay precise", Skills: []string{"repo2skill"}, Prompt: "You are optimized.", EnableResearchTools: true}
}

func toEmbeddedAgent(a agentOutput) pcwrap.EmbeddedAgent {
	return pcwrap.EmbeddedAgent{ID: a.ID, Name: a.Name, Soul: a.Soul, Skills: a.Skills, Prompt: a.Prompt, NoHistory: a.NoHistory, EnableResearchTools: a.EnableResearchTools}
}

func longPrompt(repo string) string {
	return "## Role\nYou are optimized for " + repo + ".\n## Target Repository Expertise\nUse README, docs, examples, tests, entrypoints, public APIs, package files, and usage snippets from " + repo + " as evidence. Prefer repository facts over guesses.\n## Common Tasks\nInvestigate issues, update code, review changes, and explain APIs for " + repo + ".\n## Validation Checklist\nRun available tests, type checks, examples, or documented validation commands before claiming success."
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
