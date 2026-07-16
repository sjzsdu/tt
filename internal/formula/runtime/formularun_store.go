package runtime

import (
	"fmt"
	"strings"
	"sync"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/run"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

// FormulaRunStateStore mirrors typed runtime state/events into the existing
// run.Store artifact layout. It embeds a MemoryStateStore for efficient
// snapshots while preserving the current on-disk run.json/state.json/logs.jsonl
// and per-step artifact files used by dashboard/resume commands.
type FormulaRunStateStore struct {
	Memory *MemoryStateStore
	Store  *run.Store
	mu     sync.Mutex
}

func NewFormulaRunStateStore(store *run.Store) *FormulaRunStateStore {
	return &FormulaRunStateStore{Memory: NewMemoryStateStore(), Store: store}
}

func (s *FormulaRunStateStore) memory() *MemoryStateStore {
	if s.Memory == nil {
		s.Memory = NewMemoryStateStore()
	}
	return s.Memory
}

func (s *FormulaRunStateStore) StartWorkflow(id ir.WorkflowID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.memory().StartWorkflow(id); err != nil {
		return err
	}
	return s.persistSnapshot(id)
}

func (s *FormulaRunStateStore) FinishWorkflow(id ir.WorkflowID, status steps.Status) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.memory().FinishWorkflow(id, status); err != nil {
		return err
	}
	if s.Store != nil {
		mapped := mapRuntimeStatus(status)
		if mapped == run.StatusWaitingInput {
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.memory().SaveStep(state); err != nil {
		return err
	}
	if s.Store != nil {
		stepID := string(state.NodeID)
		if state.Result != nil {
			state.Result.NormalizeOutputs()
			if primary, ok := state.Result.PrimaryOutput(); ok {
				_ = s.Store.SaveStepOutput(stepID, string(primary.Raw))
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

func (s *FormulaRunStateStore) SaveRepair(workflowID ir.WorkflowID, record RepairRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.memory().SaveRepair(workflowID, record); err != nil {
		return err
	}
	return s.persistSnapshot(workflowID)
}

func (s *FormulaRunStateStore) GetStep(workflowID ir.WorkflowID, nodeID ir.NodeID) (StepState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.memory().GetStep(workflowID, nodeID)
}

func (s *FormulaRunStateStore) AppendEvent(event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.memory().AppendEvent(event); err != nil {
		return err
	}
	if s.Store != nil {
		extra := map[string]any{"runtime_event": event.Type}
		if len(event.Path.Segments) > 0 {
			extra["execution_path"] = event.Path
		}
		_ = s.Store.AppendEvent(run.Event{
			Type:   normalizeRuntimeEventType(event.Type),
			At:     event.Time.Format(formularunTimeFormat),
			StepID: string(event.NodeID),
			Status: eventStatus(event),
			Extra:  extra,
		})
	}
	return s.persistSnapshot(event.WorkflowID)
}

func (s *FormulaRunStateStore) Snapshot(id ir.WorkflowID) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	if err := s.Store.SaveRepairs(snapshot.Repairs); err != nil {
		return err
	}
	return s.Store.SaveState(snapshot)
}

func mapRuntimeStatus(status steps.Status) string {
	switch status {
	case steps.StatusCompleted:
		return run.StatusCompleted
	case steps.StatusFailed:
		return run.StatusFailed
	case steps.StatusWaiting:
		return run.StatusWaitingInput
	case steps.StatusSkipped:
		return "skipped"
	default:
		if strings.TrimSpace(string(status)) == "running" {
			return run.StatusRunning
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
		return run.StatusRunning
	case "workflow.completed", "step.completed":
		return run.StatusCompleted
	case "step.failed":
		return run.StatusFailed
	case "step.waiting":
		return run.StatusWaitingInput
	default:
		return fmt.Sprint(event.Type)
	}
}
