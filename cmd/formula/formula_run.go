package formulacmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/tt/internal/agents"
	"github.com/sjzsdu/tt/internal/formula"
	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/run"
	"github.com/sjzsdu/tt/internal/formula/runtime"
	"github.com/sjzsdu/tt/internal/formula/steps"
	"github.com/sjzsdu/tt/internal/formula/ui"
	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
)

func runFormulaRun(cmd *cobra.Command, args []string) error {
	if formulaMaxConcurrency < 0 || formulaMaxAgentConcurrency < 0 {
		return fmt.Errorf("--max-concurrency and --max-agent-concurrency must be >= 0")
	}
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
			MaxConcurrency:      formulaMaxConcurrency,
			MaxAgentConcurrency: formulaMaxAgentConcurrency,
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

	var resumeFromRun *run.Record
	var startFromStep ir.NodeID
	var rerunSteps map[ir.NodeID]bool
	var injectContext map[string]steps.Value

	if formulaRunFromStep != "" && formulaRunRerunStep != "" {
		return fmt.Errorf("--from-step and --rerun-step cannot be used together")
	}

	if formulaRunFromStep != "" || formulaRunRerunStep != "" {
		if formulaRunID != "" {
			resumeFromRun, err = findRunByID(projectRoot, formulaRunID)
			if err != nil {
				return fmt.Errorf("cannot find run %q: %w", formulaRunID, err)
			}
			if resumeFromRun == nil {
				return fmt.Errorf("run %q not found", formulaRunID)
			}
		} else {
			resumeFromRun, err = findLatestRunForFormula(projectRoot, workflow.Name)
			if err != nil {
				return fmt.Errorf("cannot find previous run for --from-step or --rerun-step: %w", err)
			}
			if resumeFromRun == nil {
				return fmt.Errorf("no previous run found for formula %q; --from-step and --rerun-step require a prior completed run", workflow.Name)
			}
		}
		fmt.Fprintf(out, "Loading prior state from run: %s (status: %s)\n", resumeFromRun.ID, resumeFromRun.Metadata.Status)
	}

	if formulaRunFromStep != "" {
		startFromStep = ir.NodeID(formulaRunFromStep)
	}

	if formulaRunRerunStep != "" {
		rerunSteps = map[ir.NodeID]bool{ir.NodeID(formulaRunRerunStep): true}
	}

	if len(formulaRunInject) > 0 {
		injectContext, err = parseInjectOptions(formulaRunInject)
		if err != nil {
			return fmt.Errorf("invalid --inject option: %w", err)
		}
	}

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
		ResumeFromRun:       resumeFromRun,
		StartFromStep:       startFromStep,
		RerunSteps:          rerunSteps,
		InjectContext:       injectContext,
		MaxConcurrency:      formulaMaxConcurrency,
		MaxAgentConcurrency: formulaMaxAgentConcurrency,
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

func findLatestRunForFormula(projectRoot, formulaName string) (*run.Record, error) {
	root := run.DefaultRoot(projectRoot)
	records, err := run.List(root)
	if err != nil {
		return nil, err
	}
	var candidates []run.Record
	for _, record := range records {
		if record.Metadata.Formula == formulaName && record.Metadata.Status != run.StatusRunning {
			candidates = append(candidates, record)
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Metadata.StartedAt > candidates[j].Metadata.StartedAt
	})
	return &candidates[0], nil
}

func findRunByID(projectRoot, runID string) (*run.Record, error) {
	root := run.DefaultRoot(projectRoot)
	records, err := run.List(root)
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		if record.ID == runID {
			return &record, nil
		}
	}
	return nil, nil
}

func parseInjectOptions(inject []string) (map[string]steps.Value, error) {
	result := map[string]steps.Value{}
	for _, item := range inject {
		stepID, filePath, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(stepID) == "" || strings.TrimSpace(filePath) == "" {
			return nil, fmt.Errorf("invalid inject format: %q, expected step-id=file-path", item)
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("cannot read inject file %q: %w", filePath, err)
		}
		if json.Valid(data) {
			result[stepID] = steps.Value{Type: "json", Raw: data}
		} else {
			result[stepID] = steps.Value{Type: "text", Raw: data}
		}
	}
	return result, nil
}

func seedExecutorFromPreviousRun(exec *runtime.Executor, prevRun *run.Record) error {
	var snapshot runtime.Snapshot
	if err := run.LoadState(prevRun.Dir, &snapshot); err != nil {
		return fmt.Errorf("cannot load previous run state: %w", err)
	}
	for nodeID, state := range snapshot.Steps {
		if state.Status != steps.StatusCompleted || state.Result == nil {
			continue
		}
		state.Result.NormalizeOutputs()
		if err := exec.Store.SaveStep(state); err != nil {
			return fmt.Errorf("cannot save step state for %q: %w", nodeID, err)
		}
		if exec.Context != nil && state.Result != nil {
			if primary, ok := state.Result.PrimaryOutput(); ok {
				if err := exec.Context.Set(string(nodeID), primary); err != nil {
					return fmt.Errorf("cannot set context for %q: %w", nodeID, err)
				}
			}
		}
	}
	return nil
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
		processor:           runner,
		defaultAgent:        defaultAgent,
		defaultModel:        runStore.Meta.Model,
		session:             runStore.Meta.Session,
		workspace:           agentWorkspace,
		debug:               formulaDebug,
		quiet:               true,
		stepAdvice:          stepAdvice,
		isolateStepSessions: effectiveFormulaAgentConcurrency(workflow, formulaMaxConcurrency, formulaMaxAgentConcurrency) > 1,
	}
	exec, err := newFormulaRuntimeExecutor(formulaRuntimeRunOptions{
		Workflow:            workflow,
		RunStore:            runStore,
		AgentRunner:         agentRunner,
		Workspace:           projectRoot,
		Vars:                runStore.Meta.Vars,
		AllowScripts:        !formulaNoScript,
		RunID:               runStore.Meta.RunID,
		MaxConcurrency:      formulaMaxConcurrency,
		MaxAgentConcurrency: formulaMaxAgentConcurrency,
	})
	if err != nil {
		return err
	}
	seedFormulaRuntimeResumeState(exec, initialResults, initialContext)
	if dashboard != nil {
		exec.Events = formulaRuntimeDashboardEventSink{
			dashboard: dashboard, workflow: exec.Workflow, session: runStore.Meta.Session, workspace: agentWorkspace,
			isolateStepSessions: effectiveFormulaAgentConcurrency(workflow, formulaMaxConcurrency, formulaMaxAgentConcurrency) > 1,
		}
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
