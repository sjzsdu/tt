package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateEnumChoice(t *testing.T) {
	if err := validateEnumChoice("format", "site", docsVisualFormatChoices); err != nil {
		t.Fatalf("valid choice rejected: %v", err)
	}
	if err := validateEnumChoice("format", "pdf", docsVisualFormatChoices); err == nil {
		t.Fatal("invalid choice accepted")
	}
	if !strings.Contains(mockEnumErrText(), "must be one of") {
		t.Fatal("error message missing allowed values")
	}
}

// mockEnumErrText returns the error produced for an invalid format choice.
func mockEnumErrText() string {
	err := validateEnumChoice("format", "pdf", docsVisualFormatChoices)
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestBuildDocsVisualPromptDryRun(t *testing.T) {
	target := docsAnalyzeTarget{AnalysisDir: "/tmp/repo", DisplayName: "github.com/acme/project", RepoName: "project", Remote: true}
	prompt := buildDocsVisualPrompt(target, "/tmp/out", true)
	if prompt == "" {
		t.Fatal("expected non-empty prompt")
	}
	for _, want := range []string{
		"本次是 dry-run",
		"不要写任何文件",
		"理解问题",
		"diagram-design",
		"分析来源：github.com/acme/project",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("dry-run prompt missing %q: %s", want, prompt)
		}
	}
	// dry-run must not ask the agent to write the site
	if strings.Contains(prompt, "index.html；不要只给建议或计划。") {
		t.Fatalf("dry-run prompt should not request site writes: %s", prompt)
	}
}

func TestBuildDocsVisualPromptDefaults(t *testing.T) {
	target := docsAnalyzeTarget{AnalysisDir: "/tmp/repo", DisplayName: "/tmp/repo", RepoName: "repo"}
	prompt := buildDocsVisualPrompt(target, "/tmp/out", false)
	for _, want := range []string{
		"balanced（≤12 节点）",
		"mixed",
		"doc-wide 预设",
		"宁少而精",
		"index.html",
		"references/style-guide.md",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("default prompt missing %q: %s", want, prompt)
		}
	}
}

func TestBuildDocsVisualPromptOverrides(t *testing.T) {
	oldDetail, oldAudience, oldTypes, oldMax, oldBrand := docsVisualDetail, docsVisualAudience, docsVisualTypes, docsVisualMaxDiagrams, docsVisualBrand
	t.Cleanup(func() {
		docsVisualDetail, docsVisualAudience, docsVisualTypes, docsVisualMaxDiagrams, docsVisualBrand = oldDetail, oldAudience, oldTypes, oldMax, oldBrand
	})
	docsVisualDetail = "simplified"
	docsVisualAudience = "executive"
	docsVisualTypes = "architecture,sequence"
	docsVisualMaxDiagrams = 5
	docsVisualBrand = "https://example.com"

	target := docsAnalyzeTarget{AnalysisDir: "/tmp/repo", DisplayName: "/tmp/repo", RepoName: "repo"}
	prompt := buildDocsVisualPrompt(target, "/tmp/out", false)
	for _, want := range []string{
		"simplified（≤7 节点）",
		"executive",
		"architecture,sequence",
		"上限为 5 张",
		"品牌取样：https://example.com",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("override prompt missing %q: %s", want, prompt)
		}
	}
}

func TestEnsureDiagramDesignSkillProvisions(t *testing.T) {
	home := t.TempDir()
	if err := ensureDiagramDesignSkill(home); err != nil {
		t.Fatalf("provision failed: %v", err)
	}
	skillFile := filepath.Join(home, "skills", "diagram-design", "SKILL.md")
	info, err := os.Stat(skillFile)
	if err != nil {
		t.Fatalf("provisioned skill missing SKILL.md: %v", err)
	}
	if info.IsDir() {
		t.Fatal("SKILL.md copied as directory")
	}

	// Idempotent: second run must not error
	if err := ensureDiagramDesignSkill(home); err != nil {
		t.Fatalf("second provision failed: %v", err)
	}
}

func TestEnsureDiagramDesignSkillIdempotentContent(t *testing.T) {
	home := t.TempDir()
	if err := ensureDiagramDesignSkill(home); err != nil {
		t.Fatalf("provision failed: %v", err)
	}
	refFile := filepath.Join(home, "skills", "diagram-design", "references", "style-guide.md")
	if _, err := os.Stat(refFile); err != nil {
		t.Fatalf("provisioned skill missing references/style-guide.md: %v", err)
	}
}

func TestDocsVisualRegistered(t *testing.T) {
	found := false
	for _, sub := range docsCmd.Commands() {
		if sub.Name() == "visual" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("docs visual subcommand not registered")
	}
}
