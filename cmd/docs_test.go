package cmd

import (
	"strings"
	"testing"
)

func TestNormalizeDocsRepoInput(t *testing.T) {
	tests := []struct {
		input    string
		wantURL  string
		wantName string
		wantOK   bool
	}{
		{input: "github.com/acme/project", wantURL: "https://github.com/acme/project.git", wantName: "project", wantOK: true},
		{input: "acme/project", wantURL: "https://github.com/acme/project.git", wantName: "project", wantOK: true},
		{input: "https://github.com/acme/project", wantURL: "https://github.com/acme/project.git", wantName: "project", wantOK: true},
		{input: "https://github.com/acme/project.git", wantURL: "https://github.com/acme/project.git", wantName: "project", wantOK: true},
		{input: "git@github.com:acme/project.git", wantURL: "https://github.com/acme/project.git", wantName: "project", wantOK: true},
		{input: "https://gitlab.com/acme/project.git", wantURL: "https://gitlab.com/acme/project.git", wantName: "project", wantOK: true},
		{input: "./internal", wantOK: false},
		{input: "", wantOK: false},
	}

	for _, tt := range tests {
		gotURL, gotName, gotOK := normalizeDocsRepoInput(tt.input)
		if gotOK != tt.wantOK {
			t.Fatalf("normalizeDocsRepoInput(%q) ok=%v, want %v", tt.input, gotOK, tt.wantOK)
		}
		if !tt.wantOK {
			continue
		}
		if gotURL != tt.wantURL {
			t.Fatalf("normalizeDocsRepoInput(%q) url=%q, want %q", tt.input, gotURL, tt.wantURL)
		}
		if gotName != tt.wantName {
			t.Fatalf("normalizeDocsRepoInput(%q) name=%q, want %q", tt.input, gotName, tt.wantName)
		}
	}
}

func TestBuildDocsAnalyzePromptDryRun(t *testing.T) {
	target := docsAnalyzeTarget{AnalysisDir: "/tmp/repo", DisplayName: "github.com/acme/project", RepoName: "project", Remote: true}
	prompt := buildDocsAnalyzePrompt(target, "/tmp/out", true)
	if prompt == "" {
		t.Fatal("expected non-empty prompt")
	}
	if !containsAll(prompt,
		"本次是 dry-run",
		"不要写任何文件",
		"建议新增/更新的文件",
		"分析来源：github.com/acme/project",
	) {
		t.Fatalf("unexpected dry-run prompt: %s", prompt)
	}
}

func containsAll(s string, values ...string) bool {
	for _, value := range values {
		if !strings.Contains(s, value) {
			return false
		}
	}
	return true
}
