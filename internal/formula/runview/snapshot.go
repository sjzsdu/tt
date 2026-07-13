package runview

import (
	"fmt"
	"strings"
	"time"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/ui"
)

func ResolveWaitingInputStepID(snapshot ui.Snapshot, stepID string) (string, error) {
	resolvedStepID, err := ResolveStepID(snapshot, stepID)
	if err != nil {
		return "", err
	}
	for _, step := range snapshot.Steps {
		if step.ID == resolvedStepID && step.Status != ui.StatusWaitingInput {
			return "", fmt.Errorf("step %s is not waiting for input (status: %s)", resolvedStepID, step.Status)
		}
	}
	return resolvedStepID, nil
}

func ResolveStepID(snapshot ui.Snapshot, stepID string) (string, error) {
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

func MarkStepCompletedWithOutput(snapshot *ui.Snapshot, stepID, output string) error {
	if snapshot == nil {
		return fmt.Errorf("snapshot is required")
	}
	for i := range snapshot.Steps {
		if snapshot.Steps[i].ID != stepID {
			continue
		}
		snapshot.Steps[i].Status = ui.StatusCompleted
		snapshot.Steps[i].Output = output
		snapshot.Steps[i].Error = ""
		snapshot.Steps[i].FinishedAt = time.Now().Format(time.RFC3339)
		ui.AppendStepActivity(&snapshot.Steps[i], ui.StepActivity{At: time.Now().Format("15:04:05"), StepID: stepID, Title: snapshot.Steps[i].Title, Status: ui.StatusCompleted, Detail: "Human input submitted", Output: output})
		return nil
	}
	return fmt.Errorf("step %q not found in snapshot", stepID)
}

func BuildResumeState(snapshot ui.Snapshot) ([]ui.ResumeStepResult, map[string]string) {
	return BuildResumeStateExcluding(snapshot, nil)
}

func BuildResumeStateExcluding(snapshot ui.Snapshot, exclude map[string]bool) ([]ui.ResumeStepResult, map[string]string) {
	var results []ui.ResumeStepResult
	ctx := map[string]string{}
	for _, step := range snapshot.Steps {
		if exclude != nil && exclude[step.ID] {
			continue
		}
		status := step.Status
		if status != ui.StatusCompleted && status != ui.StatusSkipped {
			continue
		}
		results = append(results, ui.ResumeStepResult{StepID: step.ID, Title: step.Title, Status: status, Output: step.Output, Error: step.Error})
		if step.Output != "" {
			ctx[step.ID] = step.Output
		}
	}
	return results, ctx
}

func ResetStepForRetry(snapshot *ui.Snapshot, stepID string) {
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

func ResetForResume(snapshot *ui.Snapshot) {
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

func ClearStepAndDownstream(snapshot *ui.Snapshot, stepID string, workflow *ir.Workflow) {
	if snapshot == nil || workflow == nil {
		return
	}
	downstream := findDownstreamSteps(stepID, workflow)
	downstream[stepID] = true
	for i := range snapshot.Steps {
		if downstream[snapshot.Steps[i].ID] {
			snapshot.Steps[i].Status = "pending"
			snapshot.Steps[i].Error = ""
			snapshot.Steps[i].Output = ""
			snapshot.Steps[i].StartedAt = ""
			snapshot.Steps[i].FinishedAt = ""
			snapshot.Steps[i].DurationMS = 0
		}
	}
	snapshot.Status = "running"
	snapshot.Error = ""
}

func findDownstreamSteps(stepID string, workflow *ir.Workflow) map[string]bool {
	out := map[string]bool{}
	if workflow == nil || workflow.Graph.Nodes == nil {
		return out
	}
	children := map[string][]string{}
	for _, edge := range workflow.Graph.Edges {
		children[string(edge.From)] = append(children[string(edge.From)], string(edge.To))
	}
	var visit func(string)
	visit = func(id string) {
		if out[id] {
			return
		}
		out[id] = true
		for _, child := range children[id] {
			visit(child)
		}
	}
	visit(stepID)
	return out
}
