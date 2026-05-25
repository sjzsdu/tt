package formulacmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

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

	recipe, err := formula.Compile(context.Background(), name, getSearchPaths(), vars)
	if err != nil {
		return err
	}

	if formulaDryRun {
		return executeFormulaRecipeRuntime(context.Background(), executeFormulaRuntimeOptions{
			Recipe:       recipe,
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
