package runtime

import (
	"sync"
	"time"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

type StepState struct {
	WorkflowID  ir.WorkflowID
	NodeID      ir.NodeID
	Status      steps.Status
	Result      *steps.RunResult
	StartedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt time.Time
}

type Snapshot struct {
	WorkflowID ir.WorkflowID
	Status     steps.Status
	Steps      map[ir.NodeID]StepState
	Events     []Event
}

type StateStore interface {
	StartWorkflow(ir.WorkflowID) error
	FinishWorkflow(ir.WorkflowID, steps.Status) error
	SaveStep(StepState) error
	GetStep(ir.WorkflowID, ir.NodeID) (StepState, bool, error)
	AppendEvent(Event) error
	Snapshot(ir.WorkflowID) (Snapshot, error)
}

type MemoryStateStore struct {
	mu       sync.RWMutex
	workflow map[ir.WorkflowID]steps.Status
	steps    map[ir.WorkflowID]map[ir.NodeID]StepState
	events   map[ir.WorkflowID][]Event
	now      func() time.Time
}

func NewMemoryStateStore() *MemoryStateStore {
	return &MemoryStateStore{
		workflow: map[ir.WorkflowID]steps.Status{},
		steps:    map[ir.WorkflowID]map[ir.NodeID]StepState{},
		events:   map[ir.WorkflowID][]Event{},
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
		stepsCopy[nodeID] = state
	}
	eventsCopy := append([]Event(nil), s.events[id]...)
	return Snapshot{WorkflowID: id, Status: s.workflow[id], Steps: stepsCopy, Events: eventsCopy}, nil
}
