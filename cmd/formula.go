package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/tt/internal/agents"
	"github.com/sjzsdu/tt/internal/executor"
	"github.com/sjzsdu/tt/internal/formula"
	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formularun"
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
	formulaRunCmd.Flags().BoolVar(&formulaRuntimeEngine, "runtime-engine", false, "execute with the new typed Workflow runtime engine (experimental)")
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
	formulas := collectFormulaShowAllMarkdownFormulas()
	if len(formulas) == 0 {
		return fmt.Errorf("no formulas found in search paths or builtins")
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

func collectFormulaShowAllMarkdownFormulas() []*formula.Formula {
	paths := getSearchPaths()

	var formulas []*formula.Formula
	seen := make(map[string]bool)
	p := formula.NewParser(paths...)

	if entries, err := formula.BuiltinFormulas(); err == nil {
		for _, entry := range entries {
			f, err := p.LoadByName(entry.Name)
			if err != nil {
				continue
			}
			resolved, err := p.Resolve(f)
			if err != nil {
				continue
			}
			formulas = append(formulas, resolved)
			seen[resolved.Formula] = true
		}
	}

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

	return formulas
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
	stepCount, loopBodyCount := formulaAuthoredStepCounts(f.Steps)
	b.WriteString(fmt.Sprintf("- **Steps:** `%d`\n", stepCount))
	if loopBodyCount > 0 {
		b.WriteString(fmt.Sprintf("- **Loop body steps:** `%d`\n", loopBodyCount))
	}
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
	displayIndex := 1
	for _, step := range recipe.Steps {
		if step.IsRoot || isGeneratedBoundaryRecipeStep(step) {
			continue
		}
		priority := ""
		if step.Priority != nil {
			priority = fmt.Sprintf(" [P%d]", *step.Priority)
		}
		b.WriteString(fmt.Sprintf("### %d. `%s`%s\n\n", displayIndex, step.ID, priority))
		displayIndex++
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

		if step.Loop != nil {
			b.WriteString(generateLoopMarkdown(step))
		}
	}

	return b.String()
}

func formulaAuthoredStepCounts(steps []*formula.Step) (int, int) {
	stepCount := 0
	loopBodyCount := 0
	var walk func([]*formula.Step)
	walk = func(items []*formula.Step) {
		for _, step := range items {
			if step == nil {
				continue
			}
			stepCount++
			if step.Loop != nil {
				loopBodyCount += len(step.Loop.Body)
			}
			walk(step.Children)
		}
	}
	walk(steps)
	return stepCount, loopBodyCount
}

func generateLoopMarkdown(step formula.RecipeStep) string {
	if step.Loop == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("#### Runtime Loop\n\n")
	if summary := loopSummary(step.Loop); summary != "" {
		b.WriteString(fmt.Sprintf("- **Control:** %s\n", summary))
	}
	if step.Condition != "" {
		b.WriteString(fmt.Sprintf("- **Step condition:** `%s`\n", step.Condition))
	}
	b.WriteString(fmt.Sprintf("- **Body steps:** `%d`\n\n", len(step.Loop.Body)))

	if len(step.Loop.Body) == 0 {
		return b.String()
	}
	b.WriteString("| # | Body Step | Title | Input | Output | Condition | Agent |\n")
	b.WriteString("|---|-----------|-------|-------|--------|-----------|-------|\n")
	for i, body := range step.Loop.Body {
		if body == nil {
			continue
		}
		input := "-"
		if len(body.InputCtx) > 0 {
			input = "`" + strings.Join(body.InputCtx, "`, `") + "`"
		}
		output := "-"
		if body.OutputKey != "" {
			output = "`" + body.OutputKey + "`"
		}
		condition := "-"
		if body.Condition != "" {
			condition = "`" + body.Condition + "`"
		}
		agent := "default"
		if body.Agent != nil && body.Agent.Name != "" {
			agent = "`" + body.Agent.Name + "`"
		}
		b.WriteString(fmt.Sprintf("| %d | `%s` | %s | %s | %s | %s | %s |\n",
			i+1,
			markdownCell(body.ID),
			markdownCell(body.Title),
			markdownCell(input),
			markdownCell(output),
			markdownCell(condition),
			markdownCell(agent),
		))
	}
	b.WriteString("\n")
	b.WriteString("> During execution, each loop body result is saved as `parent.iterN.body`, for example `")
	b.WriteString(step.ID)
	b.WriteString(".iter1.<body>`.\n\n")
	return b.String()
}

func generateMermaidGraph(recipe *formula.Recipe) string {
	var b strings.Builder
	b.WriteString("graph TD\n")

	parallelSteps := findParallelSteps(recipe)
	stepByID := recipeStepMap(recipe)
	depths := computeStepDepths(recipe)
	maxDepth := 1
	for _, d := range depths {
		if d > maxDepth {
			maxDepth = d
		}
	}

	for _, step := range recipe.Steps {
		if step.IsRoot || isGeneratedBoundaryRecipeStep(step) {
			continue
		}
		nodeID := mermaidNodeID(step.ID)
		label := mermaidLabel(step)
		shape := mermaidShape(step, nil, parallelSteps)
		b.WriteString(fmt.Sprintf("    %s%s%s\n", nodeID, shape.open, label+shape.close))

		depth := depths[step.ID]
		color := depthColor(depth, maxDepth)

		if isMermaidBoundaryStep(step) {
			b.WriteString(fmt.Sprintf("    class %s nodeBoundary\n", nodeID))
		} else if step.Gate != nil {
			b.WriteString(fmt.Sprintf("    class %s nodeGate\n", nodeID))
		} else if step.Condition != "" {
			b.WriteString(fmt.Sprintf("    class %s nodeCondition\n", nodeID))
		} else if step.Loop != nil {
			b.WriteString(fmt.Sprintf("    class %s nodeLoop\n", nodeID))
		} else {
			b.WriteString(fmt.Sprintf("    classDef c%s fill:%s,stroke:%s,stroke-width:2px\n", nodeID, color.Fill, color.Stroke))
			b.WriteString(fmt.Sprintf("    class %s c%s\n", nodeID, nodeID))
		}
	}

	for _, step := range recipe.Steps {
		if step.IsRoot || isGeneratedBoundaryRecipeStep(step) {
			continue
		}
		appendMermaidLoopBody(&b, step)
	}

	b.WriteString("    classDef nodeBoundary fill:#e8eaf6,stroke:#3f51b5,stroke-width:3px\n")
	b.WriteString("    classDef nodeGate fill:#fce4ec,stroke:#c2185b,stroke-width:2px,stroke-dasharray: 5 5\n")
	b.WriteString("    classDef nodeCondition fill:#e1f5fe,stroke:#0277bd,stroke-width:2px\n")
	b.WriteString("    classDef nodeLoop fill:#fff3e0,stroke:#ef6c00,stroke-width:2px\n")
	b.WriteString("    classDef nodeLoopBody fill:#fff8e1,stroke:#f9a825,stroke-width:1px,stroke-dasharray: 3 3\n")

	for _, dep := range recipe.Deps {
		if dep.Type == "parent-child" {
			continue
		}
		from := mermaidNodeID(dep.DependsOnID)
		to := mermaidNodeID(dep.StepID)
		if dep.StepID == "" || stepByID[dep.StepID].ID == "" || stepByID[dep.StepID].IsRoot || isGeneratedBoundaryRecipeStep(stepByID[dep.StepID]) {
			continue
		}
		if dep.DependsOnID == "" || stepByID[dep.DependsOnID].ID == "" || stepByID[dep.DependsOnID].IsRoot || isGeneratedBoundaryRecipeStep(stepByID[dep.DependsOnID]) {
			continue
		}
		edgeStyle := " -->"
		if dep.Type == "waits-for" {
			edgeStyle = " -.-> |wait|"
		}
		b.WriteString(fmt.Sprintf("    %s%s %s\n", from, edgeStyle, to))
	}

	return b.String()
}

func recipeStepMap(recipe *formula.Recipe) map[string]formula.RecipeStep {
	steps := make(map[string]formula.RecipeStep, len(recipe.Steps))
	for _, step := range recipe.Steps {
		steps[step.ID] = step
	}
	return steps
}

func realRecipeSteps(recipe *formula.Recipe) []formula.RecipeStep {
	steps := make([]formula.RecipeStep, 0, len(recipe.Steps))
	for _, step := range recipe.Steps {
		if !step.IsRoot {
			steps = append(steps, step)
		}
	}
	return steps
}

func isMermaidBoundaryStep(step formula.RecipeStep) bool {
	return step.Metadata != nil && step.Metadata["formula_boundary"] != ""
}

func isGeneratedBoundaryRecipeStep(step formula.RecipeStep) bool {
	return isMermaidBoundaryStep(step) && step.Execution == "noop"
}

func appendMermaidLoopBody(b *strings.Builder, step formula.RecipeStep) {
	if step.Loop == nil || len(step.Loop.Body) == 0 {
		return
	}

	loopID := mermaidNodeID(step.ID)
	loopBodyGraphID := loopID + "_loop_body"
	b.WriteString(fmt.Sprintf("    subgraph %s[\"loop body: %s\"]\n", loopBodyGraphID, mermaidEscapeLabel(shortStepID(step.ID))))
	b.WriteString("        direction TB\n")
	var previous string
	for i, bodyStep := range step.Loop.Body {
		bodyID := mermaidNodeID(fmt.Sprintf("%s.loop.%d.%s", step.ID, i+1, bodyStep.ID))
		label := mermaidLoopBodyLabel(bodyStep)
		b.WriteString(fmt.Sprintf("        %s[\"%s\"]\n", bodyID, label))
		b.WriteString(fmt.Sprintf("    class %s nodeLoopBody\n", bodyID))
		if i > 0 {
			b.WriteString(fmt.Sprintf("        %s -.-> %s\n", previous, bodyID))
		}
		previous = bodyID
	}
	b.WriteString("    end\n")
	if first := firstLoopBodyNodeID(step); first != "" {
		b.WriteString(fmt.Sprintf("    %s -.-> |iterate| %s\n", loopID, first))
	}
	if previous != "" {
		b.WriteString(fmt.Sprintf("    %s -.-> |%s| %s\n", previous, mermaidLoopEdgeLabel(step.Loop), loopID))
	}
}

func firstLoopBodyNodeID(step formula.RecipeStep) string {
	if step.Loop == nil {
		return ""
	}
	for i, bodyStep := range step.Loop.Body {
		if bodyStep != nil {
			return mermaidNodeID(fmt.Sprintf("%s.loop.%d.%s", step.ID, i+1, bodyStep.ID))
		}
	}
	return ""
}

func mermaidLoopBodyLabel(step *formula.Step) string {
	if step == nil {
		return "loop body"
	}
	parts := []string{fmt.Sprintf("body: %s", mermaidEscapeLabel(step.ID))}
	if step.Title != "" {
		parts = append(parts, mermaidEscapeLabel(step.Title))
	}
	if step.OutputKey != "" {
		parts = append(parts, fmt.Sprintf("out: %s", mermaidEscapeLabel(step.OutputKey)))
	}
	return strings.Join(parts, "<br/>")
}

func mermaidLoopEdgeLabel(loop *formula.LoopSpec) string {
	if loop == nil {
		return "next"
	}
	var parts []string
	if loop.Until != "" {
		parts = append(parts, "until "+loop.Until)
	}
	if loop.Max > 0 {
		parts = append(parts, fmt.Sprintf("max %d", loop.Max))
	}
	if loop.Count > 0 {
		parts = append(parts, fmt.Sprintf("count %d", loop.Count))
	}
	if loop.Range != "" {
		parts = append(parts, "range "+loop.Range)
	}
	if len(parts) == 0 {
		return "next"
	}
	return mermaidEscapeLabel(strings.Join(parts, "; "))
}

func loopSummary(loop *formula.LoopSpec) string {
	if loop == nil {
		return ""
	}
	var parts []string
	if loop.Until != "" {
		parts = append(parts, fmt.Sprintf("until `%s`", loop.Until))
	}
	if loop.Max > 0 {
		parts = append(parts, fmt.Sprintf("max `%d`", loop.Max))
	}
	if loop.Count > 0 {
		parts = append(parts, fmt.Sprintf("count `%d`", loop.Count))
	}
	if loop.Range != "" {
		parts = append(parts, fmt.Sprintf("range `%s`", loop.Range))
	}
	if loop.Var != "" {
		parts = append(parts, fmt.Sprintf("var `%s`", loop.Var))
	}
	return strings.Join(parts, "; ")
}

type shapeDef struct {
	open  string
	close string
}

func mermaidShape(step formula.RecipeStep, endSteps, parallelSteps map[string]bool) shapeDef {
	if step.IsRoot {
		return shapeDef{open: "([\"", close: "\"])"}
	}
	if isMermaidBoundaryStep(step) {
		return shapeDef{open: "([\"", close: "\"])"}
	}
	if step.Gate != nil {
		return shapeDef{open: "{\"", close: "\"}"}
	}
	if step.Condition != "" {
		return shapeDef{open: "{\"", close: "\"}"}
	}
	if endSteps != nil && endSteps[step.ID] {
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

func mermaidLabel(step formula.RecipeStep) string {
	safeTitle := mermaidEscapeLabel(step.Title)
	prefix := ""
	if step.Priority != nil {
		prefix = fmt.Sprintf("[P%d] ", *step.Priority)
	}
	shortID := shortStepID(step.ID)
	parts := []string{fmt.Sprintf("%s: %s%s", mermaidEscapeLabel(shortID), prefix, safeTitle)}
	if step.Condition != "" {
		parts = append(parts, "if: "+mermaidEscapeLabel(step.Condition))
	}
	if step.OutputKey != "" {
		parts = append(parts, "out: "+mermaidEscapeLabel(step.OutputKey))
	}
	if len(step.InputCtx) > 0 {
		parts = append(parts, "in: "+mermaidEscapeLabel(strings.Join(step.InputCtx, ", ")))
	}
	if step.Loop != nil {
		parts = append(parts, "loop: "+mermaidLoopEdgeLabel(step.Loop))
	}
	return strings.Join(parts, "<br/>")
}

func shortStepID(id string) string {
	if idx := strings.LastIndex(id, "."); idx >= 0 {
		return id[idx+1:]
	}
	return id
}

func markdownCell(s string) string {
	s = strings.ReplaceAll(s, "\n", "<br/>")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "|", "\\|")
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func mermaidEscapeLabel(s string) string {
	s = strings.ReplaceAll(s, "\"", "'")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
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
	projectRoot, _ := os.Getwd()
	if err := formularun.EnsureWorkspaceState(projectRoot); err != nil {
		return err
	}
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

	rt, err := pcwrap.Load(pcwrap.Options{
		Home:      merged.Picoclaw.Home,
		Config:    merged.Picoclaw.Config,
		TTConfig:  merged,
		TTSources: loaded.Sources,
	})
	if err != nil {
		return picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	embeddedAgents, err := agents.List()
	if err != nil {
		return fmt.Errorf("list embedded agents failed: %w", err)
	}

	runSession := uniqueFormulaRunSession(formulaSession, recipe.Name)

	runner, err := rt.NewDirectRunner(pcwrap.RunOptions{
		Session:        runSession,
		Model:          formulaModel,
		Debug:          formulaDebug,
		Quiet:          true,
		Workspace:      agentWorkspace,
		EmbeddedAgents: embeddedAgents,
	})
	if err != nil {
		return picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	defer runner.Close()

	runAgent := defaultFormulaAgent(formulaAgent)
	if err := validateFormulaAgentConfiguration(rt, recipe, runAgent, formulaModel, runSession); err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()

	var runStore *formularun.Store
	if !formulaNoSave {
		runStore, err = formularun.NewWithMetadata(resolveFormulaRunDir(loaded), recipe, vars, runAgent, formulaModel, runSession, projectRoot, version)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Run ID: %s\n", runStore.Meta.RunID)
		fmt.Fprintf(out, "Saved to: %s\n", runStore.Dir)
	}

	showWeb := formulaWeb && !formulaNoWeb

	var dashboard *formulaDashboardServer
	if runStore != nil || showWeb {
		dashboard = newFormulaDashboardServer(recipe)
		dashboard.state.WorkspaceDir = formulaDashboardWorkspace(projectRoot)
		if runStore != nil {
			dashboard.attachStore(runStore)
		}
	}
	if showWeb {
		if err := dashboard.start(formulaWebPort); err != nil {
			return err
		}
	}

	if formulaRuntimeEngine {
		runCtx, stopRunSignals := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stopRunSignals()
		err := executeFormulaRecipeRuntime(runCtx, executeFormulaRuntimeOptions{
			Recipe:       recipe,
			RunStore:     runStore,
			Processor:    runner,
			DefaultAgent: runAgent,
			DefaultModel: formulaModel,
			Session:      runSession,
			Workspace:    agentWorkspace,
			Debug:        formulaDebug,
			DryRun:       formulaDryRun,
			AllowScripts: !formulaNoScript,
			Dashboard:    dashboard,
			Out:          out,
		})
		if dashboard != nil {
			_ = dashboard.persistSnapshot()
		}
		if showWeb {
			fmt.Fprintf(out, "\nWeb dashboard: http://localhost:%d\n", dashboard.port)
			fmt.Fprintln(out, "Press Ctrl-C to stop the dashboard.")
			waitForFormulaDashboardExit(dashboard)
		}
		return err
	}

	exec := executor.New(recipe, executor.RunOptions{
		Vars:         vars,
		Agent:        runAgent,
		Model:        formulaModel,
		Session:      runSession,
		DryRun:       formulaDryRun,
		Debug:        formulaDebug,
		AllowScripts: !formulaNoScript,
		AllowShell:   formulaAllowShell,
		OnStepUpdate: func(result executor.StepResult) {
			if runStore != nil {
				switch result.Status {
				case executor.StatusCompleted:
					_ = runStore.SaveStepOutput(result.StepID, result.Output)
				case executor.StatusFailed:
					_ = runStore.SaveStepError(result.StepID, result.Error)
					if result.Output != "" {
						_ = runStore.SaveStepOutput(result.StepID, result.Output)
					}
				case executor.StatusWaitingInput:
					_ = runStore.SaveStepHumanInputRequest(result.StepID, result.HumanInputRequest)
				}
			}
			if dashboard == nil {
				return
			}
			switch result.Status {
			case executor.StatusRunning:
				dashboard.markStepRunning(result.StepID, result.Title, "script", "", "")
			case executor.StatusCompleted:
				dashboard.markStepCompleted(result.StepID, result.Output)
			case executor.StatusFailed:
				dashboard.markStepFailed(result.StepID, result.Error, result.Output)
			case executor.StatusWaitingInput:
				dashboard.markStepWaitingInput(result.StepID, result.Title, result.HumanInputRequest)
			}
		},
	})

	stepRunner := func(ctx context.Context, step *formula.RecipeStep, prompt string) (string, error) {
		agent := step.Agent
		if agent == nil || agent.Name == "" {
			agent = &formula.AgentConfig{Name: runAgent, Model: formulaModel}
		}

		sessionKey := fmt.Sprintf("agent:%s:%s:%s", agent.Name, runSession, step.ID)
		if agent.Session != "" {
			sessionKey = fmt.Sprintf("agent:%s:%s:%s:%s", agent.Name, runSession, agent.Session, step.ID)
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

		if executor.ParseHumanInputRequest(resp) != nil {
			logLine("  ⏸ Waiting for human input requested by agent")
			if runStore != nil {
				_ = runStore.SaveStepOutput(step.ID, resp)
			}
			return resp, nil
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
	var waitingErr executor.WaitingInputError
	waitingForInput := errors.As(err, &waitingErr)
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
		if waitingForInput {
			status = formularun.StatusWaitingInput
			_ = runStore.MarkWaitingInput(waitingErr.StepID)
		} else if runCtx.Err() != nil {
			status = formularun.StatusInterrupted
			errMsg = runCtx.Err().Error()
		} else if err != nil {
			status = formularun.StatusFailed
			errMsg = err.Error()
		}
		if status != formularun.StatusWaitingInput {
			_ = runStore.Finish(status, errMsg)
		}
		if dashboard != nil {
			_ = dashboard.persistSnapshot()
		}
	}
	if waitingForInput {
		fmt.Fprintf(out, "Formula paused: waiting for human input at step %s\n", waitingErr.StepID)
	}

	if showWeb {
		fmt.Fprintf(out, "\nWeb dashboard: http://localhost:%d\n", dashboard.port)
		fmt.Fprintln(out, "Press Ctrl-C to stop the dashboard.")
		waitForFormulaDashboardExit(dashboard)
	}

	if waitingForInput {
		return nil
	}
	if err != nil {
		return err
	}
	return nil
}

func executeFormulaRecipe(cmd *cobra.Command, recipe *formula.Recipe, runStore *formularun.Store, dashboard *formulaDashboardServer, vars map[string]string, initialResults []executor.StepResult, initialContext map[string]string) error {
	return executeFormulaRecipeWithAdvice(cmd, recipe, runStore, dashboard, vars, initialResults, initialContext, nil)
}

func executeFormulaRecipeWithAdvice(cmd *cobra.Command, recipe *formula.Recipe, runStore *formularun.Store, dashboard *formulaDashboardServer, vars map[string]string, initialResults []executor.StepResult, initialContext map[string]string, stepAdvice map[string]string) error {
	loaded, err := loadTTConfig()
	if err != nil {
		return err
	}
	merged := loaded.Merged
	projectRoot := strings.TrimSpace(runStore.Meta.WorkspaceDir)
	if projectRoot == "" {
		projectRoot, _ = os.Getwd()
	}
	if err := formularun.EnsureWorkspaceState(projectRoot); err != nil {
		return err
	}
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
	embeddedAgents, err := agents.List()
	if err != nil {
		return fmt.Errorf("list embedded agents failed: %w", err)
	}
	defaultAgent := defaultFormulaAgent(runStore.Meta.Agent)
	if err := validateFormulaAgentConfiguration(rt, recipe, defaultAgent, runStore.Meta.Model, runStore.Meta.Session); err != nil {
		return err
	}
	runner, err := rt.NewDirectRunner(pcwrap.RunOptions{Session: runStore.Meta.Session, Model: runStore.Meta.Model, Debug: formulaDebug, Quiet: true, Workspace: agentWorkspace, EmbeddedAgents: embeddedAgents})
	if err != nil {
		return picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	defer runner.Close()

	exec := executor.New(recipe, executor.RunOptions{
		Vars:           vars,
		InitialResults: initialResults,
		InitialContext: initialContext,
		Agent:          runStore.Meta.Agent,
		Model:          runStore.Meta.Model,
		Session:        runStore.Meta.Session,
		Debug:          formulaDebug,
		AllowScripts:   !formulaNoScript,
		AllowShell:     formulaAllowShell,
		StepAdvice:     stepAdvice,
		OnStepUpdate: func(result executor.StepResult) {
			if runStore != nil {
				switch result.Status {
				case executor.StatusCompleted:
					_ = runStore.SaveStepOutput(result.StepID, result.Output)
				case executor.StatusFailed:
					_ = runStore.SaveStepError(result.StepID, result.Error)
					if result.Output != "" {
						_ = runStore.SaveStepOutput(result.StepID, result.Output)
					}
				case executor.StatusWaitingInput:
					_ = runStore.SaveStepHumanInputRequest(result.StepID, result.HumanInputRequest)
				}
			}
			if dashboard == nil {
				return
			}
			switch result.Status {
			case executor.StatusRunning:
				dashboard.markStepRunning(result.StepID, result.Title, "script", "", "")
			case executor.StatusCompleted:
				dashboard.markStepCompleted(result.StepID, result.Output)
			case executor.StatusFailed:
				dashboard.markStepFailed(result.StepID, result.Error, result.Output)
			case executor.StatusWaitingInput:
				dashboard.markStepWaitingInput(result.StepID, result.Title, result.HumanInputRequest)
			}
		},
	})
	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()

	stepRunner := func(ctx context.Context, step *formula.RecipeStep, prompt string) (string, error) {
		agent := step.Agent
		if agent == nil || agent.Name == "" {
			agent = &formula.AgentConfig{Name: defaultAgent, Model: runStore.Meta.Model}
		}
		sessionKey := fmt.Sprintf("agent:%s:%s:%s", agent.Name, runStore.Meta.Session, step.ID)
		if agent.Session != "" {
			sessionKey = fmt.Sprintf("agent:%s:%s:%s:%s", agent.Name, runStore.Meta.Session, agent.Session, step.ID)
		}
		model := agent.Model
		if model == "" {
			model = runStore.Meta.Model
		}
		logLine := func(format string, args ...any) {
			line := fmt.Sprintf(format, args...)
			fmt.Fprintln(errOut, line)
			if dashboard != nil {
				dashboard.logf("%s", line)
			}
		}
		fmt.Fprintln(errOut)
		logLine("▶ Resuming: %s", step.Title)
		prompt = renderFormulaPrompt(projectRoot, prompt)
		_ = runStore.SaveStepPrompt(step.ID, prompt)
		if dashboard != nil {
			dashboard.markStepRunning(step.ID, step.Title, agent.Name, model, sessionKey)
		}
		started := time.Now()
		_ = runStore.AppendEvent(formularun.Event{Type: "step_started", StepID: step.ID, Agent: agent.Name, Model: model, Session: sessionKey, Status: "running"})
		resp, err := runner.ProcessDirect(pcwrap.RunOptions{Message: prompt, Session: sessionKey, Agent: agent.Name, Model: model})
		resp = strings.TrimSpace(resp)
		if err != nil {
			_ = runStore.SaveStepError(step.ID, err.Error())
			_ = runStore.AppendEvent(formularun.Event{Type: "step_failed", StepID: step.ID, Status: "failed", Error: err.Error(), DurationMS: time.Since(started).Milliseconds()})
			if dashboard != nil {
				dashboard.markStepFailed(step.ID, err.Error(), resp)
			}
			return resp, err
		}
		if executor.ParseHumanInputRequest(resp) != nil {
			logLine("  ⏸ Waiting for human input requested by agent")
			_ = runStore.SaveStepOutput(step.ID, resp)
			return resp, nil
		}
		_ = runStore.SaveStepOutput(step.ID, resp)
		_ = runStore.AppendEvent(formularun.Event{Type: "step_completed", StepID: step.ID, Status: "completed", DurationMS: time.Since(started).Milliseconds()})
		if dashboard != nil {
			dashboard.markStepCompleted(step.ID, resp)
		}
		return resp, nil
	}

	fmt.Fprintf(out, "Resuming formula run: %s\n", runStore.Meta.RunID)
	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	result, err := exec.Run(runCtx, stepRunner)
	var waitingErr executor.WaitingInputError
	waitingForInput := errors.As(err, &waitingErr)
	if runCtx.Err() != nil && err == nil {
		err = runCtx.Err()
	}
	renderRunResult(cmd, result, err != nil)
	if dashboard != nil {
		dashboard.finalize(result, err)
	}
	status := formularun.StatusCompleted
	errMsg := ""
	if waitingForInput {
		status = formularun.StatusWaitingInput
		_ = runStore.MarkWaitingInput(waitingErr.StepID)
		fmt.Fprintf(out, "Formula paused: waiting for human input at step %s\n", waitingErr.StepID)
	} else if runCtx.Err() != nil {
		status = formularun.StatusInterrupted
		errMsg = runCtx.Err().Error()
	} else if err != nil {
		status = formularun.StatusFailed
		errMsg = err.Error()
	}
	if status != formularun.StatusWaitingInput {
		_ = runStore.Finish(status, errMsg)
	}
	if dashboard != nil {
		_ = dashboard.persistSnapshot()
	}
	if waitingForInput {
		return nil
	}
	return err
}

func runFormulaDryRun(recipe *formula.Recipe) error {
	fmt.Printf("Execution Plan for: %s\n\n", recipe.Name)

	batches, err := executor.TopologicalBatches(recipe)
	if err != nil {
		return err
	}

	displayBatch := 1
	for _, batch := range batches {
		var visible []*formula.RecipeStep
		for _, step := range batch {
			if !step.IsRoot {
				visible = append(visible, step)
			}
		}
		if len(visible) == 0 {
			continue
		}
		fmt.Printf("Batch %d (parallel):\n", displayBatch)
		displayBatch++
		for _, step := range visible {
			agent := "default"
			if step.Execution == "noop" {
				agent = "noop"
			} else if step.Execution == "script" {
				agent = "script"
			}
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
	fmt.Fprintf(out, "Total: %d | Completed: %d | Failed: %d | Skipped: %d | Waiting input: %d\n\n",
		result.Total, result.Completed, result.Failed, result.Skipped, result.WaitingInput)

	for _, r := range result.Steps {
		status := string(r.Status)
		switch r.Status {
		case executor.StatusCompleted:
			status = "✓ " + status
		case executor.StatusFailed:
			status = "✗ " + status
		case executor.StatusSkipped:
			status = "⊘ " + status
		case executor.StatusWaitingInput:
			status = "⏸ " + status
		}
		fmt.Fprintf(out, "  [%s] %s\n", status, r.Title)
		if r.HumanInputRequest != nil && r.HumanInputRequest.Reason != "" {
			fmt.Fprintf(out, "    Waiting reason: %s\n", r.HumanInputRequest.Reason)
		}
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
	recipe, err := formularun.LoadRecipe(record.Dir)
	if err != nil {
		return err
	}
	snapshot, err := loadFormulaRunSnapshot(record.Dir, recipe)
	if err != nil {
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
	recipe, _ := formularun.LoadRecipe(record.Dir)
	snapshot, _ := loadFormulaRunSnapshot(record.Dir, recipe)
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
	if meta.PID != 0 {
		fmt.Fprintf(out, "PID: %d\n", meta.PID)
	}
	if meta.TTVersion != "" {
		fmt.Fprintf(out, "tt Version: %s\n", meta.TTVersion)
	}
	if meta.GitBranch != "" || meta.GitCommit != "" {
		dirty := "clean"
		if meta.GitDirty {
			dirty = "dirty"
		}
		fmt.Fprintf(out, "Git: %s %s (%s)\n", meta.GitBranch, meta.GitCommit, dirty)
	}
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

func runFormulaRunRm(cmd *cobra.Command, args []string) error {
	if !formulaRunRmYes {
		return fmt.Errorf("refusing to delete formula run %q without --yes", args[0])
	}
	record, err := formularun.Delete("", args[0])
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Deleted formula run: %s\n", record.ID)
	return nil
}

func runFormulaRunResume(cmd *cobra.Command, args []string) error {
	id := "latest"
	if len(args) > 0 {
		id = args[0]
	}
	record, err := formularun.Resolve("", id)
	if err != nil {
		return err
	}
	recipe, err := formularun.LoadRecipe(record.Dir)
	if err != nil {
		return err
	}
	snapshot, err := loadFormulaRunSnapshot(record.Dir, recipe)
	if err != nil {
		return fmt.Errorf("load formula run state failed: %w", err)
	}
	initialResults, initialContext := buildResumeState(recipe, snapshot)
	store := &formularun.Store{Root: filepath.Dir(record.Dir), Dir: record.Dir, Meta: record.Metadata}
	store.Meta.Status = formularun.StatusRunning
	store.Meta.Error = ""
	store.Meta.FinishedAt = ""
	store.Meta.PID = os.Getpid()
	store.Meta.TTVersion = version
	_ = store.SaveMetadata()
	_ = store.AppendEvent(formularun.Event{Type: "run_resumed", Status: formularun.StatusRunning})
	resetSnapshotForResume(&snapshot)
	dashboard := newFormulaDashboardServerFromSnapshot(snapshot)
	dashboard.readonly = false
	dashboard.attachStore(store)
	return executeFormulaRecipe(cmd, recipe, store, dashboard, record.Metadata.Vars, initialResults, initialContext)
}

func runFormulaRunInput(cmd *cobra.Command, args []string) error {
	id := "latest"
	stepID := ""
	if len(args) == 1 {
		stepID = args[0]
	} else {
		id = args[0]
		stepID = args[1]
	}
	record, err := formularun.Resolve("", id)
	if err != nil {
		return err
	}
	if record.Metadata.Status != formularun.StatusWaitingInput {
		return fmt.Errorf("formula run %s is not waiting for input (status: %s)", record.ID, record.Metadata.Status)
	}
	recipe, err := formularun.LoadRecipe(record.Dir)
	if err != nil {
		return err
	}
	snapshot, err := loadFormulaRunSnapshot(record.Dir, recipe)
	if err != nil {
		return fmt.Errorf("load formula run state failed: %w", err)
	}
	resolvedStepID, err := resolveFormulaRunStepID(snapshot, stepID)
	if err != nil {
		return err
	}
	store := &formularun.Store{Root: filepath.Dir(record.Dir), Dir: record.Dir, Meta: record.Metadata}
	var request executor.HumanInputRequest
	if err := store.LoadStepHumanInputRequest(resolvedStepID, &request); err != nil {
		return fmt.Errorf("load human input request for step %s failed: %w", resolvedStepID, err)
	}
	response, err := parseHumanInputFields(formulaInputFields)
	if err != nil {
		return err
	}
	if err := validateHumanInputResponse(&request, response); err != nil {
		return err
	}
	outputBytes, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return err
	}
	output := string(outputBytes)
	if err := store.SaveStepHumanInputResponse(resolvedStepID, response); err != nil {
		return err
	}
	if err := store.SaveStepOutput(resolvedStepID, output); err != nil {
		return err
	}
	if err := markSnapshotStepCompletedWithOutput(&snapshot, resolvedStepID, output); err != nil {
		return err
	}
	snapshot.Status = "running"
	snapshot.Error = ""
	if err := store.SaveState(snapshot); err != nil {
		return err
	}
	if err := store.AppendEvent(formularun.Event{Type: "human_input_submitted", StepID: resolvedStepID, Status: "completed"}); err != nil {
		return err
	}
	initialResults, initialContext := buildResumeState(recipe, snapshot)
	store.Meta.Status = formularun.StatusRunning
	store.Meta.Error = ""
	store.Meta.FinishedAt = ""
	store.Meta.PID = os.Getpid()
	store.Meta.TTVersion = version
	_ = store.SaveMetadata()
	_ = store.AppendEvent(formularun.Event{Type: "run_resumed", Status: formularun.StatusRunning})
	resetSnapshotForResume(&snapshot)
	dashboard := newFormulaDashboardServerFromSnapshot(snapshot)
	dashboard.readonly = false
	dashboard.attachStore(store)
	fmt.Fprintf(cmd.OutOrStdout(), "Submitted human input for step %s\n", resolvedStepID)
	return executeFormulaRecipe(cmd, recipe, store, dashboard, record.Metadata.Vars, initialResults, initialContext)
}

func parseHumanInputFields(fields []string) (map[string]any, error) {
	if len(fields) == 0 {
		return nil, fmt.Errorf("at least one --field key=value is required")
	}
	response := map[string]any{}
	for _, raw := range fields {
		key, value, ok := strings.Cut(raw, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid --field %q, expected key=value", raw)
		}
		if existing, exists := response[key]; exists {
			switch vals := existing.(type) {
			case []string:
				response[key] = append(vals, value)
			case string:
				response[key] = []string{vals, value}
			default:
				response[key] = []string{fmt.Sprint(vals), value}
			}
		} else {
			response[key] = value
		}
	}
	return response, nil
}

func validateHumanInputResponse(request *executor.HumanInputRequest, response map[string]any) error {
	if request == nil || request.Form == nil {
		return nil
	}
	fields := map[string]*formula.FormField{}
	for _, field := range request.Form.Fields {
		if field == nil || strings.TrimSpace(field.Name) == "" {
			continue
		}
		fields[field.Name] = field
		if field.Required {
			value, ok := response[field.Name]
			if !ok || isEmptyHumanInputValue(value) {
				return fmt.Errorf("required field %q is missing", field.Name)
			}
		}
	}
	for name := range response {
		if _, ok := fields[name]; !ok && len(fields) > 0 {
			return fmt.Errorf("unknown field %q for this human input request", name)
		}
	}
	return nil
}

func isEmptyHumanInputValue(value any) bool {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v) == ""
	case []string:
		return len(v) == 0
	default:
		return value == nil
	}
}

func resolveFormulaRunStepID(snapshot formulaDashboardSnapshot, stepID string) (string, error) {
	resolvedStepID, err := resolveFormulaDashboardStepID(snapshot, stepID)
	if err != nil {
		return "", err
	}
	for _, step := range snapshot.Steps {
		if step.ID == resolvedStepID && step.Status != string(executor.StatusWaitingInput) {
			return "", fmt.Errorf("step %s is not waiting for input (status: %s)", resolvedStepID, step.Status)
		}
	}
	return resolvedStepID, nil
}

func resolveFormulaDashboardStepID(snapshot formulaDashboardSnapshot, stepID string) (string, error) {
	stepID = strings.TrimSpace(stepID)
	if stepID == "" {
		return "", fmt.Errorf("step id is required")
	}
	var matches []string
	for _, step := range snapshot.Steps {
		if step.ID == stepID || shortStepID(step.ID) == stepID || strings.HasSuffix(step.ID, "."+stepID) {
			matches = append(matches, step.ID)
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("step %q not found in run", stepID)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("step %q is ambiguous: %s", stepID, strings.Join(matches, ", "))
	}
	return matches[0], nil
}

func markSnapshotStepCompletedWithOutput(snapshot *formulaDashboardSnapshot, stepID, output string) error {
	if snapshot == nil {
		return fmt.Errorf("snapshot is required")
	}
	for i := range snapshot.Steps {
		if snapshot.Steps[i].ID != stepID {
			continue
		}
		snapshot.Steps[i].Status = string(executor.StatusCompleted)
		snapshot.Steps[i].Output = output
		snapshot.Steps[i].Error = ""
		snapshot.Steps[i].FinishedAt = time.Now().Format(time.RFC3339)
		appendStepActivity(&snapshot.Steps[i], formulaStepActivity{At: time.Now().Format("15:04:05"), StepID: stepID, Title: snapshot.Steps[i].Title, Status: string(executor.StatusCompleted), Detail: "Human input submitted", Output: output})
		return nil
	}
	return fmt.Errorf("step %q not found in snapshot", stepID)
}

func buildResumeState(recipe *formula.Recipe, snapshot formulaDashboardSnapshot) ([]executor.StepResult, map[string]string) {
	return buildResumeStateExcluding(recipe, snapshot, nil)
}

func buildResumeStateExcluding(recipe *formula.Recipe, snapshot formulaDashboardSnapshot, exclude map[string]bool) ([]executor.StepResult, map[string]string) {
	stepByID := map[string]*formula.RecipeStep{}
	for i := range recipe.Steps {
		stepByID[recipe.Steps[i].ID] = &recipe.Steps[i]
	}
	var results []executor.StepResult
	ctx := map[string]string{}
	for _, step := range snapshot.Steps {
		if exclude != nil && exclude[step.ID] {
			continue
		}
		status := executor.StepStatus(step.Status)
		if status != executor.StatusCompleted && status != executor.StatusSkipped {
			continue
		}
		results = append(results, executor.StepResult{StepID: step.ID, Title: step.Title, Status: status, Output: step.Output, Error: step.Error})
		if recipeStep := stepByID[step.ID]; recipeStep != nil && recipeStep.OutputKey != "" && step.Output != "" {
			ctx[recipeStep.OutputKey] = step.Output
		}
	}
	return results, ctx
}

func resetSnapshotStepForRetry(snapshot *formulaDashboardSnapshot, stepID string) {
	if snapshot == nil {
		return
	}
	for i := range snapshot.Steps {
		if snapshot.Steps[i].ID != stepID {
			continue
		}
		snapshot.Steps[i].Status = "pending"
		snapshot.Steps[i].Error = ""
		snapshot.Steps[i].Output = ""
		snapshot.Steps[i].StartedAt = ""
		snapshot.Steps[i].FinishedAt = ""
		snapshot.Steps[i].DurationMS = 0
		return
	}
}

func resetSnapshotForResume(snapshot *formulaDashboardSnapshot) {
	if snapshot == nil {
		return
	}
	snapshot.Status = "running"
	snapshot.Error = ""
	for i := range snapshot.Steps {
		if snapshot.Steps[i].Status == "completed" || snapshot.Steps[i].Status == "skipped" {
			continue
		}
		snapshot.Steps[i].Status = "pending"
		snapshot.Steps[i].Error = ""
		snapshot.Steps[i].FinishedAt = ""
		snapshot.Steps[i].DurationMS = 0
	}
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
		printArtifactPath(out, "Prompt", formularun.StepArtifactPath(record.Dir, step.ID, "prompt.md"))
		printArtifactPath(out, "Output file", formularun.StepArtifactPath(record.Dir, step.ID, "output.md"))
		printArtifactPath(out, "Error file", formularun.StepArtifactPath(record.Dir, step.ID, "error.txt"))
		if step.Output != "" {
			fmt.Fprintf(out, "\n--- Output ---\n\n%s\n", step.Output)
		}
		return nil
	}
	return fmt.Errorf("step %q not found in run %s", stepID, record.ID)
}

func printArtifactPath(out io.Writer, label, path string) {
	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(out, "%s: %s\n", label, path)
	}
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
