package formulacmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/tt/internal/agents"
	"github.com/sjzsdu/tt/internal/executor"
	"github.com/sjzsdu/tt/internal/formula"
	"github.com/sjzsdu/tt/internal/formularun"
	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
)

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

	workflow, err := formula.CompileWorkflowByName(context.Background(), name, getSearchPaths(), vars)
	if err != nil {
		return err
	}
	// Transitional UI/export data. Runtime execution below uses workflow directly.
	recipe, err := formula.Compile(context.Background(), name, getSearchPaths(), vars)
	if err != nil {
		return err
	}

	if formulaDryRun {
		return executeFormulaRecipeRuntime(context.Background(), executeFormulaRuntimeOptions{
			Recipe:       recipe,
			Workflow:     workflow,
			DryRun:       true,
			AllowScripts: !formulaNoScript,
			Out:          cmd.OutOrStdout(),
		})
	}

	projectRoot, _ := os.Getwd()
	if err := formularun.EnsureWorkspaceState(projectRoot); err != nil {
		return err
	}
	formulaRT, err := newFormulaPicoclawRuntime(projectRoot)
	if err != nil {
		return err
	}
	defer formulaRT.Close()
	loaded := formulaRT.Loaded
	rt := formulaRT.Runtime
	agentWorkspace := formulaRT.Workspace
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
		return formulaRT.UnavailableError(err)
	}
	defer runner.Close()

	runAgent := defaultFormulaAgent(formulaAgent)
	if err := validateFormulaAgentConfiguration(rt, recipe, runAgent, formulaModel, runSession); err != nil {
		return err
	}

	out := cmd.OutOrStdout()

	var runStore *formularun.Store
	if !formulaNoSave {
		runStore, err = formularun.NewWithMetadata(formulaDefaultRunDir(loaded), recipe, vars, runAgent, formulaModel, runSession, projectRoot, version)
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
			if err := runStore.SaveWorkflow(workflow); err != nil {
				return err
			}
			dashboard.attachStore(runStore)
		}
	}
	if showWeb {
		if err := dashboard.start(formulaWebPort); err != nil {
			return err
		}
	}

	runCtx, stopRunSignals := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stopRunSignals()
	err = executeFormulaRecipeRuntime(runCtx, executeFormulaRuntimeOptions{
		Recipe:       recipe,
		Workflow:     workflow,
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
	if showWeb {
		fmt.Fprintf(out, "\nWeb dashboard: http://localhost:%d\n", dashboard.port)
		fmt.Fprintln(out, "Press Ctrl-C to stop the dashboard.")
		waitForFormulaDashboardExit(dashboard)
	}
	return err
}

func executeFormulaResume(cmd *cobra.Command, recipe *formula.Recipe, runStore *formularun.Store, dashboard *formulaDashboardServer, vars map[string]string, initialResults []executor.StepResult, initialContext map[string]string) error {
	return executeFormulaResumeWithAdvice(cmd, recipe, runStore, dashboard, vars, initialResults, initialContext, nil)
}

func executeFormulaResumeWithAdvice(cmd *cobra.Command, recipe *formula.Recipe, runStore *formularun.Store, dashboard *formulaDashboardServer, vars map[string]string, initialResults []executor.StepResult, initialContext map[string]string, stepAdvice map[string]string) error {
	if len(stepAdvice) == 0 {
		return executeFormulaResumeRuntime(cmd, recipe, runStore, dashboard, initialResults, initialContext, nil)
	}
	return executeFormulaResumeRuntime(cmd, recipe, runStore, dashboard, initialResults, initialContext, stepAdvice)

}
func executeFormulaResumeRuntime(cmd *cobra.Command, recipe *formula.Recipe, runStore *formularun.Store, dashboard *formulaDashboardServer, initialResults []executor.StepResult, initialContext map[string]string, stepAdvice map[string]string) error {
	projectRoot := strings.TrimSpace(runStore.Meta.WorkspaceDir)
	if projectRoot == "" {
		projectRoot, _ = os.Getwd()
	}
	if err := formularun.EnsureWorkspaceState(projectRoot); err != nil {
		return err
	}
	formulaRT, err := newFormulaPicoclawRuntime(projectRoot)
	if err != nil {
		return err
	}
	defer formulaRT.Close()
	rt := formulaRT.Runtime
	agentWorkspace := formulaRT.Workspace
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
		return formulaRT.UnavailableError(err)
	}
	defer runner.Close()

	agentRunner := formulaRuntimeAgentRunner{
		processor:    runner,
		defaultAgent: defaultAgent,
		defaultModel: runStore.Meta.Model,
		session:      runStore.Meta.Session,
		workspace:    agentWorkspace,
		debug:        formulaDebug,
		quiet:        true,
		stepAdvice:   stepAdvice,
	}
	exec, err := newFormulaRuntimeExecutor(formulaRuntimeRunOptions{
		Recipe:       recipe,
		RunStore:     runStore,
		AgentRunner:  agentRunner,
		AllowScripts: !formulaNoScript,
	})
	if err != nil {
		return err
	}
	seedFormulaRuntimeResumeState(exec, initialResults, initialContext)
	if dashboard != nil {
		exec.Events = formulaRuntimeDashboardEventSink{dashboard: dashboard, workflow: exec.Workflow}
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Resuming formula run with typed runtime: %s\n", runStore.Meta.RunID)
	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	result, err := exec.Run(runCtx)
	if runCtx.Err() != nil && err == nil {
		err = runCtx.Err()
	}
	renderFormulaRuntimeResult(cmd, recipe, result, err != nil)
	status := formularun.StatusCompleted
	errMsg := ""
	if result != nil && result.Status == "waiting" {
		status = formularun.StatusWaitingInput
		fmt.Fprintln(out, "Formula paused: waiting for human input")
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
	if status == formularun.StatusWaitingInput {
		return nil
	}
	return err
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
