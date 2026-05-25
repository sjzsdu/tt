package formularunview

import (
	"fmt"
	"strings"
	"time"

	"github.com/sjzsdu/tt/internal/formulaui"
)

func ResolveWaitingInputStepID(snapshot formulaui.Snapshot, stepID string) (string, error) {
	resolvedStepID, err := ResolveStepID(snapshot, stepID)
	if err != nil {
		return "", err
	}
	for _, step := range snapshot.Steps {
		if step.ID == resolvedStepID && step.Status != formulaui.StatusWaitingInput {
			return "", fmt.Errorf("step %s is not waiting for input (status: %s)", resolvedStepID, step.Status)
		}
	}
	return resolvedStepID, nil
}

func ResolveStepID(snapshot formulaui.Snapshot, stepID string) (string, error) {
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

func MarkStepCompletedWithOutput(snapshot *formulaui.Snapshot, stepID, output string) error {
	if snapshot == nil {
		return fmt.Errorf("snapshot is required")
	}
	for i := range snapshot.Steps {
		if snapshot.Steps[i].ID != stepID {
			continue
		}
		snapshot.Steps[i].Status = formulaui.StatusCompleted
		snapshot.Steps[i].Output = output
		snapshot.Steps[i].Error = ""
		snapshot.Steps[i].FinishedAt = time.Now().Format(time.RFC3339)
		formulaui.AppendStepActivity(&snapshot.Steps[i], formulaui.StepActivity{At: time.Now().Format("15:04:05"), StepID: stepID, Title: snapshot.Steps[i].Title, Status: formulaui.StatusCompleted, Detail: "Human input submitted", Output: output})
		return nil
	}
	return fmt.Errorf("step %q not found in snapshot", stepID)
}

func BuildResumeState(snapshot formulaui.Snapshot) ([]formulaui.ResumeStepResult, map[string]string) {
	return BuildResumeStateExcluding(snapshot, nil)
}

func BuildResumeStateExcluding(snapshot formulaui.Snapshot, exclude map[string]bool) ([]formulaui.ResumeStepResult, map[string]string) {
	var results []formulaui.ResumeStepResult
	ctx := map[string]string{}
	for _, step := range snapshot.Steps {
		if exclude != nil && exclude[step.ID] {
			continue
		}
		status := step.Status
		if status != formulaui.StatusCompleted && status != formulaui.StatusSkipped {
			continue
		}
		results = append(results, formulaui.ResumeStepResult{StepID: step.ID, Title: step.Title, Status: status, Output: step.Output, Error: step.Error})
		if step.Output != "" {
			ctx[step.ID] = step.Output
		}
	}
	return results, ctx
}

func ResetStepForRetry(snapshot *formulaui.Snapshot, stepID string) {
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

func ResetForResume(snapshot *formulaui.Snapshot) {
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

func shortStepID(id string) string {
	if idx := strings.LastIndex(id, "."); idx >= 0 && idx+1 < len(id) {
		return id[idx+1:]
	}
	return id
}
