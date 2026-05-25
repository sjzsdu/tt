package cmd

import (
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
