package runtime

import (
	"strings"
	"sync"
	"time"

	"github.com/sjzsdu/tt/internal/formula/executionpath"
	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

type StepState struct {
	WorkflowID  ir.WorkflowID
	NodeID      ir.NodeID
	Path        executionpath.Path
	Status      steps.Status
	Result      *steps.RunResult
	QueuedAt    time.Time
	StartedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt time.Time
}

type RepairRecord struct {
	StepID             string   `json:"step_id"`
	Kind               string   `json:"kind,omitempty"`
	Attempt            int      `json:"attempt,omitempty"`
	Status             string   `json:"status,omitempty"`
	Reason             string   `json:"reason,omitempty"`
	FormulaUpdateHint  string   `json:"formula_update_hint,omitempty"`
	NextAttemptHint    string   `json:"next_attempt_hint,omitempty"`
	Advice             string   `json:"advice,omitempty"`
	OriginalCommand    []string `json:"original_command,omitempty"`
	FixedCommand       []string `json:"fixed_command,omitempty"`
	Error              string   `json:"error,omitempty"`
	RecordedAt         string   `json:"recorded_at,omitempty"`
	ConfirmedAt        string   `json:"confirmed_at,omitempty"`
	ConfirmationStatus string   `json:"confirmation_status,omitempty"`
}

type Snapshot struct {
	WorkflowID ir.WorkflowID
	Status     steps.Status
	Steps      map[ir.NodeID]StepState
	Events     []Event
	Repairs    []RepairRecord
}

type StateStore interface {
	StartWorkflow(ir.WorkflowID) error
	FinishWorkflow(ir.WorkflowID, steps.Status) error
	SaveStep(StepState) error
	SaveRepair(ir.WorkflowID, RepairRecord) error
	GetStep(ir.WorkflowID, ir.NodeID) (StepState, bool, error)
	AppendEvent(Event) error
	Snapshot(ir.WorkflowID) (Snapshot, error)
}

type MemoryStateStore struct {
	mu       sync.RWMutex
	workflow map[ir.WorkflowID]steps.Status
	steps    map[ir.WorkflowID]map[ir.NodeID]StepState
	events   map[ir.WorkflowID][]Event
	repairs  map[ir.WorkflowID][]RepairRecord
	now      func() time.Time
}

func NewMemoryStateStore() *MemoryStateStore {
	return &MemoryStateStore{
		workflow: map[ir.WorkflowID]steps.Status{},
		steps:    map[ir.WorkflowID]map[ir.NodeID]StepState{},
		events:   map[ir.WorkflowID][]Event{},
		repairs:  map[ir.WorkflowID][]RepairRecord{},
		now:      time.Now,
	}
}

func (s *MemoryStateStore) StartWorkflow(id ir.WorkflowID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workflow[id] = "running"
	if s.steps[id] == nil {
		s.steps[id] = map[ir.NodeID]StepState{}
	}
	return nil
}

func (s *MemoryStateStore) FinishWorkflow(id ir.WorkflowID, status steps.Status) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workflow[id] = status
	return nil
}

func (s *MemoryStateStore) SaveStep(state StepState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = s.now()
	}
	if s.steps[state.WorkflowID] == nil {
		s.steps[state.WorkflowID] = map[ir.NodeID]StepState{}
	}
	s.steps[state.WorkflowID][state.NodeID] = state
	return nil
}

func (s *MemoryStateStore) SaveRepair(workflowID ir.WorkflowID, record RepairRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(record.RecordedAt) == "" {
		record.RecordedAt = s.now().Format(time.RFC3339)
	}
	s.repairs[workflowID] = append(s.repairs[workflowID], record)
	return nil
}

func (s *MemoryStateStore) GetStep(workflowID ir.WorkflowID, nodeID ir.NodeID) (StepState, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.steps[workflowID][nodeID]
	return state, ok, nil
}

func (s *MemoryStateStore) AppendEvent(event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if event.Time.IsZero() {
		event.Time = s.now()
	}
	s.events[event.WorkflowID] = append(s.events[event.WorkflowID], event)
	return nil
}

func (s *MemoryStateStore) Snapshot(id ir.WorkflowID) (Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stepsCopy := map[ir.NodeID]StepState{}
	for nodeID, state := range s.steps[id] {
		state.Path.Segments = append([]executionpath.Segment(nil), state.Path.Segments...)
		stepsCopy[nodeID] = state
	}
	eventsCopy := append([]Event(nil), s.events[id]...)
	repairsCopy := append([]RepairRecord(nil), s.repairs[id]...)
	return Snapshot{WorkflowID: id, Status: s.workflow[id], Steps: stepsCopy, Events: eventsCopy, Repairs: repairsCopy}, nil
}
