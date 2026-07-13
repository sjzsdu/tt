package formulacmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sjzsdu/tt/internal/formula/ui"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/tt/internal/formula"
	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/run"
	"github.com/sjzsdu/tt/internal/formula/runview"
	spec "github.com/sjzsdu/tt/internal/formula/spec"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

func runFormulaRunResume(cmd *cobra.Command, args []string) error {
	id := "latest"
	if len(args) > 0 {
		id = args[0]
	}
	record, err := run.Resolve("", id)
	if err != nil {
		return err
	}
	workflow, err := formula.CompileWorkflowByName(cmd.Context(), record.Metadata.Formula, getSearchPaths(), record.Metadata.Vars)
	if err != nil {
		return err
	}
	snapshot, err := loadFormulaRunSnapshot(record.Dir, workflow)
	if err != nil {
		return fmt.Errorf("load formula run state failed: %w", err)
	}
	exclude := resumeDependencyExclusions(workflow, snapshot)
	initialResults, initialContext := runview.BuildResumeStateExcluding(snapshot, exclude)
	store := &run.Store{Root: filepath.Dir(record.Dir), Dir: record.Dir, Meta: record.Metadata}
	store.Meta.Status = run.StatusRunning
	store.Meta.Error = ""
	store.Meta.FinishedAt = ""
	store.Meta.PID = os.Getpid()
	store.Meta.TTVersion = version
	_ = store.SaveMetadata()
	_ = store.AppendEvent(run.Event{Type: "run_resumed", Status: run.StatusRunning})
	runview.ResetForResume(&snapshot)
	dashboard := newFormulaDashboardServerFromSnapshot(snapshot)
	dashboard.readonly = false
	dashboard.attachStore(store)
	if err := startResumeDashboard(cmd, dashboard); err != nil {
		return err
	}
	return executeFormulaResume(cmd, record.Metadata.Formula, store, dashboard, record.Metadata.Vars, initialResults, initialContext)
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
	record, err := run.Resolve("", id)
	if err != nil {
		return err
	}
	if record.Metadata.Status != run.StatusWaitingInput {
		return fmt.Errorf("formula run %s is not waiting for input (status: %s)", record.ID, record.Metadata.Status)
	}
	workflow, err := formula.CompileWorkflowByName(cmd.Context(), record.Metadata.Formula, getSearchPaths(), record.Metadata.Vars)
	if err != nil {
		return err
	}
	snapshot, err := loadFormulaRunSnapshot(record.Dir, workflow)
	if err != nil {
		return fmt.Errorf("load formula run state failed: %w", err)
	}
	resolvedStepID, err := runview.ResolveWaitingInputStepID(snapshot, stepID)
	if err != nil {
		return err
	}
	store := &run.Store{Root: filepath.Dir(record.Dir), Dir: record.Dir, Meta: record.Metadata}
	var request ui.HumanInputRequest
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
	if err := runview.MarkStepCompletedWithOutput(&snapshot, resolvedStepID, output); err != nil {
		return err
	}
	snapshot.Status = "running"
	snapshot.Error = ""
	if err := store.SaveState(snapshot); err != nil {
		return err
	}
	if err := store.AppendEvent(run.Event{Type: "human_input_submitted", StepID: resolvedStepID, Status: "completed"}); err != nil {
		return err
	}
	exclude := resumeDependencyExclusions(workflow, snapshot)
	initialResults, initialContext := runview.BuildResumeStateExcluding(snapshot, exclude)
	store.Meta.Status = run.StatusRunning
	store.Meta.Error = ""
	store.Meta.FinishedAt = ""
	store.Meta.PID = os.Getpid()
	store.Meta.TTVersion = version
	_ = store.SaveMetadata()
	_ = store.AppendEvent(run.Event{Type: "run_resumed", Status: run.StatusRunning})
	runview.ResetForResume(&snapshot)
	dashboard := newFormulaDashboardServerFromSnapshot(snapshot)
	dashboard.readonly = false
	dashboard.attachStore(store)
	if err := startResumeDashboard(cmd, dashboard); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Submitted human input for step %s\n", resolvedStepID)
	return executeFormulaResume(cmd, record.Metadata.Formula, store, dashboard, record.Metadata.Vars, initialResults, initialContext)
}

func startResumeDashboard(cmd *cobra.Command, dashboard *formulaDashboardServer) error {
	if dashboard == nil || formulaNoWeb {
		return nil
	}
	if err := dashboard.start(formulaWebPort); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Web dashboard: http://localhost:%d\n", dashboard.port)
	return nil
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

func validateHumanInputResponse(request *ui.HumanInputRequest, response map[string]any) error {
	if request == nil || request.Form == nil {
		return nil
	}
	fields := map[string]*spec.FormField{}
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

func resumeDependencyExclusions(workflow *ir.Workflow, snapshot ui.Snapshot) map[string]bool {
	if workflow == nil {
		return nil
	}
	failed := map[string]bool{}
	completed := map[string]bool{}
	for _, step := range snapshot.Steps {
		switch step.Status {
		case "failed":
			failed[step.ID] = true
		case "completed":
			completed[step.ID] = true
		}
	}
	if len(failed) == 0 {
		return nil
	}
	parents := map[string][]string{}
	for _, edge := range workflow.Graph.Edges {
		parents[string(edge.To)] = append(parents[string(edge.To)], string(edge.From))
	}
	exclude := map[string]bool{}
	var visit func(string)
	visit = func(id string) {
		for _, parent := range parents[id] {
			if exclude[parent] {
				continue
			}
			node := workflow.Graph.Nodes[ir.NodeID(parent)]
			if completed[parent] && node != nil && node.Step != nil && node.Step.Meta().Kind == steps.KindLoop {
				exclude[parent] = true
			}
			visit(parent)
		}
	}
	for id := range failed {
		visit(id)
	}
	if len(exclude) == 0 {
		return nil
	}
	return exclude
}

func renderFormulaRunStep(out io.Writer, record run.Record, snapshot ui.Snapshot, stepID string) error {
	for _, step := range snapshot.Steps {
		if step.ID != stepID {
			continue
		}
		fmt.Fprintf(out, "\nStep: %s\nTitle: %s\nStatus: %s\nAgent: %s\nSession: %s\n", step.ID, step.Title, step.Status, step.Agent, step.Session)
		if step.Error != "" {
			fmt.Fprintf(out, "Error: %s\n", step.Error)
		}
		printArtifactPath(out, "Prompt", run.StepArtifactPath(record.Dir, step.ID, "prompt.md"))
		printArtifactPath(out, "Output file", run.StepArtifactPath(record.Dir, step.ID, "output.md"))
		printArtifactPath(out, "Error file", run.StepArtifactPath(record.Dir, step.ID, "error.txt"))
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

func runFormulaRunChat(cmd *cobra.Command, args []string) error {
	id := "latest"
	stepID := ""
	if len(args) == 1 {
		stepID = args[0]
	} else {
		id = args[0]
		stepID = args[1]
	}
	record, err := run.Resolve("", id)
	if err != nil {
		return err
	}
	workflow, err := formula.CompileWorkflowByName(cmd.Context(), record.Metadata.Formula, getSearchPaths(), record.Metadata.Vars)
	if err != nil {
		return err
	}
	snapshot, err := loadFormulaRunSnapshot(record.Dir, workflow)
	if err != nil {
		return fmt.Errorf("load formula run state failed: %w", err)
	}
	resolvedStepID, err := runview.ResolveStepID(snapshot, stepID)
	if err != nil {
		return err
	}
	var stepOutput string
	var stepStatus string
	var isReviewWaiting bool
	for _, step := range snapshot.Steps {
		if step.ID == resolvedStepID {
			stepOutput = step.Output
			stepStatus = step.Status
			if step.Status == ui.StatusWaitingInput && step.HumanInputRequest != nil && step.HumanInputRequest.Reason != "" && strings.Contains(step.HumanInputRequest.Reason, "chat_with_coder") {
				isReviewWaiting = true
			}
			break
		}
	}
	if stepStatus != ui.StatusCompleted && stepStatus != ui.StatusFailed && !isReviewWaiting {
		return fmt.Errorf("step %s is not completed, failed, or waiting for chat (status: %s)", resolvedStepID, stepStatus)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\n--- Step %s Output ---\n\n", resolvedStepID)
	if stepOutput != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n", stepOutput)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "(no output)\n")
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\n--- Enter your feedback ---\n")
	fmt.Fprintf(cmd.OutOrStdout(), "Describe how you want to adjust this step's output.\n")
	fmt.Fprintf(cmd.OutOrStdout(), "Press Enter to skip (continue without changes) or enter feedback and press Ctrl+D.\n\n")
	feedback, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return fmt.Errorf("read feedback failed: %w", err)
	}
	feedbackStr := strings.TrimSpace(string(feedback))
	if feedbackStr == "" {
		if isReviewWaiting {
			if err := runview.MarkStepCompletedWithOutput(&snapshot, resolvedStepID, stepOutput); err != nil {
				return fmt.Errorf("mark step completed failed: %w", err)
			}
			store := &run.Store{Root: filepath.Dir(record.Dir), Dir: record.Dir, Meta: record.Metadata}
			store.Meta.Status = run.StatusRunning
			store.Meta.Error = ""
			store.Meta.FinishedAt = ""
			store.Meta.PID = os.Getpid()
			store.Meta.TTVersion = version
			_ = store.SaveMetadata()
			_ = store.AppendEvent(run.Event{Type: "run_chat_skipped", StepID: resolvedStepID, Status: run.StatusRunning})
			if err := store.SaveState(snapshot); err != nil {
				return fmt.Errorf("save state failed: %w", err)
			}
			dashboard := newFormulaDashboardServerFromSnapshot(snapshot)
			dashboard.readonly = false
			dashboard.attachStore(store)
			if err := startResumeDashboard(cmd, dashboard); err != nil {
				return err
			}
			exclude := resumeDependencyExclusions(workflow, snapshot)
			initialResults, initialContext := runview.BuildResumeStateExcluding(snapshot, exclude)
			fmt.Fprintf(cmd.OutOrStdout(), "Skipping review, continuing workflow...\n")
			return executeFormulaResume(cmd, record.Metadata.Formula, store, dashboard, record.Metadata.Vars, initialResults, initialContext)
		}
		return fmt.Errorf("feedback cannot be empty for re-running a completed/failed step")
	}
	runview.ClearStepAndDownstream(&snapshot, resolvedStepID, workflow)
	stepAdvice := map[string]string{resolvedStepID: feedbackStr}
	store := &run.Store{Root: filepath.Dir(record.Dir), Dir: record.Dir, Meta: record.Metadata}
	store.Meta.Status = run.StatusRunning
	store.Meta.Error = ""
	store.Meta.FinishedAt = ""
	store.Meta.PID = os.Getpid()
	store.Meta.TTVersion = version
	_ = store.SaveMetadata()
	_ = store.AppendEvent(run.Event{Type: "run_chat", StepID: resolvedStepID, Status: run.StatusRunning, Extra: map[string]any{"feedback": feedbackStr}})
	runview.ResetForResume(&snapshot)
	if err := store.SaveState(snapshot); err != nil {
		return fmt.Errorf("save state failed: %w", err)
	}
	dashboard := newFormulaDashboardServerFromSnapshot(snapshot)
	dashboard.readonly = false
	dashboard.attachStore(store)
	if err := startResumeDashboard(cmd, dashboard); err != nil {
		return err
	}
	exclude := resumeDependencyExclusions(workflow, snapshot)
	initialResults, initialContext := runview.BuildResumeStateExcluding(snapshot, exclude)
	fmt.Fprintf(cmd.OutOrStdout(), "Re-running step %s with your feedback...\n", resolvedStepID)
	return executeFormulaResumeWithAdvice(cmd, record.Metadata.Formula, store, dashboard, record.Metadata.Vars, initialResults, initialContext, stepAdvice)
}
