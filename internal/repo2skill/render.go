package repo2skill

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func Run(input string, opts Options, stdout io.Writer) error {
	profile, cleanup, err := Collect(input, opts)
	if err != nil {
		return err
	}
	defer cleanup()
	model, err := HeuristicAnalyzer{}.Analyze(profile)
	if err != nil {
		return err
	}
	if opts.DryRun {
		return RenderAll(model, stdout)
	}
	if opts.Markdown {
		tmp, err := os.MkdirTemp("", "repo2skill-skill-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		if err := WriteSkillFiles(model, tmp, stdout); err != nil {
			return err
		}
		cmd := exec.Command(os.Args[0], "markdown", tmp)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	dir, err := resolveSkillDir(opts.TargetDir, profile.Name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := WriteSkillFiles(model, dir, stdout); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Generated repo skill for %s in %s/\n", profile.Name, dir)
	return nil
}

func resolveSkillDir(targetDir, name string) (string, error) {
	if targetDir == "" {
		targetDir = "~/.agents/skills"
	}
	if targetDir == "~" || strings.HasPrefix(targetDir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if targetDir == "~" {
			return filepath.Join(home, "skills", name), nil
		}
		return filepath.Join(home, strings.TrimPrefix(targetDir, "~/"), name), nil
	}
	return filepath.Join(targetDir, name), nil
}

func WriteSkillFiles(m *SkillModel, dir string, log io.Writer) error {
	if err := os.MkdirAll(filepath.Join(dir, "references"), 0o755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return err
	}
	if err := RenderMainSkill(m, f); err != nil {
		f.Close()
		return err
	}
	f.Close()
	fmt.Fprintln(log, "  wrote: SKILL.md")
	refs := map[string]func(io.Writer, *SkillModel) error{"api.md": RenderAPIReference, "recipes.md": RenderRecipesReference, "evidence.md": RenderEvidenceReference}
	names := []string{}
	for n := range refs {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		rf, err := os.Create(filepath.Join(dir, "references", n))
		if err != nil {
			return err
		}
		if err := refs[n](rf, m); err != nil {
			rf.Close()
			return err
		}
		rf.Close()
		fmt.Fprintf(log, "  wrote: references/%s\n", n)
	}
	return nil
}
func RenderAll(m *SkillModel, out io.Writer) error {
	if err := RenderMainSkill(m, out); err != nil {
		return err
	}
	for _, fn := range []func(io.Writer, *SkillModel) error{RenderAPIReference, RenderRecipesReference, RenderEvidenceReference} {
		fmt.Fprint(out, "\n---\n\n")
		if err := fn(out, m); err != nil {
			return err
		}
	}
	return nil
}

func RenderMainSkill(m *SkillModel, out io.Writer) error {
	p := m.Profile
	fmt.Fprintf(out, "---\nname: %s\ndescription: Use the %s repository/library correctly in development. Generated from repo evidence for intent: %s.\n---\n\n", skillName(p.Name), p.Name, p.Intent)
	fmt.Fprintf(out, "# %s repo skill\n\n", p.Name)
	fmt.Fprint(out, "Use this skill when you need to use this repository/library as a dependency or implementation reference in a coding task. Prefer documented public APIs, README guidance, examples, and package exports over internal implementation details.\n\n")
	fmt.Fprint(out, "## What it is for\n\n")
	fmt.Fprintf(out, "%s\n\n", m.Purpose)
	if len(m.Install) > 0 {
		fmt.Fprint(out, "## Installation\n\n")
		for _, x := range m.Install {
			fmt.Fprintf(out, "```bash\n%s\n```\n", x)
		}
		fmt.Fprintln(out)
	}
	fmt.Fprint(out, "## Development guidance\n\n")
	for _, x := range m.BestPractices {
		fmt.Fprintf(out, "- %s\n", x)
	}
	fmt.Fprintln(out)
	if len(m.PublicAPI) > 0 {
		fmt.Fprint(out, "## Public API starting points\n\n| Symbol | Source |\n| --- | --- |\n")
		for i, a := range m.PublicAPI {
			if i >= 20 {
				break
			}
			fmt.Fprintf(out, "| `%s` | `%s` |\n", a.Name, a.Source)
		}
		fmt.Fprint(out, "\nSee [API reference](references/api.md) for more.\n")
	}
	if len(m.Recipes) > 0 {
		fmt.Fprint(out, "\n## Common recipes\n\n")
		for _, r := range m.Recipes {
			fmt.Fprintf(out, "- [%s](references/recipes.md#%s) - %s\n", r.Title, anchor(r.Title), r.Description)
		}
	}
	fmt.Fprint(out, "\n## Agent operating rules\n\n- Treat repository docs, examples, and package entrypoints as source of truth.\n- Do not recommend internal/private APIs unless the docs explicitly say they are public.\n- When using generated examples, run the target project's type checker or tests.\n- If a needed API is missing from this skill, inspect upstream docs before coding.\n\n## References\n\n- [API reference](references/api.md)\n- [Recipes](references/recipes.md)\n- [Evidence map](references/evidence.md)\n")
	return nil
}
func RenderAPIReference(out io.Writer, m *SkillModel) error {
	fmt.Fprintf(out, "# %s API reference\n\n", m.Profile.Name)
	if len(m.PublicAPI) == 0 {
		fmt.Fprint(out, "No public symbols were detected automatically. Use package metadata, README, and examples as source of truth.\n\n")
		return nil
	}
	fmt.Fprintln(out, "| Symbol | Kind | Source | Evidence |\n| --- | --- | --- | --- |")
	for _, a := range m.PublicAPI {
		fmt.Fprintf(out, "| `%s` | %s | `%s` | %s |\n", a.Name, a.Kind, a.Source, table(a.Evidence))
	}
	return nil
}
func RenderRecipesReference(out io.Writer, m *SkillModel) error {
	fmt.Fprintf(out, "# %s usage recipes\n\n", m.Profile.Name)
	for _, r := range m.Recipes {
		fmt.Fprintf(out, "## %s\n\n%s\n\n", r.Title, r.Description)
		if r.Example != "" {
			fmt.Fprintf(out, "```\n%s\n```\n\n", r.Example)
		}
		if len(r.Evidence) > 0 {
			fmt.Fprintf(out, "Evidence: `%s`\n\n", strings.Join(r.Evidence, "`, `"))
		}
	}
	return nil
}
func RenderEvidenceReference(out io.Writer, m *SkillModel) error {
	p := m.Profile
	fmt.Fprintf(out, "# %s evidence map\n\n", p.Name)
	fmt.Fprintf(out, "- Source: %s\n- Local path analyzed: %s\n- Intent: %s\n- Languages: %s\n\n", p.Source, p.LocalPath, p.Intent, strings.Join(p.Languages, ", "))
	fmt.Fprintln(out, "## Package files")
	for _, pf := range p.PackageFiles {
		fmt.Fprintf(out, "- `%s` (%s) %s %s\n", pf.Path, pf.Ecosystem, pf.Name, pf.Version)
	}
	fmt.Fprintln(out, "\n## Documentation")
	for _, d := range append(p.Readmes, p.Docs...) {
		fmt.Fprintf(out, "- `%s` - %s\n", d.Path, oneLine(d.Summary))
	}
	fmt.Fprintln(out, "\n## Examples and tests")
	for _, e := range p.Examples {
		fmt.Fprintf(out, "- `%s`\n", e.Path)
	}
	for _, t := range p.Tests {
		fmt.Fprintf(out, "- `%s`\n", t.Path)
	}
	return nil
}
func skillName(s string) string {
	return strings.ToLower(strings.NewReplacer("/", "-", "_", "-", " ", "-").Replace(s))
}
func anchor(s string) string {
	return strings.ToLower(strings.NewReplacer(" ", "-", "/", "-", "`", "", ".", "").Replace(s))
}
func table(s string) string   { return strings.ReplaceAll(s, "|", "\\|") }
func oneLine(s string) string { return strings.ReplaceAll(strings.TrimSpace(s), "\n", " ") }
