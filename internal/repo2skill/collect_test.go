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
	writeFile(t, dir, "package.json", `{"name":"@acme/widgets","version":"1.2.3","description":"Composable widget helpers","exports":{".":{"types":"./dist/index.d.ts","import":"./src/index.ts"},"./extra":"./src/extra.ts"},"dependencies":{"zod":"^1.0.0"}}`)
	writeFile(t, dir, "README.md", "# Widgets\n\nComposable widget helpers for apps.\n\n```ts\nimport { createWidget } from '@acme/widgets'\ncreateWidget()\n```\n")
	writeFile(t, dir, "src/index.ts", "export function createWidget() { return {} }\nexport class Widget {}\nexport { ExtraWidget as Extra } from './extra'\n")
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
	if strings.Contains(strings.Join(p.PackageFiles[0].Exports, ","), "import") || !containsString(p.PackageFiles[0].Exports, "./extra") {
		t.Fatalf("exports should include public subpaths but not condition keys: %#v", p.PackageFiles[0].Exports)
	}
	if len(p.InstallHints) == 0 || p.InstallHints[0] != "npm install @acme/widgets" {
		t.Fatalf("install hints: %#v", p.InstallHints)
	}
	if len(p.UsageSnippets) != 1 {
		t.Fatalf("snippets: %#v", p.UsageSnippets)
	}
	if !hasSymbol(p.PublicAPIs, "createWidget") || !hasSymbol(p.PublicAPIs, "Widget") || !hasSymbol(p.PublicAPIs, "Extra") {
		t.Fatalf("symbols: %#v", p.PublicAPIs)
	}
}

func TestRenderAllOmitsEvidenceByDefault(t *testing.T) {
	p := &RepoProfile{Name: "demo", Source: "local", Intent: "use-library", PublicAPIs: []APISymbol{{Name: "DoThing", Kind: "symbol", Source: "demo.go", Evidence: "demo.go exports DoThing"}}}
	m, err := HeuristicAnalyzer{}.Analyze(p)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := RenderAll(m, &b, false); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"# demo repo skill", "Public API starting points", "# demo API reference", "# demo usage recipes", "## Avoid"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Evidence map") || strings.Contains(out, "# demo evidence map") {
		t.Fatalf("evidence map should be omitted by default:\n%s", out)
	}
}

func TestRenderAllIncludesEvidenceWhenRequested(t *testing.T) {
	p := &RepoProfile{Name: "demo", Source: "local", Intent: "use-library", PublicAPIs: []APISymbol{{Name: "DoThing", Kind: "symbol", Source: "demo.go", Evidence: "demo.go exports DoThing"}}}
	m, err := HeuristicAnalyzer{}.Analyze(p)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := RenderAll(m, &b, true); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"- [Evidence map](references/evidence.md)", "# demo evidence map"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestRenderMainSkillQuotesFrontmatter(t *testing.T) {
	p := &RepoProfile{Name: "scope/pkg:demo", Intent: "use-library"}
	m := &SkillModel{Profile: p, Purpose: "Demo"}
	var b bytes.Buffer
	if err := RenderMainSkill(m, &b, false); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "name: \"") || !strings.Contains(out, "description: \"") {
		t.Fatalf("frontmatter is not quoted:\n%s", out[:min(len(out), 160)])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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

func TestNormalizeSkillModelAddsValidationNotes(t *testing.T) {
	p := &RepoProfile{Name: "demo", PublicAPIs: []APISymbol{{Name: "Known", Source: "index.ts"}}, InstallHints: []string{"npm install demo"}}
	m := NormalizeSkillModel(&SkillModel{Profile: p, Install: []string{"npm install"}, PublicAPI: []APISymbol{{Name: "Imaginary", Source: "docs.md"}}, Recipes: []Recipe{{Title: "Use it", Description: "Do the thing."}}})
	if len(m.Install) != 1 || m.Install[0] != "npm install demo" {
		t.Fatalf("install fallback failed: %#v", m.Install)
	}
	if len(p.Warnings) == 0 || !strings.Contains(p.Warnings[0], "Imaginary") {
		t.Fatalf("expected validation warning, got %#v", p.Warnings)
	}
	if !strings.Contains(m.PublicAPI[0].Evidence, "agent-suggested") {
		t.Fatalf("expected agent-suggested evidence, got %#v", m.PublicAPI[0])
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func hasSymbol(items []APISymbol, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}
