package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/tt/internal/executor"
	"github.com/sjzsdu/tt/internal/formula"
	"github.com/sjzsdu/tt/internal/formularun"
	"github.com/sjzsdu/tt/internal/molecule"
	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
)

var (
	formulaDir         string
	formulaVars        []string
	formulaOutput      string
	formulaTitle       string
	formulaMarkdown    bool
	formulaPort        int
	formulaAgent       string
	formulaModel       string
	formulaSession     string
	formulaWeb         bool
	formulaWebPort     int
	formulaDryRun      bool
	formulaDebug       bool
	formulaVerbose     bool
	formulaNoSave      bool
	formulaRunsLimit   int
	formulaRunsFormula string
	formulaRunsStatus  string
	formulaRunShowStep string
)

var formulaCmd = &cobra.Command{
	Use:   "formula",
	Short: "Manage and instantiate formula templates",
	Long: `Formula templates define structured task workflows with variables,
dependencies, and control flow. Compile and instantiate formulas to generate
task trees for complex work.

Tip: Prefer embedded agents when choosing step agents, such as coder, planner,
tester, product-manager, ui, or full-stack. Use tt agent --list to see embedded
and picoclaw-configured agents.`,
}

var formulaListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available formulas",
	Args:  cobra.NoArgs,
	RunE:  runFormulaList,
}

var formulaShowCmd = &cobra.Command{
	Use:   "show [name]",
	Short: "Show formula details",
	Long: `Show formula details. Without a name, lists all formulas.
With --markdown and no name, generates a combined Markdown preview of all formulas.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runFormulaShow,
}

var formulaCompileCmd = &cobra.Command{
	Use:   "compile <name>",
	Short: "Compile a formula and show the recipe",
	Args:  cobra.ExactArgs(1),
	RunE:  runFormulaCompile,
}

var formulaInstantiateCmd = &cobra.Command{
	Use:   "instantiate <name>",
	Short: "Instantiate a formula into a task tree",
	Args:  cobra.ExactArgs(1),
	RunE:  runFormulaInstantiate,
}

var formulaValidateCmd = &cobra.Command{
	Use:   "validate <file>",
	Short: "Validate a formula file",
	Args:  cobra.ExactArgs(1),
	RunE:  runFormulaValidate,
}

var formulaRunCmd = &cobra.Command{
	Use:   "run <name> [required-var-value]",
	Short: "Execute a formula with picoclaw agents",
	Long: `Execute a formula by running each step through the configured agent.
Steps are executed in dependency order, with parallel steps running concurrently.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runFormulaRun,
}

var formulaRunsCmd = &cobra.Command{
	Use:   "runs",
	Short: "List saved formula runs",
	Args:  cobra.NoArgs,
	RunE:  runFormulaRuns,
}

var formulaRunOpenCmd = &cobra.Command{
	Use:   "open [run-id|latest]",
	Short: "Open a saved formula run dashboard",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runFormulaRunOpen,
}

var formulaRunShowCmd = &cobra.Command{
	Use:   "show [run-id|latest]",
	Short: "Show a saved formula run",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runFormulaRunShow,
}

func init() {
	formulaCmd.PersistentFlags().StringVarP(&formulaDir, "dir", "d", "", "formula search directory (default: .tt/formulas, ~/.tt/formulas)")
	formulaCmd.PersistentFlags().StringArrayVar(&formulaVars, "var", nil, "variable override (key=value, repeatable)")

	formulaInstantiateCmd.Flags().StringVarP(&formulaOutput, "output", "o", "json", "output format: json, yaml, text, prompt")
	formulaInstantiateCmd.Flags().StringVarP(&formulaTitle, "title", "t", "", "override root task title")

	formulaShowCmd.Flags().BoolVar(&formulaMarkdown, "markdown", false, "render formula as Markdown with Mermaid diagram and preview in browser")
	formulaShowCmd.Flags().IntVarP(&formulaPort, "port", "p", 9598, "web server port for --markdown preview")

	formulaRunCmd.Flags().StringVar(&formulaAgent, "agent", "general", "default agent for steps without explicit agent config")
	formulaRunCmd.Flags().StringVar(&formulaModel, "model", "", "default model override")
	formulaRunCmd.Flags().StringVar(&formulaSession, "session", "cli:formula", "session key prefix")
	formulaRunCmd.Flags().BoolVar(&formulaWeb, "web", false, "show a live web dashboard while the formula runs")
	formulaRunCmd.Flags().IntVar(&formulaWebPort, "web-port", 9705, "dashboard web server port")
	formulaRunCmd.Flags().BoolVar(&formulaDryRun, "dry-run", false, "print execution plan without running")
	formulaRunCmd.Flags().BoolVar(&formulaDebug, "debug", false, "enable debug logging")
	formulaRunCmd.Flags().BoolVarP(&formulaVerbose, "verbose", "v", false, "show full output of each step")
	formulaRunCmd.Flags().BoolVar(&formulaNoSave, "no-save", false, "do not save formula run state under .tt/runs/formula")
	formulaRunsCmd.Flags().IntVar(&formulaRunsLimit, "limit", 20, "maximum number of runs to list")
	formulaRunsCmd.Flags().StringVar(&formulaRunsFormula, "formula", "", "filter runs by formula name")
	formulaRunsCmd.Flags().StringVar(&formulaRunsStatus, "status", "", "filter runs by status")
	formulaRunOpenCmd.Flags().IntVar(&formulaWebPort, "web-port", 9705, "dashboard web server port")
	formulaRunShowCmd.Flags().StringVar(&formulaRunShowStep, "step", "", "show details for a specific step id")
	formulaRunCmd.AddCommand(formulaRunOpenCmd)
	formulaRunCmd.AddCommand(formulaRunShowCmd)

	formulaCmd.AddCommand(formulaListCmd)
	formulaCmd.AddCommand(formulaShowCmd)
	formulaCmd.AddCommand(formulaCompileCmd)
	formulaCmd.AddCommand(formulaInstantiateCmd)
	formulaCmd.AddCommand(formulaValidateCmd)
	formulaCmd.AddCommand(formulaRunCmd)
	formulaCmd.AddCommand(formulaRunsCmd)

	rootCmd.AddCommand(formulaCmd)
}

func getSearchPaths() []string {
	if formulaDir != "" {
		return []string{formulaDir}
	}

	paths := []string{}
	if cwd, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Join(cwd, ".tt", "formulas"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".tt", "formulas"))
	}
	return paths
}

func parseVars() map[string]string {
	vars := make(map[string]string)
	for _, v := range formulaVars {
		key, value, ok := strings.Cut(v, "=")
		if ok && key != "" {
			vars[key] = value
		}
	}
	return vars
}

func applyFormulaRunPositionalVars(f *formula.Formula, values []string, vars map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	if f == nil {
		return fmt.Errorf("formula is required for positional variables")
	}
	required := f.RequiredVarNames()
	if len(required) != 1 {
		return fmt.Errorf("positional value shorthand requires exactly one required variable, found %d; use --var key=value", len(required))
	}
	name := required[0]
	if _, exists := vars[name]; exists {
		return fmt.Errorf("variable %q is already set via --var; remove the positional value or the --var override", name)
	}
	value := strings.TrimSpace(strings.Join(values, " "))
	if value == "" {
		return fmt.Errorf("positional value for required variable %q cannot be empty", name)
	}
	vars[name] = value
	return nil
}

func runFormulaList(cmd *cobra.Command, args []string) error {
	paths := getSearchPaths()
	if len(paths) == 0 {
		fmt.Println("No formula search paths configured.")
		fmt.Println("Create formulas in .tt/formulas/ or ~/.tt/formulas/")
		return nil
	}

	found := false
	for _, dir := range paths {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !isFormulaFile(name) {
				continue
			}

			formulaName := extractFormulaName(name)
			path := filepath.Join(dir, name)

			p := formula.NewParser(dir)
			f, err := p.ParseFile(path)
			if err != nil {
				fmt.Printf("  %s (parse error: %v)\n", formulaName, err)
				continue
			}

			desc := f.Description
			if desc == "" {
				desc = "(no description)"
			}
			fmt.Printf("  %-30s %s\n", formulaName, desc)
			found = true
		}
	}

	if !found {
		fmt.Println("No formulas found.")
		fmt.Println("Create formulas in .tt/formulas/ or ~/.tt/formulas/")
	}
	return nil
}

func isFormulaFile(name string) bool {
	ext := filepath.Ext(name)
	if ext == ".toml" || ext == ".json" {
		return true
	}
	return strings.HasSuffix(name, ".formula.toml") || strings.HasSuffix(name, ".formula.json")
}

func extractFormulaName(filename string) string {
	name := filename
	for _, ext := range []string{".formula.toml", ".formula.json", ".toml", ".json"} {
		if strings.HasSuffix(name, ext) {
			name = strings.TrimSuffix(name, ext)
			break
		}
	}
	return name
}

func runFormulaShow(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		if formulaMarkdown {
			return runFormulaShowAllMarkdown()
		}
		return runFormulaList(cmd, args)
	}

	name := args[0]
	p := formula.NewParser(getSearchPaths()...)

	f, err := p.LoadByName(name)
	if err != nil {
		return err
	}

	resolved, err := p.Resolve(f)
	if err != nil {
		return fmt.Errorf("resolving: %w", err)
	}

	if formulaMarkdown {
		return runFormulaShowMarkdown(resolved)
	}

	fmt.Printf("Formula: %s\n", resolved.Formula)
	if resolved.Description != "" {
		fmt.Printf("Description: %s\n", resolved.Description)
	}
	fmt.Printf("Version: %d\n", resolved.Version)
	fmt.Printf("Type: %s\n", resolved.Type)
	if resolved.Phase != "" {
		fmt.Printf("Phase: %s\n", resolved.Phase)
	}
	fmt.Println()

	if len(resolved.Vars) > 0 {
		fmt.Println("Variables:")
		for _, vname := range sortedVarNames(resolved.Vars) {
			def := resolved.Vars[vname]
			if def == nil {
				continue
			}
			req := ""
			if def.Required {
				req = " (required)"
			}
			defVal := ""
			if def.Default != nil {
				defVal = fmt.Sprintf(" (default: %s)", *def.Default)
			}
			desc := def.Description
			if desc == "" {
				desc = vname
			}
			fmt.Printf("  %-20s %s%s%s\n", vname, desc, req, defVal)
		}
		fmt.Println()
	}

	fmt.Printf("Steps (%d):\n", len(resolved.Steps))
	for i, step := range resolved.Steps {
		priority := ""
		if step.Priority != nil {
			priority = fmt.Sprintf("[P%d] ", *step.Priority)
		}
		deps := ""
		if len(step.DependsOn) > 0 {
			deps = fmt.Sprintf(" (depends: %s)", strings.Join(step.DependsOn, ", "))
		}
		if len(step.Needs) > 0 {
			deps = fmt.Sprintf(" (needs: %s)", strings.Join(step.Needs, ", "))
		}
		fmt.Printf("  %d. %s%-15s → %s%s\n", i+1, priority, step.ID, step.Title, deps)
	}

	return nil
}

func sortedVarNames(vars map[string]*formula.VarDef) []string {
	names := make([]string, 0, len(vars))
	for name := range vars {
		names = append(names, name)
	}
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if names[i] > names[j] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	return names
}

func runFormulaCompile(cmd *cobra.Command, args []string) error {
	name := args[0]
	vars := parseVars()

	recipe, err := formula.Compile(context.Background(), name, getSearchPaths(), vars)
	if err != nil {
		return err
	}

	fmt.Printf("Recipe: %s\n", recipe.Name)
	if recipe.Description != "" {
		fmt.Printf("Description: %s\n", recipe.Description)
	}
	fmt.Printf("RootOnly: %v\n", recipe.RootOnly)
	fmt.Printf("Steps (%d):\n", len(recipe.Steps))

	for i, step := range recipe.Steps {
		priority := ""
		if step.Priority != nil {
			priority = fmt.Sprintf("[P%d] ", *step.Priority)
		}
		deps := ""
		for _, dep := range recipe.Deps {
			if dep.StepID == step.ID && dep.Type != "parent-child" {
				if deps != "" {
					deps += ", "
				}
				deps += fmt.Sprintf("%s(%s)", dep.DependsOnID, dep.Type)
			}
		}
		if deps != "" {
			deps = " (blocks: " + deps + ")"
		}
		fmt.Printf("  %d. %s%-30s %s%s\n", i+1, priority, step.ID, step.Title, deps)
	}

	return nil
}

func runFormulaInstantiate(cmd *cobra.Command, args []string) error {
	name := args[0]
	vars := parseVars()

	recipe, err := formula.Compile(context.Background(), name, getSearchPaths(), vars)
	if err != nil {
		return err
	}

	opts := molecule.Options{
		Title: formulaTitle,
		Vars:  vars,
	}

	result, err := molecule.Instantiate(recipe, opts)
	if err != nil {
		return err
	}

	switch formulaOutput {
	case "json":
		return outputJSON(result)
	case "text":
		return outputText(result)
	case "prompt":
		return outputPrompt(result)
	default:
		return fmt.Errorf("unknown output format: %s", formulaOutput)
	}
}

func outputJSON(result *molecule.Result) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func outputText(result *molecule.Result) error {
	fmt.Printf("Root: %s\n", result.RootID)
	fmt.Printf("Tasks: %d\n\n", result.Created)

	for i, task := range result.Tasks {
		priority := ""
		if task.Priority != nil {
			priority = fmt.Sprintf("[P%d] ", *task.Priority)
		}
		deps := ""
		if len(task.Dependencies) > 0 {
			deps = fmt.Sprintf(" (after: %s)", strings.Join(task.Dependencies, ", "))
		}
		fmt.Printf("%d. %s%s\n", i+1, priority, task.Title)
		if task.Description != "" {
			fmt.Printf("   %s\n", task.Description)
		}
		if deps != "" {
			fmt.Printf("   %s\n", deps)
		}
		fmt.Println()
	}
	return nil
}

func outputPrompt(result *molecule.Result) error {
	fmt.Println("请按以下顺序完成任务，每个任务完成后确认再继续下一个：")
	fmt.Println()

	for i, task := range result.Tasks {
		if task.IsRoot {
			continue
		}
		priority := "普通"
		if task.Priority != nil {
			switch *task.Priority {
			case 0:
				priority = "最高"
			case 1:
				priority = "高"
			case 2:
				priority = "中"
			case 3:
				priority = "低"
			case 4:
				priority = "最低"
			}
		}
		fmt.Printf("## 任务 %d (优先级: %s)\n", i, priority)
		fmt.Printf("**%s**\n", task.Title)
		if task.Description != "" {
			fmt.Println(task.Description)
		}
		if task.Notes != "" {
			fmt.Println(task.Notes)
		}
		if len(task.Dependencies) > 0 {
			fmt.Printf("依赖: 任务 %s 完成后开始\n", strings.Join(task.Dependencies, ", "))
		}
		fmt.Println()
	}
	return nil
}

func runFormulaValidate(cmd *cobra.Command, args []string) error {
	path := args[0]

	p := formula.NewParser()
	f, err := p.ParseFile(path)
	if err != nil {
		return err
	}

	if err := f.Validate(); err != nil {
		return err
	}

	fmt.Printf("Formula %q is valid.\n", f.Formula)
	return nil
}

func runFormulaShowMarkdown(resolved *formula.Formula) error {
	recipe, err := formula.Compile(context.Background(), resolved.Formula, getSearchPaths(), nil)
	if err != nil {
		return err
	}

	md := generateFormulaMarkdown(resolved, recipe)

	tmpDir, err := os.MkdirTemp("", "tt-formula-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}

	mdPath := filepath.Join(tmpDir, resolved.Formula+".md")
	if err := os.WriteFile(mdPath, []byte(md), 0644); err != nil {
		os.RemoveAll(tmpDir)
		return fmt.Errorf("write formula file: %w", err)
	}

	mdRoot = tmpDir
	mdContent = ""
	mdContentOnly = false
	mdPort = formulaPort
	mdInitialPath = "/view/" + resolved.Formula + ".md"

	defer os.RemoveAll(tmpDir)
	return runMarkdownServer()
}

func runFormulaShowAllMarkdown() error {
	paths := getSearchPaths()
	if len(paths) == 0 {
		return fmt.Errorf("no formula search paths configured")
	}

	var formulas []*formula.Formula
	seen := make(map[string]bool)

	for _, dir := range paths {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !isFormulaFile(entry.Name()) {
				continue
			}
			name := extractFormulaName(entry.Name())
			if seen[name] {
				continue
			}

			p := formula.NewParser(dir)
			f, err := p.ParseFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				continue
			}
			resolved, err := p.Resolve(f)
			if err != nil {
				continue
			}
			formulas = append(formulas, resolved)
			seen[name] = true
		}
	}

	if len(formulas) == 0 {
		return fmt.Errorf("no formulas found in search paths")
	}

	tmpDir, err := os.MkdirTemp("", "tt-formulas-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}

	for _, f := range formulas {
		recipe, err := formula.Compile(context.Background(), f.Formula, getSearchPaths(), nil)
		if err != nil {
			continue
		}

		md := generateFormulaMarkdown(f, recipe)
		mdPath := filepath.Join(tmpDir, f.Formula+".md")
		if err := os.WriteFile(mdPath, []byte(md), 0644); err != nil {
			return fmt.Errorf("write %s: %w", f.Formula, err)
		}
	}

	mdRoot = tmpDir
	mdContent = ""
	mdContentOnly = false
	mdPort = formulaPort
	mdInitialPath = ""

	fmt.Printf("Generated %d formula files in %s\n", len(formulas), tmpDir)
	defer os.RemoveAll(tmpDir)
	return runMarkdownServer()
}

func generateFormulaMarkdown(f *formula.Formula, recipe *formula.Recipe) string {
	var b strings.Builder

	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("title: \"%s\"\n", escapeYAML(f.Formula)))
	if f.Description != "" {
		b.WriteString(fmt.Sprintf("description: \"%s\"\n", escapeYAML(f.Description)))
	}
	b.WriteString("---\n\n")

	b.WriteString(fmt.Sprintf("# %s\n\n", f.Formula))
	if f.Description != "" {
		b.WriteString(fmt.Sprintf("> %s\n\n", f.Description))
	}

	b.WriteString("## Formula Details\n\n")
	b.WriteString(fmt.Sprintf("- **Version:** `%d`\n", f.Version))
	b.WriteString(fmt.Sprintf("- **Type:** `%s`\n", f.Type))
	if f.Phase != "" {
		b.WriteString(fmt.Sprintf("- **Phase:** `%s`\n", f.Phase))
	}
	b.WriteString(fmt.Sprintf("- **Steps:** `%d`\n", len(recipe.Steps)))
	b.WriteString("\n")

	if len(f.Vars) > 0 {
		b.WriteString("## Variables\n\n")
		b.WriteString("| Name | Description | Default | Required |\n")
		b.WriteString("|------|-------------|---------|----------|\n")
		for _, vname := range sortedVarNames(f.Vars) {
			def := f.Vars[vname]
			if def == nil {
				continue
			}
			desc := def.Description
			if desc == "" {
				desc = "-"
			}
			defVal := "-"
			if def.Default != nil {
				defVal = fmt.Sprintf("`%s`", *def.Default)
			}
			req := ""
			if def.Required {
				req = "✅"
			}
			b.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s |\n", vname, desc, defVal, req))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Dependency Graph\n\n")
	b.WriteString("```mermaid\n")
	b.WriteString(generateMermaidGraph(recipe))
	b.WriteString("\n```\n\n")

	b.WriteString("## Quick Start\n\n")
	b.WriteString(generateQuickStart(f, recipe))
	b.WriteString("\n")

	b.WriteString("## Steps\n\n")
	for i, step := range recipe.Steps {
		if step.IsRoot {
			continue
		}
		priority := ""
		if step.Priority != nil {
			priority = fmt.Sprintf(" [P%d]", *step.Priority)
		}
		b.WriteString(fmt.Sprintf("### %d. `%s`%s\n\n", i, step.ID, priority))
		b.WriteString(fmt.Sprintf("**%s**\n\n", step.Title))
		if step.Description != "" {
			b.WriteString(fmt.Sprintf("%s\n\n", step.Description))
		}
		if step.Notes != "" {
			b.WriteString(fmt.Sprintf("> %s\n\n", step.Notes))
		}

		deps := findDepsForStep(recipe, step.ID)
		if len(deps) > 0 {
			b.WriteString(fmt.Sprintf("**Dependencies:** %s\n\n", strings.Join(deps, ", ")))
		}

		if len(step.Labels) > 0 {
			b.WriteString(fmt.Sprintf("**Labels:** %s\n\n", strings.Join(step.Labels, ", ")))
		}

		if step.Gate != nil {
			b.WriteString(fmt.Sprintf("**Gate:** %s (type: %s)\n\n", step.Gate.ID, step.Gate.Type))
		}
	}

	return b.String()
}

func generateMermaidGraph(recipe *formula.Recipe) string {
	var b strings.Builder
	b.WriteString("graph TD\n")

	parallelSteps := findParallelSteps(recipe)
	endSteps := findEndSteps(recipe)
	depths := computeStepDepths(recipe)
	maxDepth := 1
	for _, d := range depths {
		if d > maxDepth {
			maxDepth = d
		}
	}

	for _, step := range recipe.Steps {
		nodeID := mermaidNodeID(step.ID)
		label := mermaidLabel(step.ID, step.Title, step.Priority)
		shape := mermaidShape(step, endSteps, parallelSteps)
		b.WriteString(fmt.Sprintf("    %s%s%s\n", nodeID, shape.open, label+shape.close))

		depth := depths[step.ID]
		color := depthColor(depth, maxDepth)

		if step.IsRoot {
			b.WriteString(fmt.Sprintf("    class %s nodeRoot\n", nodeID))
		} else if step.Gate != nil {
			b.WriteString(fmt.Sprintf("    class %s nodeGate\n", nodeID))
		} else {
			b.WriteString(fmt.Sprintf("    classDef c%s fill:%s,stroke:%s,stroke-width:2px\n", nodeID, color.Fill, color.Stroke))
			b.WriteString(fmt.Sprintf("    class %s c%s\n", nodeID, nodeID))
		}
	}

	b.WriteString("    classDef nodeRoot fill:#e8eaf6,stroke:#3f51b5,stroke-width:3px\n")
	b.WriteString("    classDef nodeGate fill:#fce4ec,stroke:#c2185b,stroke-width:2px,stroke-dasharray: 5 5\n")

	for _, dep := range recipe.Deps {
		if dep.Type == "parent-child" {
			continue
		}
		from := mermaidNodeID(dep.DependsOnID)
		to := mermaidNodeID(dep.StepID)
		edgeStyle := " -->"
		if dep.Type == "waits-for" {
			edgeStyle = " -.-> |wait|"
		}
		b.WriteString(fmt.Sprintf("    %s%s %s\n", from, edgeStyle, to))
	}

	return b.String()
}

type shapeDef struct {
	open  string
	close string
}

func mermaidShape(step formula.RecipeStep, endSteps, parallelSteps map[string]bool) shapeDef {
	if step.IsRoot {
		return shapeDef{open: "([\"", close: "\"])"}
	}
	if step.Gate != nil {
		return shapeDef{open: "{\"", close: "\"}"}
	}
	if endSteps[step.ID] {
		return shapeDef{open: "([\"", close: "\"])"}
	}
	if parallelSteps[step.ID] {
		return shapeDef{open: "(\"", close: "\")"}
	}
	return shapeDef{open: "[\"", close: "\"]"}
}

type nodeColor struct {
	Fill   string
	Stroke string
}

func depthColor(depth, maxDepth int) nodeColor {
	ratio := float64(depth) / float64(maxDepth)

	stops := []struct {
		ratio  float64
		fill   string
		stroke string
	}{
		{0.0, "#e3f2fd", "#1976d2"},
		{0.25, "#e0f2f1", "#00796b"},
		{0.5, "#e8f5e9", "#388e3c"},
		{0.75, "#fff8e1", "#f57f17"},
		{1.0, "#fbe9e7", "#d84315"},
	}

	if ratio <= stops[0].ratio {
		return nodeColor{Fill: stops[0].fill, Stroke: stops[0].stroke}
	}
	for i := 1; i < len(stops); i++ {
		if ratio <= stops[i].ratio {
			return nodeColor{Fill: stops[i].fill, Stroke: stops[i].stroke}
		}
	}
	last := stops[len(stops)-1]
	return nodeColor{Fill: last.fill, Stroke: last.stroke}
}

func findEndSteps(recipe *formula.Recipe) map[string]bool {
	hasDependent := make(map[string]bool)
	for _, dep := range recipe.Deps {
		if dep.Type == "parent-child" {
			continue
		}
		hasDependent[dep.DependsOnID] = true
	}

	ends := make(map[string]bool)
	for _, step := range recipe.Steps {
		if step.IsRoot {
			continue
		}
		if !hasDependent[step.ID] {
			ends[step.ID] = true
		}
	}
	return ends
}

func computeStepDepths(recipe *formula.Recipe) map[string]int {
	depths := make(map[string]int)
	for _, step := range recipe.Steps {
		depths[step.ID] = 0
	}

	for _, step := range recipe.Steps {
		d := stepDepth(step.ID, recipe, depths, make(map[string]bool))
		depths[step.ID] = d
	}
	return depths
}

func stepDepth(id string, recipe *formula.Recipe, depths map[string]int, visiting map[string]bool) int {
	if d, ok := depths[id]; ok && d > 0 {
		return d
	}
	if visiting[id] {
		return 0
	}
	visiting[id] = true

	maxParentDepth := 0
	for _, dep := range recipe.Deps {
		if dep.StepID == id && dep.Type != "parent-child" {
			parentD := stepDepth(dep.DependsOnID, recipe, depths, visiting)
			if parentD+1 > maxParentDepth {
				maxParentDepth = parentD + 1
			}
		}
	}

	depths[id] = maxParentDepth
	return maxParentDepth
}

func findParallelSteps(recipe *formula.Recipe) map[string]bool {
	parallel := make(map[string]bool)
	depMap := make(map[string][]string)
	for _, dep := range recipe.Deps {
		if dep.Type == "parent-child" {
			continue
		}
		depMap[dep.StepID] = append(depMap[dep.StepID], dep.DependsOnID)
	}

	sourceTargets := make(map[string][]string)
	for stepID, deps := range depMap {
		for _, dep := range deps {
			sourceTargets[dep] = append(sourceTargets[dep], stepID)
		}
	}

	for _, targets := range sourceTargets {
		if len(targets) > 1 {
			for _, t := range targets {
				parallel[t] = true
			}
		}
	}

	return parallel
}

func findDepsForStep(recipe *formula.Recipe, stepID string) []string {
	var deps []string
	for _, dep := range recipe.Deps {
		if dep.StepID == stepID && dep.Type != "parent-child" {
			deps = append(deps, dep.DependsOnID)
		}
	}
	return deps
}

func mermaidNodeID(id string) string {
	result := strings.ReplaceAll(id, ".", "_")
	result = strings.ReplaceAll(result, "-", "_")
	return result
}

func mermaidLabel(id string, title string, priority *int) string {
	safeTitle := strings.ReplaceAll(title, "\"", "'")
	prefix := ""
	if priority != nil {
		prefix = fmt.Sprintf("[P%d] ", *priority)
	}
	shortID := id
	if idx := strings.LastIndex(id, "."); idx >= 0 {
		shortID = id[idx+1:]
	}
	return fmt.Sprintf("%s: %s%s", shortID, prefix, safeTitle)
}

func generateQuickStart(f *formula.Formula, recipe *formula.Recipe) string {
	var b strings.Builder
	b.WriteString("```bash\n")
	b.WriteString(fmt.Sprintf("# 查看公式详情（带 Mermaid 流程图）\n"))
	b.WriteString(fmt.Sprintf("tt formula show %s --markdown\n\n", f.Formula))
	b.WriteString(fmt.Sprintf("# 编译公式，查看任务依赖\n"))
	b.WriteString(fmt.Sprintf("tt formula compile %s\n\n", f.Formula))
	b.WriteString(fmt.Sprintf("# 实例化为 JSON 任务树\n"))
	b.WriteString(fmt.Sprintf("tt formula instantiate %s -o json\n\n", f.Formula))
	b.WriteString(fmt.Sprintf("# 实例化为中文任务提示（适合给 AI agent）\n"))
	b.WriteString(fmt.Sprintf("tt formula instantiate %s -o prompt\n\n", f.Formula))
	b.WriteString(fmt.Sprintf("# 试运行执行计划（不调用大模型）\n"))
	b.WriteString(fmt.Sprintf("tt formula run %s --dry-run\n\n", f.Formula))
	b.WriteString(fmt.Sprintf("# 执行公式\n"))
	b.WriteString(fmt.Sprintf("tt formula run %s\n\n", f.Formula))

	requiredVars := f.RequiredVarNames()
	if len(requiredVars) > 0 {
		vars := make([]string, len(requiredVars))
		for i, v := range requiredVars {
			vars[i] = fmt.Sprintf("--var %s=<value>", v)
		}
		if len(requiredVars) == 1 {
			b.WriteString(fmt.Sprintf("# 带必填变量执行: %s（位置参数简写）\n", requiredVars[0]))
			b.WriteString(fmt.Sprintf("tt formula run %s <value>\n", f.Formula))
		} else {
			b.WriteString(fmt.Sprintf("# 带必填变量执行: %s\n", strings.Join(requiredVars, ", ")))
			b.WriteString(fmt.Sprintf("tt formula run %s %s\n", f.Formula, strings.Join(vars, " ")))
		}
	} else {
		b.WriteString(fmt.Sprintf("# 传入变量值\n"))
		b.WriteString(fmt.Sprintf("tt formula run %s --var key=value\n", f.Formula))
	}
	b.WriteString("```\n")
	return b.String()
}

func escapeYAML(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return s
}

func setMarkdownContent(content string, port int) {
	mdContent = content
	mdContentOnly = true
	mdPort = port
	mdInitialPath = ""
}

func runFormulaRun(cmd *cobra.Command, args []string) error {
	name := args[0]
	vars := parseVars()

	p := formula.NewParser(getSearchPaths()...)
	f, err := p.LoadByName(name)
	if err != nil {
		return err
	}
	if err := applyFormulaRunPositionalVars(f, args[1:], vars); err != nil {
		return err
	}

	if formulaSession == "" {
		formulaSession = "cli:formula"
	}

	recipe, err := formula.Compile(context.Background(), name, getSearchPaths(), vars)
	if err != nil {
		return err
	}

	if formulaDryRun {
		return runFormulaDryRun(recipe)
	}

	loaded, err := loadTTConfig()
	if err != nil {
		return err
	}
	merged := loaded.Merged

	if err := ensurePicoclawConfigAvailable(merged.Picoclaw.Home, merged.Picoclaw.Config); err != nil {
		return err
	}

	rt, err := pcwrap.Load(pcwrap.Options{
		Home:      merged.Picoclaw.Home,
		Config:    merged.Picoclaw.Config,
		TTConfig:  merged,
		TTSources: loaded.Sources,
	})
	if err != nil {
		return picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	projectRoot, _ := os.Getwd()
	if err := formularun.EnsureWorkspaceState(projectRoot); err != nil {
		return err
	}
	restoreSessionsDir, err := useFormulaSessionsDir(projectRoot)
	if err != nil {
		return err
	}

	runner, err := rt.NewDirectRunner(pcwrap.RunOptions{
		Session:   formulaSession,
		Model:     formulaModel,
		Debug:     formulaDebug,
		Quiet:     true,
		Workspace: projectRoot,
	})
	restoreSessionsDir()
	if err != nil {
		return picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	defer runner.Close()

	exec := executor.New(recipe, executor.RunOptions{
		Vars:    vars,
		Agent:   formulaAgent,
		Model:   formulaModel,
		Session: formulaSession,
		DryRun:  formulaDryRun,
		Debug:   formulaDebug,
	})

	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()

	var runStore *formularun.Store
	if !formulaNoSave {
		runStore, err = formularun.New("", recipe, vars, formulaAgent, formulaModel, formulaSession, projectRoot)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Run ID: %s\n", runStore.Meta.RunID)
		fmt.Fprintf(out, "Saved to: %s\n", runStore.Dir)
	}

	var dashboard *formulaDashboardServer
	if runStore != nil || formulaWeb {
		dashboard = newFormulaDashboardServer(recipe)
		dashboard.state.WorkspaceDir = formulaDashboardWorkspace(projectRoot)
		if runStore != nil {
			dashboard.attachStore(runStore)
		}
	}
	if formulaWeb {
		if err := dashboard.start(formulaWebPort); err != nil {
			return err
		}
	}

	stepRunner := func(ctx context.Context, step *formula.RecipeStep, prompt string) (string, error) {
		agent := step.Agent
		if agent == nil || agent.Name == "" {
			agent = &formula.AgentConfig{Name: formulaAgent, Model: formulaModel}
		}

		sessionKey := fmt.Sprintf("agent:%s:%s:%s", agent.Name, formulaSession, step.ID)
		if agent.Session != "" {
			sessionKey = fmt.Sprintf("agent:%s:%s:%s", agent.Name, formulaSession, agent.Session)
		}

		model := agent.Model
		if model == "" {
			model = formulaModel
		}
		modelDisplay := model
		if modelDisplay == "" {
			modelDisplay = "(default from picoclaw)"
		}

		logLine := func(format string, args ...any) {
			line := fmt.Sprintf(format, args...)
			fmt.Fprintln(errOut, line)
			if dashboard != nil {
				dashboard.logf("%s", line)
			}
		}

		fmt.Fprintln(errOut)
		logLine("▶ Running: %s", step.Title)
		logLine("  Agent: %s | Model: %s", agent.Name, modelDisplay)

		if step.Condition != "" {
			condResult := executor.EvaluateCondition(step.Condition, exec.Context())
			logLine("  Condition: %s → %v", step.Condition, condResult)
			if !condResult {
				return "", nil
			}
		}

		if len(step.InputCtx) > 0 {
			inputLine := fmt.Sprintf("  Input context: %s", strings.Join(step.InputCtx, ", "))
			fmt.Fprintln(errOut, inputLine)
			if dashboard != nil {
				dashboard.logf("%s", inputLine)
			}
		}

		prompt = renderFormulaPrompt(projectRoot, prompt)
		if runStore != nil {
			_ = runStore.SaveStepPrompt(step.ID, prompt)
		}
		if dashboard != nil {
			dashboard.markStepRunning(step.ID, step.Title, agent.Name, model, sessionKey)
		}
		stepStarted := time.Now()
		if runStore != nil {
			_ = runStore.AppendEvent(formularun.Event{
				Type:    "step_started",
				StepID:  step.ID,
				Agent:   agent.Name,
				Model:   model,
				Session: sessionKey,
				Status:  "running",
			})
		}

		resp, err := runner.ProcessDirect(pcwrap.RunOptions{
			Message: prompt,
			Session: sessionKey,
			Agent:   agent.Name,
			Model:   model,
		})
		resp = strings.TrimSpace(resp)

		if err != nil {
			duration := time.Since(stepStarted).Milliseconds()
			if runStore != nil {
				_ = runStore.SaveStepError(step.ID, err.Error())
				if resp != "" {
					_ = runStore.SaveStepOutput(step.ID, resp)
				}
				_ = runStore.AppendEvent(formularun.Event{Type: "step_failed", StepID: step.ID, Status: "failed", Error: err.Error(), DurationMS: duration})
			}
			if dashboard != nil {
				dashboard.markStepFailed(step.ID, err.Error(), resp)
			}
			logLine("  ✗ Failed: %v", err)
			return resp, err
		}

		if resp == "" {
			logLine("  ⚠ Empty response from agent")
		} else {
			logLine("  ✓ Completed (%d chars)", len(resp))
			if formulaVerbose {
				fmt.Fprintf(errOut, "\n%s\n\n", resp)
				if dashboard != nil {
					dashboard.logf("%s", resp)
				}
			}
		}

		if dashboard != nil {
			dashboard.markStepCompleted(step.ID, resp)
		}
		if runStore != nil {
			_ = runStore.SaveStepOutput(step.ID, resp)
			_ = runStore.AppendEvent(formularun.Event{Type: "step_completed", StepID: step.ID, Status: "completed", DurationMS: time.Since(stepStarted).Milliseconds()})
		}

		if step.OutputKey != "" {
			logLine("  → Output key: %s", step.OutputKey)
		}

		return resp, nil
	}

	fmt.Fprintf(out, "Executing formula: %s\n", recipe.Name)
	fmt.Fprintf(out, "Steps: %d (excluding root)\n", len(recipe.Steps)-1)
	fmt.Fprintln(out, strings.Repeat("─", 50))

	runCtx, stopRunSignals := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stopRunSignals()
	result, err := exec.Run(runCtx, stepRunner)
	if runCtx.Err() != nil && err == nil {
		err = runCtx.Err()
	}

	fmt.Fprintln(out, strings.Repeat("─", 50))
	renderRunResult(cmd, result, err != nil)
	if dashboard != nil {
		dashboard.finalize(result, err)
	}
	if runStore != nil {
		status := formularun.StatusCompleted
		errMsg := ""
		if runCtx.Err() != nil {
			status = formularun.StatusInterrupted
			errMsg = runCtx.Err().Error()
		} else if err != nil {
			status = formularun.StatusFailed
			errMsg = err.Error()
		}
		_ = runStore.Finish(status, errMsg)
		if dashboard != nil {
			_ = dashboard.persistSnapshot()
		}
	}

	if formulaWeb {
		fmt.Fprintf(out, "\nWeb dashboard: http://localhost:%d\n", dashboard.port)
		fmt.Fprintln(out, "Press Ctrl-C to stop the dashboard.")
		waitForFormulaDashboardExit(dashboard)
	}

	if err != nil {
		return err
	}
	return nil
}

func runFormulaDryRun(recipe *formula.Recipe) error {
	fmt.Printf("Execution Plan for: %s\n\n", recipe.Name)

	batches, err := executor.TopologicalBatches(recipe)
	if err != nil {
		return err
	}

	for i, batch := range batches {
		fmt.Printf("Batch %d (parallel):\n", i+1)
		for _, step := range batch {
			if step.IsRoot {
				continue
			}
			agent := "default"
			if step.Agent != nil && step.Agent.Name != "" {
				agent = step.Agent.Name
			}
			skip := ""
			if step.Condition != "" {
				skip = fmt.Sprintf(" [if: %s]", step.Condition)
			}
			output := ""
			if step.OutputKey != "" {
				output = fmt.Sprintf(" → output: %s", step.OutputKey)
			}
			fmt.Printf("  - %s (%s)%s%s\n", step.ID, agent, skip, output)
		}
		fmt.Println()
	}

	return nil
}

func renderRunResult(cmd *cobra.Command, result *executor.RunResult, hasError bool) {
	out := cmd.OutOrStdout()

	fmt.Fprintf(out, "\nExecution Result: %s\n", result.RecipeName)
	fmt.Fprintf(out, "Total: %d | Completed: %d | Failed: %d | Skipped: %d\n\n",
		result.Total, result.Completed, result.Failed, result.Skipped)

	for _, r := range result.Steps {
		status := string(r.Status)
		switch r.Status {
		case executor.StatusCompleted:
			status = "✓ " + status
		case executor.StatusFailed:
			status = "✗ " + status
		case executor.StatusSkipped:
			status = "⊘ " + status
		}
		fmt.Fprintf(out, "  [%s] %s\n", status, r.Title)
		if r.Error != "" {
			fmt.Fprintf(out, "    Error: %s\n", r.Error)
		}
	}

	if result.FinalOutput != "" {
		fmt.Fprintf(out, "\n--- Final Output ---\n\n%s\n", result.FinalOutput)
	}
	fmt.Fprintln(out)
}

func runFormulaRuns(cmd *cobra.Command, args []string) error {
	_ = args
	records, err := formularun.List("")
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if len(records) == 0 {
		fmt.Fprintln(out, "No saved formula runs found.")
		return nil
	}
	records = filterFormulaRunRecords(records)
	if len(records) == 0 {
		fmt.Fprintln(out, "No matching formula runs found.")
		return nil
	}
	limit := formulaRunsLimit
	if limit <= 0 || limit > len(records) {
		limit = len(records)
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tFORMULA\tSTATUS\tSTARTED\tFINISHED")
	for _, record := range records[:limit] {
		meta := record.Metadata
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", record.ID, meta.Formula, meta.Status, shortTime(meta.StartedAt), shortTime(meta.FinishedAt))
	}
	return w.Flush()
}

func filterFormulaRunRecords(records []formularun.Record) []formularun.Record {
	formulaFilter := strings.TrimSpace(formulaRunsFormula)
	statusFilter := strings.TrimSpace(formulaRunsStatus)
	if formulaFilter == "" && statusFilter == "" {
		return records
	}
	out := make([]formularun.Record, 0, len(records))
	for _, record := range records {
		if formulaFilter != "" && !strings.EqualFold(record.Metadata.Formula, formulaFilter) {
			continue
		}
		if statusFilter != "" && !strings.EqualFold(record.Metadata.Status, statusFilter) {
			continue
		}
		out = append(out, record)
	}
	return out
}

func runFormulaRunOpen(cmd *cobra.Command, args []string) error {
	id := "latest"
	if len(args) > 0 {
		id = args[0]
	}
	record, err := formularun.Resolve("", id)
	if err != nil {
		return err
	}
	var snapshot formulaDashboardSnapshot
	if err := formularun.LoadState(record.Dir, &snapshot); err != nil {
		return fmt.Errorf("load formula run state failed: %w", err)
	}
	dashboard := newFormulaDashboardServerFromSnapshot(snapshot)
	if err := dashboard.start(formulaWebPort); err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Opened formula run: %s\n", record.ID)
	fmt.Fprintf(out, "Web dashboard: http://localhost:%d\n", dashboard.port)
	fmt.Fprintln(out, "Press Ctrl-C to stop the dashboard.")
	waitForFormulaDashboardExit(dashboard)
	return nil
}

func runFormulaRunShow(cmd *cobra.Command, args []string) error {
	id := "latest"
	if len(args) > 0 {
		id = args[0]
	}
	record, err := formularun.Resolve("", id)
	if err != nil {
		return err
	}
	var snapshot formulaDashboardSnapshot
	_ = formularun.LoadState(record.Dir, &snapshot)
	out := cmd.OutOrStdout()
	meta := record.Metadata
	fmt.Fprintf(out, "Run: %s\n", record.ID)
	fmt.Fprintf(out, "Formula: %s\n", meta.Formula)
	fmt.Fprintf(out, "Status: %s\n", meta.Status)
	if meta.Error != "" {
		fmt.Fprintf(out, "Error: %s\n", meta.Error)
	}
	fmt.Fprintf(out, "Started: %s\n", shortTime(meta.StartedAt))
	fmt.Fprintf(out, "Finished: %s\n", shortTime(meta.FinishedAt))
	fmt.Fprintf(out, "Directory: %s\n", record.Dir)
	fmt.Fprintf(out, "Sessions: %s\n", filepath.Join(meta.WorkspaceDir, ".tt", "sessions"))
	if strings.TrimSpace(formulaRunShowStep) != "" {
		return renderFormulaRunStep(out, record, snapshot, formulaRunShowStep)
	}
	if len(snapshot.Steps) > 0 {
		fmt.Fprintln(out, "\nSteps:")
		for _, step := range snapshot.Steps {
			fmt.Fprintf(out, "  [%s] %s (%s)\n", step.Status, step.ID, step.Title)
			if step.Error != "" {
				fmt.Fprintf(out, "    Error: %s\n", step.Error)
			}
		}
	}
	if snapshot.FinalOutput != "" {
		fmt.Fprintf(out, "\n--- Final Output ---\n\n%s\n", snapshot.FinalOutput)
	}
	return nil
}

func renderFormulaRunStep(out io.Writer, record formularun.Record, snapshot formulaDashboardSnapshot, stepID string) error {
	for _, step := range snapshot.Steps {
		if step.ID != stepID {
			continue
		}
		fmt.Fprintf(out, "\nStep: %s\nTitle: %s\nStatus: %s\nAgent: %s\nSession: %s\n", step.ID, step.Title, step.Status, step.Agent, step.Session)
		if step.Error != "" {
			fmt.Fprintf(out, "Error: %s\n", step.Error)
		}
		if step.Output != "" {
			fmt.Fprintf(out, "\n--- Output ---\n\n%s\n", step.Output)
		}
		return nil
	}
	return fmt.Errorf("step %q not found in run %s", stepID, record.ID)
}

func useFormulaSessionsDir(projectRoot string) (func(), error) {
	sessionsDir := filepath.Join(projectRoot, ".tt", "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		return nil, err
	}
	const envName = "PICOCLAW_SESSIONS_DIR"
	prev, ok := os.LookupEnv(envName)
	if err := os.Setenv(envName, sessionsDir); err != nil {
		return nil, err
	}
	return func() {
		if ok {
			_ = os.Setenv(envName, prev)
			return
		}
		_ = os.Unsetenv(envName)
	}, nil
}

func shortTime(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.Local().Format("2006-01-02 15:04:05")
	}
	return value
}
