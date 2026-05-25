package formulacmd

import (
	"github.com/spf13/cobra"

	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
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

var formulaCmdConfigured bool

func init() {
	New(Dependencies{})
}

func New(deps Dependencies) *cobra.Command {
	configureDependencies(deps)
	if formulaCmdConfigured {
		return formulaCmd
	}
	formulaCmdConfigured = true
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

	return formulaCmd
}
