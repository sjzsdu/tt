package formulacmd

import (
	"errors"
	"fmt"
	"time"

	"github.com/sjzsdu/tt/internal/executor"
)

func (s *formulaDashboardServer) logf(format string, args ...any) {
	s.mu.Lock()
	s.state.Logs = append(s.state.Logs, formulaDashboardLogEntry{At: time.Now().Format("15:04:05"), Text: fmt.Sprintf(format, args...)})
	if len(s.state.Logs) > 200 {
		s.state.Logs = append([]formulaDashboardLogEntry(nil), s.state.Logs[len(s.state.Logs)-200:]...)
	}
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
		appendStepActivity(&s.state.Steps[i], formulaStepActivity{At: time.Now().Format("15:04:05"), StepID: stepID, Title: title, Status: "running", Detail: fmt.Sprintf("Agent %s started this step", agent)})
		break
	}
	if !found {
		s.markLoopActivityLocked(stepID, title, "running", fmt.Sprintf("Agent %s started loop body", agent), "", "", 0)
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
		appendStepActivity(&s.state.Steps[i], formulaStepActivity{At: time.Now().Format("15:04:05"), StepID: stepID, Title: s.state.Steps[i].Title, Status: "completed", Detail: fmt.Sprintf("Completed with %d chars of output", len(output)), Output: output, DurationMS: s.state.Steps[i].DurationMS})
		break
	}
	if !found {
		s.markLoopActivityLocked(stepID, "", "completed", fmt.Sprintf("Completed with %d chars of output", len(output)), output, "", 0)
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
		appendStepActivity(&s.state.Steps[i], formulaStepActivity{At: time.Now().Format("15:04:05"), StepID: stepID, Title: s.state.Steps[i].Title, Status: "failed", Detail: errMsg, Output: output, Error: errMsg, DurationMS: s.state.Steps[i].DurationMS})
		break
	}
	if !found {
		s.markLoopActivityLocked(stepID, "", "failed", errMsg, output, errMsg, 0)
	}
	s.state.Status = "failed"
	s.state.Error = errMsg
	s.mu.Unlock()
	s.broadcast()
}

func (s *formulaDashboardServer) markStepWaitingInput(stepID, title string, request *executor.HumanInputRequest) {
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
		appendStepActivity(&s.state.Steps[i], formulaStepActivity{At: time.Now().Format("15:04:05"), StepID: stepID, Title: s.state.Steps[i].Title, Status: "waiting_input", Detail: "Waiting for human input"})
		break
	}
	if !found {
		s.markLoopActivityLocked(stepID, title, "waiting_input", "Waiting for human input", "", "", 0)
	}
	s.state.Status = "waiting_input"
	s.state.Error = ""
	s.mu.Unlock()
	s.broadcast()
}

func (s *formulaDashboardServer) markLoopActivityLocked(stepID, title, status, detail, output, errMsg string, durationMS int64) {
	parentID := loopParentStepID(stepID)
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
		appendStepActivity(&s.state.Steps[i], formulaStepActivity{At: time.Now().Format("15:04:05"), StepID: stepID, Title: title, Status: status, Detail: detail, Output: output, Error: errMsg, DurationMS: durationMS})
		return
	}
}

func (s *formulaDashboardServer) finalize(result *executor.RunResult, runErr error) {
	s.mu.Lock()
	if result != nil {
		s.state.RecipeName = result.RecipeName
		s.state.FinalOutput = result.FinalOutput
		s.state.Status = "completed"
		if result.WaitingInput > 0 {
			s.state.Status = "waiting_input"
		}
		var waitingErr executor.WaitingInputError
		if runErr != nil && !errors.As(runErr, &waitingErr) {
			s.state.Status = "failed"
			s.state.Error = runErr.Error()
		}
		for _, step := range result.Steps {
			for i := range s.state.Steps {
				if s.state.Steps[i].ID != step.StepID {
					continue
				}
				s.state.Steps[i].Title = step.Title
				s.state.Steps[i].Status = string(step.Status)
				s.state.Steps[i].Output = step.Output
				s.state.Steps[i].Error = step.Error
				if step.Status == executor.StatusSkipped {
					s.state.Steps[i].FinishedAt = time.Now().Format(time.RFC3339)
				}
				break
			}
		}
	}
	s.mu.Unlock()
	s.broadcast()
}
