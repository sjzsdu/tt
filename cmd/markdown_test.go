package cmd

import (
	"net/http/httptest"
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

func TestHandleHTMLFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "architecture.html"), []byte("<h1>Architecture</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "diagram.css"), []byte("body { color: red; }"), 0o644); err != nil {
		t.Fatal(err)
	}

	previousRoot := mdRoot
	mdRoot = root
	t.Cleanup(func() { mdRoot = previousRoot })

	page := httptest.NewRecorder()
	handleHTMLFile(page, httptest.NewRequest("GET", "http://example.test/html/architecture.html", nil))
	if page.Code != 200 || page.Body.String() != "<h1>Architecture</h1>" {
		t.Fatalf("HTML response status=%d body=%q", page.Code, page.Body.String())
	}
	if got := page.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("HTML content type=%q, want text/html; charset=utf-8", got)
	}

	asset := httptest.NewRecorder()
	handleHTMLFile(asset, httptest.NewRequest("GET", "http://example.test/html/assets/diagram.css", nil))
	if asset.Code != 200 || asset.Body.String() != "body { color: red; }" {
		t.Fatalf("asset response status=%d body=%q", asset.Code, asset.Body.String())
	}

	escape := httptest.NewRecorder()
	handleHTMLFile(escape, httptest.NewRequest("GET", "http://example.test/html/../outside.html", nil))
	if escape.Code != 400 {
		t.Fatalf("path escape status=%d, want 400", escape.Code)
	}

	directory := httptest.NewRecorder()
	handleHTMLFile(directory, httptest.NewRequest("GET", "http://example.test/html/assets/", nil))
	if directory.Code != 404 {
		t.Fatalf("directory status=%d, want 404 without index.html", directory.Code)
	}
}
