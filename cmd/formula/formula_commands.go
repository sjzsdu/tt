package formulacmd

import (
	"github.com/spf13/cobra"

	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
)

const formulaLong = `Formula templates define structured task workflows with variables,
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
and picoclaw-configured agents.`

func New(deps Dependencies) *cobra.Command {
	configureDependencies(deps)
	app := &App{}

	formulaCmd := &cobra.Command{
		Use:   "formula",
		Short: "Manage and instantiate formula templates",
		Long:  formulaLong,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			app.installOptions()
		},
	}
	formulaCmd.PersistentFlags().StringVarP(&app.opts.Dir, "dir", "d", "", "formula search directory (default: .tt/formulas, ~/.tt/formulas)")
	formulaCmd.PersistentFlags().StringArrayVar(&app.opts.Vars, "var", nil, "variable override (key=value, repeatable)")

	formulaListCmd := app.newFormulaListCmd()
	formulaShowCmd := app.newFormulaShowCmd()
	formulaCompileCmd := app.newFormulaCompileCmd()
	formulaInstantiateCmd := app.newFormulaInstantiateCmd()
	formulaValidateCmd := newFormulaValidateCmd()
	formulaCopyCmd := newFormulaCopyCmd()
	formulaCreateCmd := app.newFormulaCreateCmd()
	formulaOptimizeCmd := app.newFormulaOptimizeCmd()
	formulaRunCmd := app.newFormulaRunCmd()
	formulaRunsCmd := app.newFormulaRunsCmd()

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

func (a *App) newFormulaListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available formulas",
		Args:  cobra.NoArgs,
		RunE:  runFormulaList,
	}
	cmd.Flags().BoolVar(&a.opts.ListBuiltin, "builtin", false, "show only builtin formulas")
	cmd.Flags().BoolVar(&a.opts.ListUser, "user", false, "show only user formulas from search paths")
	cmd.Flags().StringVar(&a.opts.ListCategory, "category", "", "filter formulas by category")
	return cmd
}

func (a *App) newFormulaShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show [name]",
		Short: "Show formula details",
		Long: `Show formula details. Without a name, lists all formulas.
With --markdown and no name, generates a combined Markdown preview of all formulas.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runFormulaShow,
	}
	cmd.Flags().BoolVar(&a.opts.Markdown, "markdown", false, "render formula as Markdown with Mermaid diagram and preview in browser")
	cmd.Flags().IntVarP(&a.opts.Port, "port", "p", 9598, "web server port for --markdown preview")
	return cmd
}

func (a *App) newFormulaCompileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compile <name>",
		Short: "Compile a formula and show the recipe",
		Args:  cobra.ExactArgs(1),
		RunE:  runFormulaCompile,
	}
	cmd.Flags().BoolVar(&a.opts.CompileWorkflow, "workflow", false, "print graph-first typed Workflow IR instead of Recipe")
	return cmd
}

func (a *App) newFormulaInstantiateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "instantiate <name>",
		Short: "Instantiate a formula into a task tree",
		Args:  cobra.ExactArgs(1),
		RunE:  runFormulaInstantiate,
	}
	cmd.Flags().StringVarP(&a.opts.Output, "output", "o", "json", "output format: json, yaml, text, prompt")
	cmd.Flags().StringVarP(&a.opts.Title, "title", "t", "", "override root task title")
	return cmd
}

func newFormulaValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <file>",
		Short: "Validate a formula file",
		Args:  cobra.ExactArgs(1),
		RunE:  runFormulaValidate,
	}
}

func newFormulaCopyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "copy <name> [output]",
		Short: "Copy a builtin formula to a local TOML file",
		Args:  cobra.RangeArgs(1, 2),
		RunE:  runFormulaCopy,
	}
}

func (a *App) newFormulaCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <name> <prompt...>",
		Short: "Create a formula with the embedded formula-writer agent",
		Long: `Create a formula template from a natural-language prompt.

The embedded formula-writer agent designs the workflow, chooses agent/script
steps, and emits TOML. By default the result is written to .tt/formulas/<name>.toml
or to --output when provided, then validated locally. Use --stdout to print only.`,
		Args: cobra.MinimumNArgs(2),
		RunE: runFormulaCreate,
	}
	cmd.Flags().StringVarP(&a.opts.CreateOutput, "output", "o", "", "output formula file path (default: .tt/formulas/<name>.toml or --dir/<name>.toml)")
	cmd.Flags().BoolVarP(&a.opts.CreateForce, "force", "f", false, "overwrite an existing formula file")
	cmd.Flags().BoolVar(&a.opts.CreateStdout, "stdout", false, "print generated formula instead of writing a file")
	cmd.Flags().StringVar(&a.opts.Model, "model", "", "model override for formula-writer agent")
	cmd.Flags().BoolVar(&a.opts.Debug, "debug", false, "enable debug logging")
	return cmd
}

func (a *App) newFormulaOptimizeCmd() *cobra.Command {
	cmd := &cobra.Command{
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
	cmd.Flags().StringVarP(&a.opts.OptimizeOutput, "output", "o", "", "write optimized formula to this path instead of overwriting the source")
	cmd.Flags().BoolVar(&a.opts.OptimizeStdout, "stdout", false, "print optimized formula instead of writing a file")
	cmd.Flags().BoolVar(&a.opts.OptimizeBuiltin, "built-in", false, "optimize a builtin formula and write result to your formula directory unless --output/--stdout is set")
	cmd.Flags().StringVar(&a.opts.Model, "model", "", "model override for formula-writer agent")
	cmd.Flags().BoolVar(&a.opts.Debug, "debug", false, "enable debug logging")
	return cmd
}

func (a *App) newFormulaRunCmd() *cobra.Command {
	cmd := &cobra.Command{
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
	cmd.Flags().StringVar(&a.opts.Agent, "agent", pcwrap.DefaultAgentID, "default agent for steps without explicit agent config")
	cmd.Flags().StringVar(&a.opts.Model, "model", "", "default model override")
	cmd.Flags().StringVar(&a.opts.Session, "session", "cli:formula", "session key prefix")
	cmd.Flags().BoolVar(&a.opts.Web, "web", true, "show a live web dashboard while the formula runs (default true; kept for compatibility)")
	cmd.Flags().BoolVar(&a.opts.NoWeb, "no-web", false, "do not open or keep a live web dashboard for this formula run")
	cmd.Flags().IntVar(&a.opts.WebPort, "web-port", 9705, "dashboard web server port")
	cmd.Flags().BoolVar(&a.opts.DryRun, "dry-run", false, "print execution plan without running")
	cmd.Flags().BoolVar(&a.opts.Debug, "debug", false, "enable debug logging")
	cmd.Flags().BoolVarP(&a.opts.Verbose, "verbose", "v", false, "show full output of each step")
	cmd.Flags().BoolVar(&a.opts.NoSave, "no-save", false, "do not save formula run state under .tt/runs/formula")
	cmd.Flags().BoolVar(&a.opts.NoScript, "no-script", false, "disable formula script steps for this run")
	cmd.Flags().BoolVar(&a.opts.AllowShell, "allow-shell-script", false, "allow script steps to run through an explicit shell")

	cmd.AddCommand(a.newFormulaRunOpenCmd())
	cmd.AddCommand(a.newFormulaRunShowCmd())
	cmd.AddCommand(a.newFormulaRunRmCmd())
	cmd.AddCommand(a.newFormulaRunResumeCmd())
	cmd.AddCommand(a.newFormulaRunInputCmd())
	return cmd
}

func (a *App) newFormulaRunsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "runs",
		Short: "List saved formula runs",
		Args:  cobra.NoArgs,
		RunE:  runFormulaRuns,
	}
	cmd.Flags().IntVar(&a.opts.RunsLimit, "limit", 20, "maximum number of runs to list")
	cmd.Flags().StringVar(&a.opts.RunsFormula, "formula", "", "filter runs by formula name")
	cmd.Flags().StringVar(&a.opts.RunsStatus, "status", "", "filter runs by status")
	return cmd
}

func (a *App) newFormulaRunOpenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "open [run-id|latest]",
		Short: "Open a saved formula run dashboard",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runFormulaRunOpen,
	}
	cmd.Flags().IntVar(&a.opts.WebPort, "web-port", 9705, "dashboard web server port")
	return cmd
}

func (a *App) newFormulaRunShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show [run-id|latest]",
		Short: "Show a saved formula run",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runFormulaRunShow,
	}
	cmd.Flags().StringVar(&a.opts.RunShowStep, "step", "", "show details for a specific step id")
	return cmd
}

func (a *App) newFormulaRunRmCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rm <run-id>",
		Short: "Delete a saved formula run",
		Args:  cobra.ExactArgs(1),
		RunE:  runFormulaRunRm,
	}
	cmd.Flags().BoolVarP(&a.opts.RunRmYes, "yes", "y", false, "confirm deletion without prompting")
	return cmd
}

func (a *App) newFormulaRunResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume [run-id|latest]",
		Short: "Resume a saved formula run from unfinished steps",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runFormulaRunResume,
	}
}

func (a *App) newFormulaRunInputCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "input [run-id|latest] <step-id>",
		Short: "Submit human input for a waiting formula step and resume the run",
		Args:  cobra.RangeArgs(1, 2),
		RunE:  runFormulaRunInput,
	}
	cmd.Flags().StringArrayVar(&a.opts.InputFields, "field", nil, "human input field value (key=value, repeatable; duplicate keys become arrays)")
	return cmd
}
