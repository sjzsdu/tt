package formulacmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/sjzsdu/tt/internal/formula/ui"
)

func (s *formulaDashboardServer) logf(format string, args ...any) {
	s.mu.Lock()
	s.appendLogLocked(fmt.Sprintf(format, args...))
	s.mu.Unlock()
	s.broadcast()
}

func (s *formulaDashboardServer) appendLogLocked(text string) {
	s.state.Logs = append(s.state.Logs, ui.LogEntry{At: time.Now().Format("15:04:05"), Text: text})
	if len(s.state.Logs) > 200 {
		s.state.Logs = append([]ui.LogEntry(nil), s.state.Logs[len(s.state.Logs)-200:]...)
	}
}

func (s *formulaDashboardServer) markWorkflowRunning() {
	s.mu.Lock()
	s.state.Status = "running"
	s.state.Error = ""
	s.appendLogLocked("Workflow started")
	s.mu.Unlock()
	s.broadcast()
}

func (s *formulaDashboardServer) markWorkflowWorkspaceReady(workspace string) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return
	}
	s.mu.Lock()
	s.state.WorkspaceDir = workspace
	if s.state.FinalReportChat != nil {
		s.state.FinalReportChat.Error = ""
	}
	s.appendLogLocked(fmt.Sprintf("Workspace ready: %s", workspace))
	s.mu.Unlock()
	s.broadcast()
}

func (s *formulaDashboardServer) markWorkflowCompleted(finalOutput string) {
	s.mu.Lock()
	s.state.Status = "completed"
	s.state.Error = ""
	s.state.FinalOutput = finalOutput
	if strings.TrimSpace(finalOutput) == "" {
		s.state.FinalReportChat = nil
	} else if s.state.FinalReportChat != nil {
		s.state.FinalReportChat.Error = ""
		if s.state.FinalReportChat.Status == "" {
			s.state.FinalReportChat.Status = "idle"
		}
	}
	s.appendLogLocked("Workflow completed")
	s.mu.Unlock()
	s.broadcast()
}

func (s *formulaDashboardServer) markStepRunning(stepID, title, agent, model, session string) {
	s.mu.Lock()
	found := false
	for i := range s.state.Steps {
		if s.state.Steps[i].ID != stepID {
			continue
		}
		found = true
		if title != "" {
			s.state.Steps[i].Title = title
		}
		s.state.Steps[i].Agent = agent
		s.state.Steps[i].Model = model
		s.state.Steps[i].Session = session
		s.state.Steps[i].Status = "running"
		s.state.Steps[i].StartedAt = time.Now().Format(time.RFC3339)
		s.state.Steps[i].FinishedAt = ""
		s.state.Steps[i].DurationMS = 0
		s.state.Steps[i].Error = ""
		s.state.Steps[i].Output = ""
		ui.AppendStepActivity(&s.state.Steps[i], ui.StepActivity{At: time.Now().Format("15:04:05"), StepID: stepID, Title: title, Status: "running", Session: session, Detail: fmt.Sprintf("Agent %s started this step", agent)})
		s.recordExecutionTransitionLocked(stepID, s.state.Steps[i].Title, "running", session, fmt.Sprintf("Agent %s started this step", agent), "", "", 0)
		s.appendLogLocked(fmt.Sprintf("Step %s started", stepID))
		break
	}
	if !found {
		s.markLoopActivityLocked(stepID, title, "running", session, fmt.Sprintf("Agent %s started loop body", agent), "", "", 0)
		s.appendLogLocked(fmt.Sprintf("Loop step %s started", stepID))
	}
	if s.state.Status == "pending" {
		s.state.Status = "running"
	}
	s.mu.Unlock()
	s.broadcast()
}

func (s *formulaDashboardServer) markStepCompleted(stepID, output string) {
	s.mu.Lock()
	found := false
	for i := range s.state.Steps {
		if s.state.Steps[i].ID != stepID {
			continue
		}
		found = true
		s.state.Steps[i].Status = "completed"
		s.state.Steps[i].Output = output
		s.state.Steps[i].FinishedAt = time.Now().Format(time.RFC3339)
		if s.state.Steps[i].StartedAt != "" {
			if started, err := time.Parse(time.RFC3339, s.state.Steps[i].StartedAt); err == nil {
				s.state.Steps[i].DurationMS = time.Since(started).Milliseconds()
			}
		}
		ui.AppendStepActivity(&s.state.Steps[i], ui.StepActivity{At: time.Now().Format("15:04:05"), StepID: stepID, Title: s.state.Steps[i].Title, Status: "completed", Detail: fmt.Sprintf("Completed with %d chars of output", len(output)), Output: output, DurationMS: s.state.Steps[i].DurationMS})
		s.recordExecutionTransitionLocked(stepID, s.state.Steps[i].Title, "completed", s.state.Steps[i].Session, fmt.Sprintf("Completed with %d chars of output", len(output)), output, "", s.state.Steps[i].DurationMS)
		s.appendLogLocked(fmt.Sprintf("Step %s completed", stepID))
		break
	}
	if !found {
		s.markLoopActivityLocked(stepID, "", "completed", "", fmt.Sprintf("Completed with %d chars of output", len(output)), output, "", 0)
		s.appendLogLocked(fmt.Sprintf("Loop step %s completed", stepID))
	}
	s.mu.Unlock()
	s.broadcast()
}

func (s *formulaDashboardServer) markStepSkipped(stepID, reason string) {
	s.mu.Lock()
	found := false
	for i := range s.state.Steps {
		if s.state.Steps[i].ID != stepID {
			continue
		}
		found = true
		s.state.Steps[i].Status = "skipped"
		s.state.Steps[i].Error = ""
		s.state.Steps[i].FinishedAt = time.Now().Format(time.RFC3339)
		if s.state.Steps[i].StartedAt != "" {
			if started, err := time.Parse(time.RFC3339, s.state.Steps[i].StartedAt); err == nil {
				s.state.Steps[i].DurationMS = time.Since(started).Milliseconds()
			}
		}
		detail := reason
		if detail == "" {
			detail = "Step condition evaluated to false"
		}
		ui.AppendStepActivity(&s.state.Steps[i], ui.StepActivity{At: time.Now().Format("15:04:05"), StepID: stepID, Title: s.state.Steps[i].Title, Status: "skipped", Detail: detail})
		s.recordExecutionTransitionLocked(stepID, s.state.Steps[i].Title, "skipped", s.state.Steps[i].Session, detail, "", "", s.state.Steps[i].DurationMS)
		s.appendLogLocked(fmt.Sprintf("Step %s skipped", stepID))
		break
	}
	if !found {
		detail := reason
		if detail == "" {
			detail = "Loop body condition evaluated to false"
		}
		s.markLoopActivityLocked(stepID, "", "skipped", "", detail, "", "", 0)
		s.appendLogLocked(fmt.Sprintf("Loop step %s skipped", stepID))
	}
	s.mu.Unlock()
	s.broadcast()
}

func (s *formulaDashboardServer) markStepFailed(stepID, errMsg, output string) {
	s.mu.Lock()
	found := false
	for i := range s.state.Steps {
		if s.state.Steps[i].ID != stepID {
			continue
		}
		found = true
		s.state.Steps[i].Status = "failed"
		s.state.Steps[i].Error = errMsg
		s.state.Steps[i].Output = output
		s.state.Steps[i].FinishedAt = time.Now().Format(time.RFC3339)
		if s.state.Steps[i].StartedAt != "" {
			if started, err := time.Parse(time.RFC3339, s.state.Steps[i].StartedAt); err == nil {
				s.state.Steps[i].DurationMS = time.Since(started).Milliseconds()
			}
		}
		ui.AppendStepActivity(&s.state.Steps[i], ui.StepActivity{At: time.Now().Format("15:04:05"), StepID: stepID, Title: s.state.Steps[i].Title, Status: "failed", Detail: errMsg, Output: output, Error: errMsg, DurationMS: s.state.Steps[i].DurationMS})
		s.recordExecutionTransitionLocked(stepID, s.state.Steps[i].Title, "failed", s.state.Steps[i].Session, errMsg, output, errMsg, s.state.Steps[i].DurationMS)
		s.appendLogLocked(fmt.Sprintf("Step %s failed: %s", stepID, errMsg))
		break
	}
	if !found {
		s.markLoopActivityLocked(stepID, "", "failed", "", errMsg, output, errMsg, 0)
		s.appendLogLocked(fmt.Sprintf("Loop step %s failed: %s", stepID, errMsg))
	}
	s.state.Status = "failed"
	s.state.Error = errMsg
	s.mu.Unlock()
	s.broadcast()
}

func (s *formulaDashboardServer) markStepInterrupted(stepID, errMsg, output string) {
	s.mu.Lock()
	found := false
	for i := range s.state.Steps {
		if s.state.Steps[i].ID != stepID {
			continue
		}
		found = true
		s.state.Steps[i].Status = "interrupted"
		s.state.Steps[i].Error = errMsg
		s.state.Steps[i].Output = output
		s.state.Steps[i].FinishedAt = time.Now().Format(time.RFC3339)
		if s.state.Steps[i].StartedAt != "" {
			if started, err := time.Parse(time.RFC3339, s.state.Steps[i].StartedAt); err == nil {
				s.state.Steps[i].DurationMS = time.Since(started).Milliseconds()
			}
		}
		if errMsg == "" {
			errMsg = "step interrupted"
		}
		ui.AppendStepActivity(&s.state.Steps[i], ui.StepActivity{At: time.Now().Format("15:04:05"), StepID: stepID, Title: s.state.Steps[i].Title, Status: "interrupted", Detail: errMsg, Output: output, Error: errMsg, DurationMS: s.state.Steps[i].DurationMS})
		s.recordExecutionTransitionLocked(stepID, s.state.Steps[i].Title, "interrupted", s.state.Steps[i].Session, errMsg, output, errMsg, s.state.Steps[i].DurationMS)
		s.appendLogLocked(fmt.Sprintf("Step %s interrupted: %s", stepID, errMsg))
		break
	}
	if !found {
		if errMsg == "" {
			errMsg = "loop body interrupted"
		}
		s.markLoopActivityLocked(stepID, "", "interrupted", "", errMsg, output, errMsg, 0)
		s.appendLogLocked(fmt.Sprintf("Loop step %s interrupted: %s", stepID, errMsg))
	}
	s.state.Status = "interrupted"
	s.state.Error = errMsg
	s.mu.Unlock()
	s.broadcast()
}

func (s *formulaDashboardServer) markStepWaitingInput(stepID, title string, request *ui.HumanInputRequest) {
	s.mu.Lock()
	found := false
	for i := range s.state.Steps {
		if s.state.Steps[i].ID != stepID {
			continue
		}
		found = true
		if title != "" {
			s.state.Steps[i].Title = title
		}
		s.state.Steps[i].Status = "waiting_input"
		if request != nil {
			s.state.Steps[i].HumanInputRequest = request
		}
		s.state.Steps[i].FinishedAt = ""
		ui.AppendStepActivity(&s.state.Steps[i], ui.StepActivity{At: time.Now().Format("15:04:05"), StepID: stepID, Title: s.state.Steps[i].Title, Status: "waiting_input", Detail: "Waiting for human input"})
		s.recordExecutionTransitionLocked(stepID, s.state.Steps[i].Title, "waiting_input", s.state.Steps[i].Session, "Waiting for human input", "", "", 0)
		s.appendLogLocked(fmt.Sprintf("Step %s waiting for human input", stepID))
		break
	}
	if !found {
		s.markLoopActivityLocked(stepID, title, "waiting_input", "", "Waiting for human input", "", "", 0)
		s.appendLogLocked(fmt.Sprintf("Loop step %s waiting for human input", stepID))
	}
	s.state.Status = "waiting_input"
	s.state.Error = ""
	s.mu.Unlock()
	s.broadcast()
}

func (s *formulaDashboardServer) recordRepair(repair ui.RepairRecord) {
	if strings.TrimSpace(repair.StepID) == "" {
		return
	}
	s.mu.Lock()
	updated := false
	for i := range s.state.Repairs {
		if s.state.Repairs[i].StepID == repair.StepID && s.state.Repairs[i].Attempt == repair.Attempt {
			s.state.Repairs[i] = repair
			updated = true
			break
		}
	}
	if !updated {
		s.state.Repairs = append(s.state.Repairs, repair)
	}
	msg := fmt.Sprintf("Repair %s for %s attempt %d", repair.Status, repair.StepID, repair.Attempt)
	if strings.TrimSpace(repair.Reason) != "" {
		msg += ": " + repair.Reason
	}
	s.appendLogLocked(msg)
	s.mu.Unlock()
	s.broadcast()
}

func (s *formulaDashboardServer) confirmRepair(stepID string, attempt int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Repairs {
		if s.state.Repairs[i].StepID != stepID || s.state.Repairs[i].Attempt != attempt {
			continue
		}
		s.state.Repairs[i].ConfirmedAt = time.Now().Format(time.RFC3339)
		s.state.Repairs[i].ConfirmationStatus = "confirmed"
		s.appendLogLocked(fmt.Sprintf("Repair confirmed for %s attempt %d", stepID, attempt))
		return true
	}
	return false
}

func (s *formulaDashboardServer) markLoopActivityLocked(stepID, title, status, session, detail, output, errMsg string, durationMS int64) {
	parentID := ui.LoopParentStepID(stepID)
	if parentID == "" {
		return
	}
	for i := range s.state.Steps {
		if s.state.Steps[i].ID != parentID {
			continue
		}
		if status == "running" && s.state.Steps[i].Status == "pending" {
			s.state.Steps[i].Status = "running"
			s.state.Steps[i].StartedAt = time.Now().Format(time.RFC3339)
		}
		if status == "failed" || status == "interrupted" || status == "waiting_input" || status == "completed" {
			s.state.Steps[i].Status = status
			if errMsg != "" {
				s.state.Steps[i].Error = errMsg
			}
			if status == "failed" || status == "interrupted" || status == "completed" {
				s.state.Steps[i].FinishedAt = time.Now().Format(time.RFC3339)
				if s.state.Steps[i].StartedAt != "" {
					if started, err := time.Parse(time.RFC3339, s.state.Steps[i].StartedAt); err == nil {
						s.state.Steps[i].DurationMS = time.Since(started).Milliseconds()
					}
				}
			}
		}
		ui.AppendStepActivity(&s.state.Steps[i], ui.StepActivity{At: time.Now().Format("15:04:05"), StepID: stepID, Title: title, Status: status, Session: session, Detail: detail, Output: output, Error: errMsg, DurationMS: durationMS})
		s.recordExecutionTransitionLocked(stepID, title, status, session, detail, output, errMsg, durationMS)
		return
	}
}

func (s *formulaDashboardServer) recordExecutionTransitionLocked(stepID, title, status, session, detail, output, errMsg string, durationMS int64) {
	ui.RecordExecutionTransition(&s.state, ui.ExecutionTransition{
		Address: stepID, Title: title, Status: status, Session: session,
		Detail: detail, Output: output, Error: errMsg, DurationMS: durationMS,
	})
}
