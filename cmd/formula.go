package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/tt/internal/agents"
	"github.com/sjzsdu/tt/internal/formula"
	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/molecule"
	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	ttconfig "github.com/sjzsdu/tt/internal/ttconfig"
)

var (
	formulaDir             string
	formulaVars            []string
	formulaOutput          string
	formulaTitle           string
	formulaMarkdown        bool
	formulaPort            int
	formulaAgent           string
	formulaModel           string
	formulaSession         string
	formulaWeb             bool
	formulaNoWeb           bool
	formulaWebPort         int
	formulaDryRun          bool
	formulaDebug           bool
	formulaVerbose         bool
	formulaNoSave          bool
	formulaNoScript        bool
	formulaAllowShell      bool
	formulaRuntimeEngine   bool
	formulaLegacyEngine    bool
	formulaCreateOutput    string
	formulaCreateForce     bool
	formulaCreateStdout    bool
	formulaOptimizeOutput  string
	formulaOptimizeStdout  bool
	formulaOptimizeBuiltin bool
	formulaRunsLimit       int
	formulaRunsFormula     string
	formulaRunsStatus      string
	formulaRunShowStep     string
	formulaRunRmYes        bool
	formulaInputFields     []string
	formulaListBuiltin     bool
	formulaListUser        bool
	formulaListCategory    string
	formulaCompileWorkflow bool
	formulaRunSessionSeq   uint64
)

var formulaCmd = &cobra.Command{
	Use:   "formula",
	Short: "Manage and instantiate formula templates",
	Long: `Formula templates define structured task workflows with variables,
dependencies, and control flow. Compile and instantiate formulas to generate
task trees for complex work.

Runtime choice:
  A step can write structured output with output_key, then later steps can use
  condition to choose a branch while the formula is running.

    [[steps]]
    id = "decide"
    title = "Decide path"
    description = "Output JSON: {\"path\":\"frontend\"} or {\"path\":\"backend\"}."
    output_key = "decision"

    [[steps]]
    id = "frontend-plan"
    title = "Frontend branch"
    depends_on = ["decide"]
    input_context = ["decision"]
    condition = "decision.path == frontend"

    [[steps]]
    id = "backend-plan"
    title = "Backend branch"
    depends_on = ["decide"]
    input_context = ["decision"]
    condition = "decision.path == backend"

Runtime loop:
  A loop step can run body steps repeatedly and stop based on the latest agent
  output saved through output_key.

    [[steps]]
    id = "improve"
    title = "Improve until approved"
    depends_on = ["frontend-plan"]
    condition = "decision.path == frontend"

      [steps.loop]
      until = "review.approved == true"
      max = 3

      [[steps.loop.body]]
      id = "draft"
      title = "Draft iteration {{iteration}}"
      output_key = "draft"

      [[steps.loop.body]]
      id = "review"
      title = "Review iteration {{iteration}}"
      input_context = ["draft"]
      description = "Output JSON: {\"approved\":true} or {\"approved\":false}."
      output_key = "review"

Start/end flow:
  Compiled recipes always contain real start and end boundary steps. If you do
  not define them, tt inserts noop steps named <formula>.start and <formula>.end.
  All entry steps depend on start, and all terminal steps converge into end.
  You may explicitly define start/end when you want custom agent work there.

    [[steps]]
    id = "start"
    title = "Initialize run context"
    output_key = "run_context"

    [[steps]]
    id = "end"
    title = "Summarize final outcome"
    depends_on = ["frontend-plan", "backend-plan", "improve"]
    input_context = ["decision", "draft", "review"]
    output_key = "final_summary"

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

var formulaCopyCmd = &cobra.Command{
	Use:   "copy <name> [output]",
	Short: "Copy a builtin formula to a local TOML file",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runFormulaCopy,
}

var formulaCreateCmd = &cobra.Command{
	Use:   "create <name> <prompt...>",
	Short: "Create a formula with the embedded formula-writer agent",
	Long: `Create a formula template from a natural-language prompt.

The embedded formula-writer agent designs the workflow, chooses agent/script
steps, and emits TOML. By default the result is written to .tt/formulas/<name>.toml
or to --output when provided, then validated locally. Use --stdout to print only.`,
	Args: cobra.MinimumNArgs(2),
	RunE: runFormulaCreate,
}

var formulaOptimizeCmd = &cobra.Command{
	Use:   "optimize <name> <suggestion...>",
	Short: "Optimize an existing formula with the embedded formula-writer agent",
	Long: `Optimize an existing formula from natural-language suggestions.

The command first resolves <name> from the formula search paths. If the formula
does not exist, it fails immediately and does not call the agent. Otherwise it
sends the current formula TOML plus your suggestions to the embedded
formula-writer agent, writes the improved formula back to the source file by
default, and validates the result locally. Use --stdout to preview only.`,
	Args: cobra.MinimumNArgs(2),
	RunE: runFormulaOptimize,
}

var formulaRunCmd = &cobra.Command{
	Use:   "run <name> [required-var-value]",
	Short: "Execute a formula with picoclaw agents",
	Long: `Execute a formula by running each step through the configured agent.
Steps are executed in dependency order, with parallel steps running concurrently.

Runtime control notes:
  - Steps without explicit agent config use picoclaw agent "main" by default.
  - condition expressions are evaluated at runtime against saved output_key data.
  - loop.until expressions are evaluated after each loop iteration.
  - start/end boundary steps are real recipe steps; generated boundaries are noop.`,
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

var formulaRunRmCmd = &cobra.Command{
	Use:   "rm <run-id>",
	Short: "Delete a saved formula run",
	Args:  cobra.ExactArgs(1),
	RunE:  runFormulaRunRm,
}

var formulaRunResumeCmd = &cobra.Command{
	Use:   "resume [run-id|latest]",
	Short: "Resume a saved formula run from unfinished steps",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runFormulaRunResume,
}

var formulaRunInputCmd = &cobra.Command{
	Use:   "input [run-id|latest] <step-id>",
	Short: "Submit human input for a waiting formula step and resume the run",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runFormulaRunInput,
}

func init() {
	formulaCmd.PersistentFlags().StringVarP(&formulaDir, "dir", "d", "", "formula search directory (default: .tt/formulas, ~/.tt/formulas)")
	formulaCmd.PersistentFlags().StringArrayVar(&formulaVars, "var", nil, "variable override (key=value, repeatable)")

	formulaCompileCmd.Flags().BoolVar(&formulaCompileWorkflow, "workflow", false, "print graph-first typed Workflow IR instead of legacy recipe")
	formulaInstantiateCmd.Flags().StringVarP(&formulaOutput, "output", "o", "json", "output format: json, yaml, text, prompt")
	formulaInstantiateCmd.Flags().StringVarP(&formulaTitle, "title", "t", "", "override root task title")
	formulaCreateCmd.Flags().StringVarP(&formulaCreateOutput, "output", "o", "", "output formula file path (default: .tt/formulas/<name>.toml or --dir/<name>.toml)")
	formulaCreateCmd.Flags().BoolVarP(&formulaCreateForce, "force", "f", false, "overwrite an existing formula file")
	formulaCreateCmd.Flags().BoolVar(&formulaCreateStdout, "stdout", false, "print generated formula instead of writing a file")
	formulaCreateCmd.Flags().StringVar(&formulaModel, "model", "", "model override for formula-writer agent")
	formulaCreateCmd.Flags().BoolVar(&formulaDebug, "debug", false, "enable debug logging")
	formulaOptimizeCmd.Flags().StringVarP(&formulaOptimizeOutput, "output", "o", "", "write optimized formula to this path instead of overwriting the source")
	formulaOptimizeCmd.Flags().BoolVar(&formulaOptimizeStdout, "stdout", false, "print optimized formula instead of writing a file")
	formulaOptimizeCmd.Flags().BoolVar(&formulaOptimizeBuiltin, "built-in", false, "optimize a builtin formula and write result to your formula directory unless --output/--stdout is set")
	formulaOptimizeCmd.Flags().StringVar(&formulaModel, "model", "", "model override for formula-writer agent")
	formulaOptimizeCmd.Flags().BoolVar(&formulaDebug, "debug", false, "enable debug logging")

	formulaShowCmd.Flags().BoolVar(&formulaMarkdown, "markdown", false, "render formula as Markdown with Mermaid diagram and preview in browser")
	formulaShowCmd.Flags().IntVarP(&formulaPort, "port", "p", 9598, "web server port for --markdown preview")
	formulaListCmd.Flags().BoolVar(&formulaListBuiltin, "builtin", false, "show only builtin formulas")
	formulaListCmd.Flags().BoolVar(&formulaListUser, "user", false, "show only user formulas from search paths")
	formulaListCmd.Flags().StringVar(&formulaListCategory, "category", "", "filter formulas by category")

	formulaRunCmd.Flags().StringVar(&formulaAgent, "agent", pcwrap.DefaultAgentID, "default agent for steps without explicit agent config")
	formulaRunCmd.Flags().StringVar(&formulaModel, "model", "", "default model override")
	formulaRunCmd.Flags().StringVar(&formulaSession, "session", "cli:formula", "session key prefix")
	formulaRunCmd.Flags().BoolVar(&formulaWeb, "web", true, "show a live web dashboard while the formula runs (default true; kept for compatibility)")
	formulaRunCmd.Flags().BoolVar(&formulaNoWeb, "no-web", false, "do not open or keep a live web dashboard for this formula run")
	formulaRunCmd.Flags().IntVar(&formulaWebPort, "web-port", 9705, "dashboard web server port")
	formulaRunCmd.Flags().BoolVar(&formulaDryRun, "dry-run", false, "print execution plan without running")
	formulaRunCmd.Flags().BoolVar(&formulaDebug, "debug", false, "enable debug logging")
	formulaRunCmd.Flags().BoolVarP(&formulaVerbose, "verbose", "v", false, "show full output of each step")
	formulaRunCmd.Flags().BoolVar(&formulaNoSave, "no-save", false, "do not save formula run state under .tt/runs/formula")
	formulaRunCmd.Flags().BoolVar(&formulaNoScript, "no-script", false, "disable formula script steps for this run")
	formulaRunCmd.Flags().BoolVar(&formulaAllowShell, "allow-shell-script", false, "allow script steps to run through an explicit shell")
	formulaRunCmd.Flags().BoolVar(&formulaRuntimeEngine, "runtime-engine", true, "execute with the typed Workflow runtime engine")
	formulaRunCmd.Flags().BoolVar(&formulaLegacyEngine, "legacy-engine", false, "execute with the legacy formula executor")
	formulaRunsCmd.Flags().IntVar(&formulaRunsLimit, "limit", 20, "maximum number of runs to list")
	formulaRunsCmd.Flags().StringVar(&formulaRunsFormula, "formula", "", "filter runs by formula name")
	formulaRunsCmd.Flags().StringVar(&formulaRunsStatus, "status", "", "filter runs by status")
	formulaRunOpenCmd.Flags().IntVar(&formulaWebPort, "web-port", 9705, "dashboard web server port")
	formulaRunShowCmd.Flags().StringVar(&formulaRunShowStep, "step", "", "show details for a specific step id")
	formulaRunRmCmd.Flags().BoolVarP(&formulaRunRmYes, "yes", "y", false, "confirm deletion without prompting")
	formulaRunInputCmd.Flags().StringArrayVar(&formulaInputFields, "field", nil, "human input field value (key=value, repeatable; duplicate keys become arrays)")
	formulaRunCmd.AddCommand(formulaRunOpenCmd)
	formulaRunCmd.AddCommand(formulaRunShowCmd)
	formulaRunCmd.AddCommand(formulaRunRmCmd)
	formulaRunCmd.AddCommand(formulaRunResumeCmd)
	formulaRunCmd.AddCommand(formulaRunInputCmd)

	formulaCmd.AddCommand(formulaListCmd)
	formulaCmd.AddCommand(formulaShowCmd)
	formulaCmd.AddCommand(formulaCompileCmd)
	formulaCmd.AddCommand(formulaInstantiateCmd)
	formulaCmd.AddCommand(formulaValidateCmd)
	formulaCmd.AddCommand(formulaCopyCmd)
	formulaCmd.AddCommand(formulaCreateCmd)
	formulaCmd.AddCommand(formulaOptimizeCmd)
	formulaCmd.AddCommand(formulaRunCmd)
	formulaCmd.AddCommand(formulaRunsCmd)

	rootCmd.AddCommand(formulaCmd)
}

func getSearchPaths() []string {
	homeDir, _ := os.UserHomeDir()
	return formulaSearchPaths(mustLoadTTConfig(), formulaDir, homeDir)
}

func formulaSearchPaths(loaded ttconfig.Loaded, explicitDir, homeDir string) []string {
	if explicitDir != "" {
		return []string{explicitDir}
	}

	paths := []string{resolveFormulaDir(loaded)}
	if homeDir != "" {
		paths = appendUniquePath(paths, filepath.Join(homeDir, ".tt", "formulas"))
	}
	return paths
}

func appendUniquePath(paths []string, path string) []string {
	clean := filepath.Clean(path)
	for _, existing := range paths {
		if filepath.Clean(existing) == clean {
			return paths
		}
	}
	return append(paths, clean)
}

func uniqueFormulaRunSession(base, formulaName string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "cli:formula"
	}
	formulaSlug := sessionSlug(formulaName)
	if formulaSlug == "" {
		formulaSlug = "formula"
	}
	seq := atomic.AddUint64(&formulaRunSessionSeq, 1)
	return fmt.Sprintf("%s:%s:%s-%d", base, formulaSlug, time.Now().UTC().Format("20060102T150405.000000000Z"), seq)
}

func sessionSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
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

func defaultFormulaAgent(agent string) string {
	if strings.TrimSpace(agent) == "" {
		return pcwrap.DefaultAgentID
	}
	return agent
}

type formulaAgentRequirement struct {
	Name      string
	StepID    string
	StepTitle string
	Source    string
}

func validateFormulaAgentConfiguration(rt *pcwrap.Runtime, recipe *formula.Recipe, defaultAgent, model, session string) error {
	if rt == nil {
		return fmt.Errorf("picoclaw runtime not loaded")
	}
	requirements := collectFormulaAgentRequirements(recipe, defaultAgent)
	embeddedAgents, err := agents.List()
	if err != nil {
		return fmt.Errorf("list embedded agents failed: %w", err)
	}
	availableConfigured := uniqueSortedStrings(rt.Summary().Agents)
	availableEmbedded := embeddedAgentIDs(embeddedAgents)
	for _, req := range requirements {
		_, err := rt.ResolveRunOptions(pcwrap.RunOptions{
			Session:        session,
			Agent:          req.Name,
			Model:          model,
			EmbeddedAgents: embeddedAgents,
		})
		if err != nil {
			return fmt.Errorf("formula agent preflight failed for %s %q (%s): %w\navailable configured agents: %s\navailable embedded agents: %s", req.Source, req.Name, formulaAgentRequirementLabel(req), err, joinOrNone(availableConfigured), joinOrNone(availableEmbedded))
		}
	}
	return nil
}

func collectFormulaAgentRequirements(recipe *formula.Recipe, defaultAgent string) []formulaAgentRequirement {
	seen := map[string]formulaAgentRequirement{}
	add := func(name, stepID, stepTitle, source string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		key := strings.ToLower(name) + "|" + stepID + "|" + source
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = formulaAgentRequirement{Name: name, StepID: stepID, StepTitle: stepTitle, Source: source}
	}
	add(defaultAgent, "", "", "default agent")
	var walkSteps func([]formula.RecipeStep)
	walkSteps = func(steps []formula.RecipeStep) {
		for _, step := range steps {
			if step.IsRoot || step.Execution == "noop" || step.Execution == "script" {
				continue
			}
			if step.Agent != nil && strings.TrimSpace(step.Agent.Name) != "" {
				add(step.Agent.Name, step.ID, step.Title, "step agent")
			}
			if step.Loop != nil {
				for _, body := range step.Loop.Body {
					if body == nil || strings.TrimSpace(body.Execution) == "noop" || strings.TrimSpace(body.Execution) == "script" {
						continue
					}
					if body.Agent != nil && strings.TrimSpace(body.Agent.Name) != "" {
						add(body.Agent.Name, step.ID+".loop."+body.ID, body.Title, "loop body agent")
					}
				}
			}
		}
	}
	if recipe != nil {
		walkSteps(recipe.Steps)
	}
	out := make([]formulaAgentRequirement, 0, len(seen))
	for _, req := range seen {
		out = append(out, req)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		if out[i].StepID != out[j].StepID {
			return out[i].StepID < out[j].StepID
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func formulaAgentRequirementLabel(req formulaAgentRequirement) string {
	if strings.TrimSpace(req.StepID) == "" {
		return "formula default"
	}
	if strings.TrimSpace(req.StepTitle) == "" {
		return req.StepID
	}
	return fmt.Sprintf("%s / %s", req.StepID, req.StepTitle)
}

func embeddedAgentIDs(items []pcwrap.EmbeddedAgent) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if id := strings.TrimSpace(item.ID); id != "" {
			out = append(out, id)
		}
	}
	return uniqueSortedStrings(out)
}

func uniqueSortedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "(none)"
	}
	return strings.Join(values, ", ")
}

func runFormulaList(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	showBuiltin := !formulaListUser
	showUser := !formulaListBuiltin
	category := strings.TrimSpace(formulaListCategory)

	if showBuiltin {
		entries, err := formula.BuiltinFormulas()
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			fmt.Fprintln(out, "BUILTIN")
			for _, entry := range entries {
				if category != "" && entry.Category != category {
					continue
				}
				desc := entry.Description
				if desc == "" {
					desc = "(no description)"
				}
				fmt.Fprintf(out, "  %-22s %-14s %s\n", entry.Name, entry.Category, desc)
			}
			if showUser {
				fmt.Fprintln(out)
			}
		}
	}

	if !showUser {
		return nil
	}

	paths := getSearchPaths()
	if len(paths) == 0 {
		fmt.Fprintln(out, "No formula search paths configured.")
		fmt.Fprintln(out, "Create formulas in .tt/formulas/ or ~/.tt/formulas/")
		return nil
	}

	found := false
	seen := make(map[string]bool)
	fmt.Fprintln(out, "USER")
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
			if seen[formulaName] {
				continue
			}
			path := filepath.Join(dir, name)

			p := formula.NewParser(dir)
			f, err := p.ParseFile(path)
			if err != nil {
				fmt.Printf("  %s (parse error: %v)\n", formulaName, err)
				continue
			}

			desc := f.Description
			if category != "" && f.Category != category {
				continue
			}
			if desc == "" {
				desc = "(no description)"
			}
			fmt.Fprintf(out, "  %-22s %-14s %s\n", formulaName, f.Category, desc)
			seen[formulaName] = true
			found = true
		}
	}

	if !found {
		fmt.Fprintln(out, "No user formulas found.")
		fmt.Fprintln(out, "Create formulas in .tt/formulas/ or ~/.tt/formulas/")
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
	if resolved.Title != "" {
		fmt.Printf("Title: %s\n", resolved.Title)
	}
	if resolved.Description != "" {
		fmt.Printf("Description: %s\n", resolved.Description)
	}
	if resolved.Category != "" {
		fmt.Printf("Category: %s\n", resolved.Category)
	}
	if len(resolved.Tags) > 0 {
		fmt.Printf("Tags: %s\n", strings.Join(resolved.Tags, ", "))
	}
	if resolved.Source != "" {
		fmt.Printf("Source: %s\n", resolved.Source)
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

	if formulaCompileWorkflow {
		workflow := formula.WorkflowFromRecipe(recipe)
		fmt.Printf("Workflow: %s\n", workflow.Name)
		fmt.Printf("Nodes (%d):\n", len(workflow.Graph.Nodes))
		ids := make([]string, 0, len(workflow.Graph.Nodes))
		for id := range workflow.Graph.Nodes {
			ids = append(ids, string(id))
		}
		sort.Strings(ids)
		for _, id := range ids {
			node := workflow.Graph.Nodes[ir.NodeID(id)]
			meta := node.Step.Meta()
			fmt.Printf("  - %-30s kind=%s title=%q\n", id, meta.Kind, meta.Title)
		}
		fmt.Printf("Edges (%d):\n", len(workflow.Graph.Edges))
		for _, edge := range workflow.Graph.Edges {
			fmt.Printf("  - %s -> %s (%s)\n", edge.From, edge.To, edge.Type)
		}
		return nil
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

func runFormulaCopy(cmd *cobra.Command, args []string) error {
	name := strings.TrimSpace(args[0])
	data, ok, err := formula.BuiltinFormulaContent(name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("builtin formula %q not found", name)
	}
	outPath := ""
	if len(args) > 1 {
		outPath = args[1]
	} else {
		outPath = filepath.Join(resolveFormulaDir(mustLoadTTConfig()), name+formula.CanonicalTOMLExt)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(outPath); err == nil {
		return fmt.Errorf("output file already exists: %s", outPath)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Copied builtin formula %q to %s\n", name, outPath)
	return nil
}

func runFormulaCreate(cmd *cobra.Command, args []string) error {
	name := strings.TrimSpace(args[0])
	if name == "" {
		return fmt.Errorf("formula name is required")
	}
	prompt := strings.TrimSpace(strings.Join(args[1:], " "))
	if prompt == "" {
		return fmt.Errorf("formula prompt is required")
	}

	projectRoot, _ := os.Getwd()
	loaded, err := loadTTConfig()
	if err != nil {
		return err
	}
	merged := loaded.Merged
	agentWorkspace := formulaAgentWorkspace(projectRoot)
	_, resolvedHome, resolvedConfig, restoreStorage, err := useTTAgentStorage(merged.Picoclaw.Home, merged.Picoclaw.Config)
	if err != nil {
		return err
	}
	defer restoreStorage()
	merged.Picoclaw.Home = resolvedHome
	merged.Picoclaw.Config = resolvedConfig
	if err := ensurePicoclawConfigAvailable(merged.Picoclaw.Home, merged.Picoclaw.Config); err != nil {
		return err
	}
	rt, err := pcwrap.Load(pcwrap.Options{Home: merged.Picoclaw.Home, Config: merged.Picoclaw.Config, TTConfig: merged, TTSources: loaded.Sources})
	if err != nil {
		return picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	embedded := []pcwrap.EmbeddedAgent{agents.FormulaWriter()}
	session := "cli:formula:create:" + name
	runner, err := rt.NewDirectRunner(pcwrap.RunOptions{Session: session, Agent: agents.FormulaWriterID, Model: formulaModel, Workspace: agentWorkspace, Debug: formulaDebug, Quiet: !formulaDebug, EmbeddedAgents: embedded})
	if err != nil {
		return err
	}
	defer runner.Close()

	message := buildFormulaCreatePrompt(name, prompt)
	loading := startLLMLoading("正在用 formula-writer agent 生成 formula", formulaDebug)
	resp, err := runner.ProcessDirect(pcwrap.RunOptions{Message: message, Session: session, Agent: agents.FormulaWriterID, Model: formulaModel, Workspace: agentWorkspace, Debug: formulaDebug, Quiet: !formulaDebug, EmbeddedAgents: embedded})
	loading.Stop()
	if err != nil {
		return err
	}
	toml := extractFormulaTOML(resp)
	if toml == "" {
		return fmt.Errorf("formula-writer returned empty formula")
	}

	if formulaCreateStdout {
		fmt.Fprintln(cmd.OutOrStdout(), toml)
		return nil
	}

	outPath := formulaCreateOutputPath(name)
	if !formulaCreateForce {
		if _, err := os.Stat(outPath); err == nil {
			return fmt.Errorf("formula file already exists: %s (use --force to overwrite)", outPath)
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(outPath, []byte(toml+"\n"), 0o644); err != nil {
		return err
	}

	p := formula.NewParser()
	f, err := p.ParseFile(outPath)
	if err != nil {
		return fmt.Errorf("generated formula written to %s but failed to parse: %w", outPath, err)
	}
	if err := f.Validate(); err != nil {
		return fmt.Errorf("generated formula written to %s but failed validation: %w", outPath, err)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Created formula: %s\n", outPath)
	fmt.Fprintf(out, "Formula %q is valid.\n", f.Formula)
	fmt.Fprintf(out, "Next: tt formula compile %s --dir %s\n", f.Formula, filepath.Dir(outPath))
	return nil
}

func runFormulaOptimize(cmd *cobra.Command, args []string) error {
	name := strings.TrimSpace(args[0])
	if name == "" {
		return fmt.Errorf("formula name is required")
	}
	suggestion := strings.TrimSpace(strings.Join(args[1:], " "))
	if suggestion == "" {
		return fmt.Errorf("optimization suggestion is required")
	}

	p := formula.NewParser(getSearchPaths()...)
	f, err := p.LoadByName(name)
	if err != nil {
		return fmt.Errorf("formula %q not found: %w", name, err)
	}
	if strings.TrimSpace(f.Source) == "" {
		return fmt.Errorf("formula %q source path is unknown", name)
	}
	if !formula.IsTOMLFilename(f.Source) && !formulaOptimizeBuiltin && strings.TrimSpace(formulaOptimizeOutput) == "" && !formulaOptimizeStdout {
		return fmt.Errorf("formula %q is not a TOML file (%s); use --output <path.toml> or --stdout", name, f.Source)
	}
	var existing []byte
	if strings.HasPrefix(strings.TrimSpace(f.Source), "builtin:") {
		data, ok, err := formula.BuiltinFormulaContent(name)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("builtin formula %q not found", name)
		}
		existing = data
	} else {
		existing, err = os.ReadFile(f.Source)
		if err != nil {
			return fmt.Errorf("read formula %s: %w", f.Source, err)
		}
	}

	projectRoot, _ := os.Getwd()
	loaded, err := loadTTConfig()
	if err != nil {
		return err
	}
	merged := loaded.Merged
	agentWorkspace := formulaAgentWorkspace(projectRoot)
	_, resolvedHome, resolvedConfig, restoreStorage, err := useTTAgentStorage(merged.Picoclaw.Home, merged.Picoclaw.Config)
	if err != nil {
		return err
	}
	defer restoreStorage()
	merged.Picoclaw.Home = resolvedHome
	merged.Picoclaw.Config = resolvedConfig
	if err := ensurePicoclawConfigAvailable(merged.Picoclaw.Home, merged.Picoclaw.Config); err != nil {
		return err
	}
	rt, err := pcwrap.Load(pcwrap.Options{Home: merged.Picoclaw.Home, Config: merged.Picoclaw.Config, TTConfig: merged, TTSources: loaded.Sources})
	if err != nil {
		return picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	embedded := []pcwrap.EmbeddedAgent{agents.FormulaWriter()}
	session := "cli:formula:optimize:" + name
	runner, err := rt.NewDirectRunner(pcwrap.RunOptions{Session: session, Agent: agents.FormulaWriterID, Model: formulaModel, Workspace: agentWorkspace, Debug: formulaDebug, Quiet: !formulaDebug, EmbeddedAgents: embedded})
	if err != nil {
		return err
	}
	defer runner.Close()

	message := buildFormulaOptimizePrompt(name, string(existing), suggestion)
	loading := startLLMLoading("正在用 formula-writer agent 优化 formula", formulaDebug)
	resp, err := runner.ProcessDirect(pcwrap.RunOptions{Message: message, Session: session, Agent: agents.FormulaWriterID, Model: formulaModel, Workspace: agentWorkspace, Debug: formulaDebug, Quiet: !formulaDebug, EmbeddedAgents: embedded})
	loading.Stop()
	if err != nil {
		return err
	}
	toml := extractFormulaTOML(resp)
	if toml == "" {
		return fmt.Errorf("formula-writer returned empty optimized formula")
	}
	optimized, err := validateFormulaTOMLContent(toml)
	if err != nil {
		repairMessage := buildFormulaOptimizeRepairPrompt(name, suggestion, toml, err)
		loading := startLLMLoading("生成结果校验失败，正在让 formula-writer 修复 TOML", formulaDebug)
		resp, repairErr := runner.ProcessDirect(pcwrap.RunOptions{Message: repairMessage, Session: session + ":repair", Agent: agents.FormulaWriterID, Model: formulaModel, Workspace: agentWorkspace, Debug: formulaDebug, Quiet: !formulaDebug, EmbeddedAgents: embedded})
		loading.Stop()
		if repairErr != nil {
			return fmt.Errorf("optimized formula failed validation: %w; repair attempt failed: %w", err, repairErr)
		}
		toml = extractFormulaTOML(resp)
		if toml == "" {
			return fmt.Errorf("optimized formula failed validation: %w; repair attempt returned empty formula", err)
		}
		optimized, err = validateFormulaTOMLContent(toml)
		if err != nil {
			return fmt.Errorf("optimized formula failed validation after repair: %w", err)
		}
	}
	if optimized.Formula != f.Formula {
		return fmt.Errorf("optimized formula changed name from %q to %q", f.Formula, optimized.Formula)
	}

	if formulaOptimizeStdout {
		fmt.Fprintln(cmd.OutOrStdout(), toml)
		return nil
	}
	outPath := strings.TrimSpace(formulaOptimizeOutput)
	if outPath == "" {
		if formula.IsTOMLFilename(f.Source) {
			outPath = f.Source
		} else {
			outPath = filepath.Join(resolveFormulaDir(mustLoadTTConfig()), name+formula.CanonicalTOMLExt)
		}
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(outPath, []byte(toml+"\n"), 0o644); err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Optimized formula: %s\n", outPath)
	fmt.Fprintf(out, "Formula %q is valid.\n", optimized.Formula)
	fmt.Fprintf(out, "Next: tt formula compile %s --dir %s\n", optimized.Formula, filepath.Dir(outPath))
	return nil
}

func buildFormulaCreatePrompt(name, userPrompt string) string {
	return fmt.Sprintf(`Create a tt formula TOML file.

Formula name: %s
User request:
%s

Requirements:
- Output only valid TOML, preferably no Markdown fences.
- Set formula = %q exactly.
- Use version = 1 and type = "workflow".
- Prefer script steps for deterministic context collection or validation.
- Prefer agent steps for reasoning, planning, implementation, review, and reporting.
- Use safe argv-style script commands; avoid shell.
- Add output_key for data consumed downstream.
- Add depends_on and input_context where data flows between steps.
- If a condition or loop depends on agent output, make that step output ONLY compact JSON.
- Use embedded agents where appropriate: coder, planner, tester, product-manager, ui, full-stack, reporter.
`, name, userPrompt, name)
}

func buildFormulaOptimizePrompt(name, currentTOML, suggestion string) string {
	return fmt.Sprintf(`Optimize an existing tt formula TOML file.

Formula name: %s
User optimization request:
%s

Current formula TOML:
---BEGIN TOML---
%s
---END TOML---

Requirements:
- Output only the full optimized TOML, preferably no Markdown fences.
- Preserve formula = %q exactly.
- Preserve the user's existing intent unless the suggestion explicitly changes it.
- Improve step boundaries, data flow, output_key, input_context, conditions, loops, script safety, descriptions, and agent choices where useful.
- Prefer script steps for deterministic context collection or validation.
- Prefer agent steps for reasoning, planning, implementation, review, and reporting.
- Use safe argv-style script commands; avoid shell.
- For agent config, use exactly one TOML style per step: either agent.name = "coder" OR [steps.agent] name = "coder", never both in the same [[steps]].
- Prefer preserving the current file's style. If the current formula uses agent.name = "...", keep using dotted agent.name and do not add [steps.agent] tables.
- Do not remove important variables or steps unless the suggestion asks for simplification.
- Ensure all depends_on references point to existing local step ids.
`, name, suggestion, currentTOML, name)
}

func buildFormulaOptimizeRepairPrompt(name, suggestion, invalidTOML string, validationErr error) string {
	return fmt.Sprintf(`The optimized tt formula TOML failed local validation.

Formula name: %s
Original optimization request:
%s

Validation error:
%v

Invalid TOML to repair:
---BEGIN TOML---
%s
---END TOML---

Return only the full repaired TOML.

Hard requirements:
- Preserve formula = %q exactly.
- Fix the validation error without changing the user's intent.
- Do not mix dotted agent keys and agent tables in the same step. If a step has agent.name = "...", do not also add [steps.agent] for that step.
- Prefer dotted agent.name = "..." style for consistency with the current formula.
- Ensure all TOML tables are valid and every [[steps]] table is closed before starting the next step.
`, name, suggestion, validationErr, invalidTOML, name)
}

func validateFormulaTOMLContent(content string) (*formula.Formula, error) {
	p := formula.NewParser()
	f, err := p.ParseTOML([]byte(content))
	if err != nil {
		return nil, err
	}
	if err := f.Validate(); err != nil {
		return nil, err
	}
	return f, nil
}

func formulaCreateOutputPath(name string) string {
	if strings.TrimSpace(formulaCreateOutput) != "" {
		return formulaCreateOutput
	}
	dir := strings.TrimSpace(formulaDir)
	if dir == "" {
		dir = resolveFormulaDir(mustLoadTTConfig())
	}
	return filepath.Join(dir, name+".toml")
}

func extractFormulaTOML(resp string) string {
	resp = strings.TrimSpace(resp)
	if resp == "" {
		return ""
	}
	for _, fence := range []string{"```toml", "```TOML", "```"} {
		idx := strings.Index(resp, fence)
		if idx < 0 {
			continue
		}
		start := idx + len(fence)
		if nl := strings.Index(resp[start:], "\n"); nl >= 0 {
			start += nl + 1
		}
		end := strings.Index(resp[start:], "```")
		if end >= 0 {
			return strings.TrimSpace(resp[start : start+end])
		}
	}
	return strings.TrimSpace(resp)
}
