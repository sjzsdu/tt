package formulacmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"text/tabwriter"
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

	if formulaDryRun && (formulaLegacyEngine || !formulaRuntimeEngine) {
		return runFormulaDryRun(recipe)
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
	errOut := cmd.ErrOrStderr()

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

	if formulaRuntimeEngine && !formulaLegacyEngine {
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
		// Do not persist the legacy dashboard snapshot here. The runtime engine owns
		// state.json through FormulaRunStateStore, and open/show/resume can read that
		// runtime snapshot directly.
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
