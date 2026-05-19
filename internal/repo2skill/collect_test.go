package repo2skill

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectNPMRepo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name":"@acme/widgets","version":"1.2.3","description":"Composable widget helpers","exports":{".":"./src/index.ts"},"dependencies":{"zod":"^1.0.0"}}`)
	writeFile(t, dir, "README.md", "# Widgets\n\nComposable widget helpers for apps.\n\n```ts\nimport { createWidget } from '@acme/widgets'\ncreateWidget()\n```\n")
	writeFile(t, dir, "src/index.ts", "export function createWidget() { return {} }\nexport class Widget {}\n")
	writeFile(t, dir, "examples/basic.ts", "import { createWidget } from '../src'\n")

	p, cleanup, err := Collect(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if p.Name != "acme-widgets" {
		t.Fatalf("name = %q", p.Name)
	}
	if len(p.PackageFiles) != 1 || p.PackageFiles[0].Ecosystem != "npm" {
		t.Fatalf("package files: %#v", p.PackageFiles)
	}
	if len(p.InstallHints) == 0 || p.InstallHints[0] != "npm install @acme/widgets" {
		t.Fatalf("install hints: %#v", p.InstallHints)
	}
	if len(p.UsageSnippets) != 1 {
		t.Fatalf("snippets: %#v", p.UsageSnippets)
	}
	if !hasSymbol(p.PublicAPIs, "createWidget") || !hasSymbol(p.PublicAPIs, "Widget") {
		t.Fatalf("symbols: %#v", p.PublicAPIs)
	}
}

func TestRenderAllIncludesReferences(t *testing.T) {
	p := &RepoProfile{Name: "demo", Source: "local", Intent: "use-library", PublicAPIs: []APISymbol{{Name: "DoThing", Kind: "symbol", Source: "demo.go", Evidence: "demo.go exports DoThing"}}}
	m, err := HeuristicAnalyzer{}.Analyze(p)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := RenderAll(m, &b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"# demo repo skill", "Public API starting points", "# demo API reference", "# demo usage recipes", "# demo evidence map"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hasSymbol(items []APISymbol, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}
