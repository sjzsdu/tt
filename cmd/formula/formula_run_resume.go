package formulacmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/tt/internal/executor"
	"github.com/sjzsdu/tt/internal/formula"
	"github.com/sjzsdu/tt/internal/formularun"
)

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
