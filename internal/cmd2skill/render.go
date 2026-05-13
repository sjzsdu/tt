package cmd2skill

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func Run(commandName string, opts Options, stdout io.Writer) error {
	model, err := Discover(commandName, opts)
	if err != nil {
		return fmt.Errorf("parse command %s: %w", commandName, err)
	}
	if opts.DryRun {
		return RenderAll(model, stdout)
	}
	if opts.Markdown {
		tempDir, err := os.MkdirTemp("", "cmd2skill-*")
		if err != nil {
			return fmt.Errorf("create temp dir: %w", err)
		}
		defer os.RemoveAll(tempDir)
		if err := WriteSkillFiles(model, tempDir, stdout); err != nil {
			return err
		}
		cmd := exec.Command(os.Args[0], "markdown", tempDir)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("run markdown command: %w", err)
		}
		return nil
	}
	skillDir, err := resolveSkillDir(opts.TargetDir, model.Name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return fmt.Errorf("create skill dir: %w", err)
	}
	if err := WriteSkillFiles(model, skillDir, stdout); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Generated skills for %s in %s/\n", model.Name, skillDir)
	return nil
}

func resolveSkillDir(targetDir, name string) (string, error) {
	if targetDir == "" {
		targetDir = "~/.agents/skills"
	}
	if targetDir == "~" || strings.HasPrefix(targetDir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get home dir: %w", err)
		}
		if targetDir == "~" {
			return filepath.Join(home, "skills", name), nil
		}
		return filepath.Join(home, strings.TrimPrefix(targetDir, "~/"), name), nil
	}
	return filepath.Join(targetDir, name), nil
}

func WriteSkillFiles(model *CLIModel, dir string, log io.Writer) error {
	mainPath := filepath.Join(dir, "SKILL.md")
	mainFile, err := os.Create(mainPath)
	if err != nil {
		return fmt.Errorf("create main skill file: %w", err)
	}
	if err := RenderMainSkill(model, mainFile); err != nil {
		mainFile.Close()
		return err
	}
	mainFile.Close()
	fmt.Fprintln(log, "  wrote: SKILL.md")
	refDir := filepath.Join(dir, "references")
	if err := os.MkdirAll(refDir, 0o755); err != nil {
		return fmt.Errorf("create references dir: %w", err)
	}
	for _, n := range referenceNodes(model.Root) {
		file := filepath.Join(refDir, filenameFor(n)+".md")
		f, err := os.Create(file)
		if err != nil {
			return fmt.Errorf("create ref file: %w", err)
		}
		if err := RenderCommandReference(model.Name, n, f); err != nil {
			f.Close()
			return err
		}
		f.Close()
		fmt.Fprintf(log, "  wrote: references/%s.md\n", filenameFor(n))
	}
	return nil
}

func RenderAll(model *CLIModel, out io.Writer) error {
	if err := RenderMainSkill(model, out); err != nil {
		return err
	}
	for _, n := range referenceNodes(model.Root) {
		fmt.Fprint(out, "\n---\n\n")
		if err := RenderCommandReference(model.Name, n, out); err != nil {
			return err
		}
	}
	return nil
}

func RenderMainSkill(model *CLIModel, out io.Writer) error {
	root := model.Root
	desc := root.Description
	if desc == "" {
		desc = model.Name + " command line tool"
	}
	fmt.Fprintf(out, "---\nname: %s\ndescription: Use %s. %s\n---\n\n", sanitizeSkillName(model.Name), model.Name, escapeYAMLLine(desc))
	fmt.Fprintf(out, "# %s\n\n", model.Name)
	fmt.Fprintf(out, "Use this skill when you need to operate the `%s` CLI. Prefer read-only discovery commands first, inspect available options, and use dry-run or confirmation flags before destructive changes when the CLI supports them.\n\n", model.Name)
	if root.Usage != "" {
		fmt.Fprintf(out, "## Usage\n\n```bash\n%s\n```\n\n", root.Usage)
	}
	if len(root.Children) > 0 {
		fmt.Fprintln(out, "## Quick command guide")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "| Command | When to use |")
		fmt.Fprintln(out, "| --- | --- |")
		for _, child := range root.Children {
			fmt.Fprintf(out, "| `%s` | %s |\n", strings.Join(child.Path, " "), tableEscape(child.Description))
		}
		fmt.Fprintln(out)
	}
	if len(root.Flags) > 0 {
		fmt.Fprintln(out, "## Global options")
		fmt.Fprintln(out)
		renderFlags(out, root.Flags)
		fmt.Fprintln(out)
	}
	fmt.Fprintln(out, "## Agent operating guidance")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "- Start with `--help` on the exact subcommand when unsure about required arguments.")
	fmt.Fprintln(out, "- Prefer non-mutating commands for inspection before create/update/delete operations.")
	fmt.Fprintln(out, "- Use explicit paths, namespaces, projects, or targets instead of relying on ambient defaults when possible.")
	fmt.Fprintln(out, "- After running a mutating command, verify the result with the closest read-only status/list/show command.")
	fmt.Fprintln(out)
	refs := referenceNodes(root)
	if len(refs) > 0 {
		fmt.Fprintln(out, "## References")
		fmt.Fprintln(out)
		for _, n := range refs {
			fmt.Fprintf(out, "- [%s](references/%s.md)\n", strings.Join(n.Path, " "), filenameFor(n))
		}
	}
	if len(model.Failures) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "## Discovery notes")
		fmt.Fprintln(out)
		for _, f := range model.Failures {
			fmt.Fprintf(out, "- `%s`: %s\n", strings.Join(f.Path, " "), f.Error)
		}
	}
	return nil
}

func RenderCommandReference(main string, n *CommandNode, out io.Writer) error {
	desc := n.Description
	if desc == "" {
		desc = strings.Join(n.Path, " ") + " command"
	}
	fmt.Fprintf(out, "---\nname: %s\ndescription: %s\n---\n\n", sanitizeSkillName(strings.Join(n.Path, "-")), escapeYAMLLine(desc))
	fmt.Fprintf(out, "# %s\n\n", strings.Join(n.Path, " "))
	if n.Description != "" {
		fmt.Fprintf(out, "%s\n\n", n.Description)
	}
	if n.Usage != "" {
		fmt.Fprintf(out, "## Usage\n\n```bash\n%s\n```\n\n", n.Usage)
	}
	if len(n.Flags) > 0 {
		fmt.Fprintln(out, "## Options")
		fmt.Fprintln(out)
		renderFlags(out, n.Flags)
		fmt.Fprintln(out)
	}
	if len(n.Examples) > 0 {
		fmt.Fprintln(out, "## Examples")
		fmt.Fprintln(out)
		for _, e := range n.Examples {
			fmt.Fprintf(out, "```bash\n%s\n```\n", e.Command)
			if e.Desc != "" {
				fmt.Fprintf(out, "%s\n", e.Desc)
			}
			fmt.Fprintln(out)
		}
	}
	if len(n.Children) > 0 {
		fmt.Fprintln(out, "## Subcommands")
		fmt.Fprintln(out)
		renderNestedSubcommands(out, n.Children, 3)
	}
	return nil
}

func renderNestedSubcommands(out io.Writer, nodes []*CommandNode, headingLevel int) {
	kids := sortedNodes(nodes)
	for _, c := range kids {
		fmt.Fprintf(out, "%s %s\n\n", strings.Repeat("#", headingLevel), strings.Join(c.Path, " "))
		if c.Description != "" {
			fmt.Fprintf(out, "%s\n\n", c.Description)
		}
		if c.Usage != "" {
			fmt.Fprintf(out, "```bash\n%s\n```\n\n", c.Usage)
		}
		if len(c.Flags) > 0 {
			renderFlags(out, c.Flags)
			fmt.Fprintln(out)
		}
		if len(c.Children) > 0 {
			renderNestedSubcommands(out, c.Children, headingLevel+1)
		}
	}
}

func renderFlags(out io.Writer, flags []Flag) {
	fmt.Fprintln(out, "| Flags | Type | Description |")
	fmt.Fprintln(out, "| --- | --- | --- |")
	for _, f := range flags {
		desc := f.Description
		if f.Global {
			desc += " (global)"
		}
		fmt.Fprintf(out, "| `%s` | %s | %s |\n", formatFlag(f), tableEscape(f.Type), tableEscape(desc))
	}
}

func referenceNodes(root *CommandNode) []*CommandNode {
	if root == nil {
		return nil
	}
	return sortedNodes(root.Children)
}

func sortedNodes(nodes []*CommandNode) []*CommandNode {
	sorted := append([]*CommandNode{}, nodes...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return strings.Join(sorted[i].Path, " ") < strings.Join(sorted[j].Path, " ")
	})
	return sorted
}
func formatFlag(f Flag) string {
	if f.Shorthand != "" && f.Name != f.Shorthand {
		return fmt.Sprintf("-%s, --%s", f.Shorthand, f.Name)
	}
	if len(f.Name) == 1 {
		return "-" + f.Name
	}
	return "--" + f.Name
}
func filenameFor(n *CommandNode) string { return sanitizeSkillName(strings.Join(n.Path[1:], "-")) }
func sanitizeSkillName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	r := strings.NewReplacer(" ", "-", "_", "-", "/", "-", ":", "-")
	s = r.Replace(s)
	return strings.Trim(s, "-")
}
func tableEscape(s string) string { return strings.ReplaceAll(s, "|", "\\|") }
func escapeYAMLLine(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), ": ", " - ")
}
