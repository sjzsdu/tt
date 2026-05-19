package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseGitHubRepo(t *testing.T) {
	tests := []struct {
		input string
		ok    bool
		owner string
		name  string
	}{
		{input: "https://github.com/sjzsdu/tt", ok: true, owner: "sjzsdu", name: "tt"},
		{input: "https://github.com/sjzsdu/tt.git", ok: true, owner: "sjzsdu", name: "tt"},
		{input: "https://github.com/sjzsdu/tt/tree/main/docs", ok: true, owner: "sjzsdu", name: "tt"},
		{input: "github.com/sjzsdu/tt", ok: true, owner: "sjzsdu", name: "tt"},
		{input: "git@github.com:sjzsdu/tt.git", ok: true, owner: "sjzsdu", name: "tt"},
		{input: "https://gitlab.com/sjzsdu/tt", ok: false},
		{input: "not-a-url", ok: false},
	}

	for _, tt := range tests {
		repo, ok := parseGitHubRepo(tt.input)
		if ok != tt.ok {
			t.Fatalf("parseGitHubRepo(%q) ok=%v, want %v", tt.input, ok, tt.ok)
		}
		if !tt.ok {
			continue
		}
		if repo.Owner != tt.owner || repo.Name != tt.name {
			t.Fatalf("parseGitHubRepo(%q)=%+v, want owner=%q name=%q", tt.input, repo, tt.owner, tt.name)
		}
		wantClone := "https://github.com/" + tt.owner + "/" + tt.name + ".git"
		if repo.CloneURL != wantClone {
			t.Fatalf("parseGitHubRepo(%q) cloneURL=%q, want %q", tt.input, repo.CloneURL, wantClone)
		}
	}
}

func TestFindMarkdownEntry(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs", "guide"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "index.md"), []byte("# Docs"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := findMarkdownEntry(root); got != "docs/index.md" {
		t.Fatalf("findMarkdownEntry()=%q, want docs/index.md", got)
	}

	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# Readme"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := findMarkdownEntry(root); got != "README.md" {
		t.Fatalf("findMarkdownEntry()=%q, want README.md", got)
	}
}

func TestEscapeViewPath(t *testing.T) {
	got := escapeViewPath("docs/My File.md")
	want := "docs/My%20File.md"
	if got != want {
		t.Fatalf("escapeViewPath()=%q, want %q", got, want)
	}
}
