package runtime

import (
	"fmt"
	"strings"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/steps"
	"github.com/sjzsdu/tt/internal/formularun"
)

// FormulaRunStateStore mirrors typed runtime state/events into the existing
// formularun.Store artifact layout. It embeds a MemoryStateStore for efficient
// snapshots while preserving the current on-disk run.json/state.json/logs.jsonl
// and per-step artifact files used by dashboard/resume commands.
type FormulaRunStateStore struct {
	Memory *MemoryStateStore
	Store  *formularun.Store
}

func NewFormulaRunStateStore(store *formularun.Store) *FormulaRunStateStore {
	return &FormulaRunStateStore{Memory: NewMemoryStateStore(), Store: store}
}

func (s *FormulaRunStateStore) memory() *MemoryStateStore {
	if s.Memory == nil {
		s.Memory = NewMemoryStateStore()
	}
	return s.Memory
}

func (s *FormulaRunStateStore) StartWorkflow(id ir.WorkflowID) error {
	if err := s.memory().StartWorkflow(id); err != nil {
		return err
	}
	return s.persistSnapshot(id)
}

func (s *FormulaRunStateStore) FinishWorkflow(id ir.WorkflowID, status steps.Status) error {
	if err := s.memory().FinishWorkflow(id, status); err != nil {
		return err
	}
	if s.Store != nil {
		mapped := mapRuntimeStatus(status)
		if mapped == formularun.StatusWaitingInput {
			// The specific waiting step is recorded when SaveStep sees StatusWaiting.
			if err := s.Store.SaveMetadata(); err != nil {
				return err
			}
		} else if err := s.Store.Finish(mapped, ""); err != nil {
			return err
		}
	}
	return s.persistSnapshot(id)
}

func (s *FormulaRunStateStore) SaveStep(state StepState) error {
	if err := s.memory().SaveStep(state); err != nil {
		return err
	}
	if s.Store != nil {
		stepID := string(state.NodeID)
		if state.Result != nil {
			if len(state.Result.Output.Raw) > 0 {
				_ = s.Store.SaveStepOutput(stepID, string(state.Result.Output.Raw))
			}
			if state.Result.Error != nil {
				_ = s.Store.SaveStepError(stepID, state.Result.Error.Error())
			}
			if state.Result.Await != nil {
				_ = s.Store.SaveStepHumanInputRequest(stepID, state.Result.Await)
				_ = s.Store.MarkWaitingInput(stepID)
			}
		}
	}
	return s.persistSnapshot(state.WorkflowID)
}

func (s *FormulaRunStateStore) GetStep(workflowID ir.WorkflowID, nodeID ir.NodeID) (StepState, bool, error) {
	return s.memory().GetStep(workflowID, nodeID)
}

func (s *FormulaRunStateStore) AppendEvent(event Event) error {
	if err := s.memory().AppendEvent(event); err != nil {
		return err
	}
	if s.Store != nil {
		_ = s.Store.AppendEvent(formularun.Event{
			Type:   normalizeRuntimeEventType(event.Type),
			At:     event.Time.Format(formularunTimeFormat),
			StepID: string(event.NodeID),
			Status: eventStatus(event),
			Extra:  map[string]any{"runtime_event": event.Type},
		})
	}
	return s.persistSnapshot(event.WorkflowID)
}

func (s *FormulaRunStateStore) Snapshot(id ir.WorkflowID) (Snapshot, error) {
	return s.memory().Snapshot(id)
}

const formularunTimeFormat = "2006-01-02T15:04:05Z07:00"

func (s *FormulaRunStateStore) persistSnapshot(id ir.WorkflowID) error {
	if s.Store == nil {
		return nil
	}
	snapshot, err := s.memory().Snapshot(id)
	if err != nil {
		return err
	}
	return s.Store.SaveState(snapshot)
}

func mapRuntimeStatus(status steps.Status) string {
	switch status {
	case steps.StatusCompleted:
		return formularun.StatusCompleted
	case steps.StatusFailed:
		return formularun.StatusFailed
	case steps.StatusWaiting:
		return formularun.StatusWaitingInput
	case steps.StatusSkipped:
		return "skipped"
	default:
		if strings.TrimSpace(string(status)) == "running" {
			return formularun.StatusRunning
		}
		return string(status)
	}
}

func normalizeRuntimeEventType(value string) string {
	switch value {
	case "workflow.started":
		return "run_started"
	case "workflow.completed":
		return "run_finished"
	case "step.started":
		return "step_started"
	case "step.completed":
		return "step_completed"
	case "step.failed":
		return "step_failed"
	case "step.waiting":
		return "human_input_required"
	default:
		return strings.ReplaceAll(value, ".", "_")
	}
}

func eventStatus(event Event) string {
	switch event.Type {
	case "workflow.started", "step.started":
		return formularun.StatusRunning
	case "workflow.completed", "step.completed":
		return formularun.StatusCompleted
	case "step.failed":
		return formularun.StatusFailed
	case "step.waiting":
		return formularun.StatusWaitingInput
	default:
		return fmt.Sprint(event.Type)
	}
}
