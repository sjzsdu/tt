package formulacmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/tt/internal/agents"
	"github.com/sjzsdu/tt/internal/formula"
	"github.com/sjzsdu/tt/internal/formula/run"
	"github.com/sjzsdu/tt/internal/formula/ui"
	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
)

func runFormulaRun(cmd *cobra.Command, args []string) error {
	cliVars := parseVars()
	fileName, fileVars, err := loadFormulaFile(formulaFile)
	if err != nil {
		return err
	}
	name := fileName
	positionalArgs := args
	if len(args) > 0 {
		name = args[0]
		positionalArgs = args[1:]
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("formula name is required; pass <name> or set formula in --file")
	}
	if strings.TrimSpace(formulaFile) == "" {
		_, defaultFileVars, _, found, err := loadDefaultFormulaVarsFile(name)
		if err != nil {
			return err
		}
		if found {
			fileVars = mergeFormulaVars(defaultFileVars, fileVars)
		}
	}
	vars := mergeFormulaVars(fileVars, cliVars)

	p := formula.NewParser(getSearchPaths()...)
	f, err := p.LoadByName(name)
	if err != nil {
		return err
	}
	if err := applyFormulaRunPositionalVars(f, positionalArgs, vars); err != nil {
		return err
	}

	if formulaSession == "" {
		formulaSession = "cli:formula"
	}

	resolvedFormula, err := formula.ResolveFormulaByName(context.Background(), name, getSearchPaths(), vars)
	if err != nil {
		return err
	}
	workflow := formula.WorkflowFromFormula(resolvedFormula)

	projectRoot, _ := os.Getwd()
	if formulaDryRun {
		return executeFormulaRecipeRuntime(context.Background(), executeFormulaRuntimeOptions{
			Workflow:            workflow,
			DryRun:              true,
			AllowScripts:        !formulaNoScript,
			ExternalAgentDriver: formulaExternalDriver,
			Workspace:           projectRoot,
			Vars:                vars,
			Out:                 cmd.OutOrStdout(),
		})
	}

	if err := runFormulaPreflight(context.Background(), resolvedFormula, projectRoot, vars); err != nil {
		return err
	}

	if err := run.EnsureWorkspaceState(projectRoot); err != nil {
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

	runSession := uniqueFormulaRunSession(formulaSession, workflow.Name)

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
	if err := validateFormulaAgentConfiguration(rt, workflow, runAgent, formulaModel, runSession); err != nil {
		return err
	}

	out := cmd.OutOrStdout()

	var runStore *run.Store
	if !formulaNoSave {
		runStore, err = run.NewWithMetadata(formulaDefaultRunDir(loaded), workflow, vars, runAgent, formulaModel, runSession, projectRoot, version)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Run ID: %s\n", runStore.Meta.RunID)
		fmt.Fprintf(out, "Saved to: %s\n", runStore.Dir)
	}

	showWeb := formulaWeb && !formulaNoWeb

	var dashboard *formulaDashboardServer
	if runStore != nil || showWeb {
		dashboard = newFormulaDashboardServer(workflow)
		dashboard.state.WorkspaceDir = formulaDashboardWorkspace(projectRoot)
		dashboard.directProcessor = runner
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
		Workflow:            workflow,
		RunStore:            runStore,
		Processor:           runner,
		DefaultAgent:        runAgent,
		DefaultModel:        formulaModel,
		ExternalAgentDriver: formulaExternalDriver,
		Session:             runSession,
		Workspace:           agentWorkspace,
		Vars:                vars,
		Debug:               formulaDebug,
		DryRun:              formulaDryRun,
		AllowScripts:        !formulaNoScript,
		Dashboard:           dashboard,
		Out:                 out,
	})
	if showWeb {
		fmt.Fprintf(out, "\nWeb dashboard: http://localhost:%d\n", dashboard.port)
		if runCtx.Err() == nil {
			fmt.Fprintln(out, "Press Ctrl-C to stop the dashboard.")
			waitForFormulaDashboardExit(dashboard)
		}
	}
	return err
}

func executeFormulaResume(cmd *cobra.Command, workflowName string, runStore *run.Store, dashboard *formulaDashboardServer, vars map[string]string, initialResults []ui.ResumeStepResult, initialContext map[string]string) error {
	return executeFormulaResumeWithAdvice(cmd, workflowName, runStore, dashboard, vars, initialResults, initialContext, nil)
}

func executeFormulaResumeWithAdvice(cmd *cobra.Command, workflowName string, runStore *run.Store, dashboard *formulaDashboardServer, vars map[string]string, initialResults []ui.ResumeStepResult, initialContext map[string]string, stepAdvice map[string]string) error {
	if len(stepAdvice) == 0 {
		return executeFormulaResumeRuntime(cmd, workflowName, runStore, dashboard, initialResults, initialContext, nil)
	}
	return executeFormulaResumeRuntime(cmd, workflowName, runStore, dashboard, initialResults, initialContext, stepAdvice)

}
func executeFormulaResumeRuntime(cmd *cobra.Command, workflowName string, runStore *run.Store, dashboard *formulaDashboardServer, initialResults []ui.ResumeStepResult, initialContext map[string]string, stepAdvice map[string]string) error {
	projectRoot := strings.TrimSpace(runStore.Meta.WorkspaceDir)
	if projectRoot == "" {
		projectRoot, _ = os.Getwd()
	}
	if err := run.EnsureWorkspaceState(projectRoot); err != nil {
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
	workflow, err := formula.CompileWorkflowByName(context.Background(), workflowName, getSearchPaths(), runStore.Meta.Vars)
	if err != nil {
		return err
	}
	if err := run.ValidateWorkflowDefinition(runStore.Meta, workflow); err != nil {
		return err
	}
	if err := validateFormulaAgentConfiguration(rt, workflow, defaultAgent, runStore.Meta.Model, runStore.Meta.Session); err != nil {
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
		Workflow:     workflow,
		RunStore:     runStore,
		AgentRunner:  agentRunner,
		Workspace:    projectRoot,
		Vars:         runStore.Meta.Vars,
		AllowScripts: !formulaNoScript,
		RunID:        runStore.Meta.RunID,
	})
	if err != nil {
		return err
	}
	seedFormulaRuntimeResumeState(exec, initialResults, initialContext)
	if dashboard != nil {
		exec.Events = formulaRuntimeDashboardEventSink{dashboard: dashboard, workflow: exec.Workflow, session: runStore.Meta.Session, workspace: agentWorkspace}
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Resuming formula run with typed runtime: %s\n", runStore.Meta.RunID)
	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	result, err := exec.Run(runCtx)
	if runCtx.Err() != nil && err == nil {
		err = runCtx.Err()
	}
	renderFormulaRuntimeResult(cmd, workflow, result, err != nil)
	status := run.StatusCompleted
	errMsg := ""
	if result != nil && result.Status == "waiting" {
		status = run.StatusWaitingInput
		fmt.Fprintln(out, "Formula paused: waiting for human input")
	} else if runCtx.Err() != nil {
		status = run.StatusInterrupted
		errMsg = runCtx.Err().Error()
	} else if err != nil {
		status = run.StatusFailed
		errMsg = err.Error()
	}
	if status != run.StatusWaitingInput {
		_ = runStore.Finish(status, errMsg)
	}
	if dashboard != nil {
		_ = dashboard.persistSnapshot()
	}
	if status == run.StatusWaitingInput {
		return nil
	}
	return err
}
