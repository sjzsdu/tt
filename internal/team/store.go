package team

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	ThreadStatusIdle        = "idle"
	ThreadStatusRunning     = "running"
	ThreadStatusInterrupted = "interrupted"
	ThreadStatusFailed      = "failed"

	RoundStatusRunning     = "running"
	RoundStatusCompleted   = "completed"
	RoundStatusInterrupted = "interrupted"
	RoundStatusFailed      = "failed"

	PhaseInitial  = "initial"
	PhaseReview   = "review"
	PhaseFinal    = "final"
	PhaseMemory   = "memory"
	PhaseComplete = "complete"
)

type Thread struct {
	ID             string `json:"id"`
	Team           string `json:"team"`
	Title          string `json:"title,omitempty"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	Workspace      string `json:"workspace"`
	DefinitionHash string `json:"definition_hash"`
	DefinitionPath string `json:"definition_path,omitempty"`
	CurrentRound   int    `json:"current_round"`
	MemoryPath     string `json:"memory_path"`
	LastAnswer     string `json:"last_answer,omitempty"`
	Error          string `json:"error,omitempty"`
}

type RoundState struct {
	Number        int                 `json:"number"`
	Status        string              `json:"status"`
	Phase         string              `json:"phase"`
	ReviewWave    int                 `json:"review_wave,omitempty"`
	Question      string              `json:"question"`
	StartedAt     string              `json:"started_at"`
	FinishedAt    string              `json:"finished_at,omitempty"`
	FinalAnswer   string              `json:"final_answer,omitempty"`
	MemoryVersion int                 `json:"memory_version,omitempty"`
	Error         string              `json:"error,omitempty"`
	Collaboration *CollaborationState `json:"collaboration,omitempty"`
}

type CollaborationState struct {
	TurnCount            int          `json:"turn_count"`
	Cycle                int          `json:"cycle"`
	BroadReviewWaves     int          `json:"broad_review_waves"`
	Pending              []Activation `json:"pending,omitempty"`
	Objections           []Objection  `json:"objections,omitempty"`
	ProposalBy           string       `json:"proposal_by,omitempty"`
	Converged            bool         `json:"converged,omitempty"`
	StopReason           string       `json:"stop_reason,omitempty"`
	InitializedAtEventID int64        `json:"initialized_at_event_id,omitempty"`
}

type Activation struct {
	MemberID      string `json:"member_id"`
	Reason        string `json:"reason"`
	SourceEventID int64  `json:"source_event_id,omitempty"`
}

type Objection struct {
	EventID           int64    `json:"event_id"`
	From              string   `json:"from"`
	Targets           []string `json:"targets,omitempty"`
	Content           string   `json:"content,omitempty"`
	Resolved          bool     `json:"resolved,omitempty"`
	ResolvedByEventID int64    `json:"resolved_by_event_id,omitempty"`
}

type State struct {
	NextEventID int64       `json:"next_event_id"`
	Current     *RoundState `json:"current_round,omitempty"`
}

type Event struct {
	ID       int64    `json:"id"`
	Type     string   `json:"type"`
	At       string   `json:"at"`
	ThreadID string   `json:"thread_id"`
	Round    int      `json:"round"`
	Phase    string   `json:"phase,omitempty"`
	Wave     int      `json:"wave,omitempty"`
	From     string   `json:"from,omitempty"`
	To       []string `json:"to,omitempty"`
	Session  string   `json:"session,omitempty"`
	Signal   string   `json:"signal,omitempty"`
	Ref      int64    `json:"ref,omitempty"`
	Content  string   `json:"content,omitempty"`
	Error    string   `json:"error,omitempty"`
}

type Store struct {
	Root   string
	Dir    string
	Thread Thread
	State  State

	mu sync.Mutex
}

func DefaultRunRoot(workspace string) string {
	return filepath.Join(workspace, ".tt", "runs", "team")
}

func NewStore(workspace string, definition *Definition) (*Store, error) {
	if definition == nil {
		return nil, fmt.Errorf("team definition is required")
	}
	if err := definition.Validate(); err != nil {
		return nil, err
	}
	workspace, err := filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("resolve team workspace: %w", err)
	}
	root := DefaultRunRoot(workspace)
	idPart := newThreadID(time.Now())
	teamSlug := slug(definition.Team)
	dir := filepath.Join(root, teamSlug, idPart)
	if err := os.MkdirAll(filepath.Join(dir, "rounds"), 0o755); err != nil {
		return nil, fmt.Errorf("create team thread directory: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	store := &Store{
		Root: root,
		Dir:  dir,
		Thread: Thread{
			ID:             filepath.ToSlash(filepath.Join(teamSlug, idPart)),
			Team:           definition.Team,
			Title:          definition.Title,
			Status:         ThreadStatusIdle,
			CreatedAt:      now,
			UpdatedAt:      now,
			Workspace:      workspace,
			DefinitionHash: definition.DefinitionHash,
			DefinitionPath: definition.Source,
			MemoryPath:     ResolveMemoryPath(workspace, definition),
		},
		State: State{NextEventID: 1},
	}
	if err := store.saveDefinition(definition); err != nil {
		return nil, err
	}
	if err := store.saveLocked(); err != nil {
		return nil, err
	}
	return store, nil
}

func OpenStore(dir string) (*Store, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve team thread directory: %w", err)
	}
	var thread Thread
	if err := readJSON(filepath.Join(dir, "thread.json"), &thread); err != nil {
		return nil, fmt.Errorf("load team thread metadata: %w", err)
	}
	var state State
	if err := readJSON(filepath.Join(dir, "state.json"), &state); err != nil {
		return nil, fmt.Errorf("load team thread state: %w", err)
	}
	if state.NextEventID <= 0 {
		state.NextEventID = 1
		events, loadErr := loadEvents(filepath.Join(dir, "events.jsonl"))
		if loadErr == nil && len(events) > 0 {
			state.NextEventID = events[len(events)-1].ID + 1
		}
	}
	return &Store{
		Root:   filepath.Dir(filepath.Dir(dir)),
		Dir:    dir,
		Thread: thread,
		State:  state,
	}, nil
}

func ResolveStore(workspace, id string) (*Store, error) {
	records, err := ListThreads(workspace)
	if err != nil {
		return nil, err
	}
	id = strings.TrimSpace(id)
	if id == "" || strings.EqualFold(id, "latest") {
		if len(records) == 0 {
			return nil, fmt.Errorf("no team threads found")
		}
		return OpenStore(records[0].Dir)
	}
	var matches []ThreadRecord
	for _, record := range records {
		if record.Thread.ID == id || filepath.Base(record.Thread.ID) == id {
			matches = append(matches, record)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("team thread %q not found", id)
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("team thread id %q is ambiguous; use the full team/id value", id)
	}
	return OpenStore(matches[0].Dir)
}

type ThreadRecord struct {
	Dir    string
	Thread Thread
}

func ListThreads(workspace string) ([]ThreadRecord, error) {
	root := DefaultRunRoot(workspace)
	teamDirs, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list team thread root: %w", err)
	}
	var records []ThreadRecord
	for _, teamDir := range teamDirs {
		if !teamDir.IsDir() {
			continue
		}
		threadDirs, readErr := os.ReadDir(filepath.Join(root, teamDir.Name()))
		if readErr != nil {
			continue
		}
		for _, threadDir := range threadDirs {
			if !threadDir.IsDir() {
				continue
			}
			dir := filepath.Join(root, teamDir.Name(), threadDir.Name())
			var thread Thread
			if readJSON(filepath.Join(dir, "thread.json"), &thread) != nil {
				continue
			}
			records = append(records, ThreadRecord{Dir: dir, Thread: thread})
		}
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].Thread.UpdatedAt > records[j].Thread.UpdatedAt
	})
	return records, nil
}

func (s *Store) LoadDefinition() (*Definition, error) {
	if s == nil {
		return nil, fmt.Errorf("team store is required")
	}
	var definition Definition
	if err := readJSON(filepath.Join(s.Dir, "team.json"), &definition); err != nil {
		return nil, fmt.Errorf("load pinned team definition: %w", err)
	}
	if err := definition.Validate(); err != nil {
		return nil, fmt.Errorf("validate pinned team definition: %w", err)
	}
	return &definition, nil
}

func (s *Store) StartRound(question string) (*RoundState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	question = strings.TrimSpace(question)
	if question == "" {
		return nil, fmt.Errorf("team question is required")
	}
	if s.State.Current != nil && s.State.Current.Status != RoundStatusCompleted {
		return nil, fmt.Errorf("team thread round %d is %s; resume it before starting a new round", s.State.Current.Number, s.State.Current.Status)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	round := &RoundState{
		Number:    s.Thread.CurrentRound + 1,
		Status:    RoundStatusRunning,
		Phase:     PhaseInitial,
		Question:  question,
		StartedAt: now,
	}
	s.State.Current = round
	s.Thread.CurrentRound = round.Number
	s.Thread.Status = ThreadStatusRunning
	s.Thread.Error = ""
	s.Thread.UpdatedAt = now
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	if err := s.appendEventLocked(Event{Type: "round_started", Round: round.Number, Phase: PhaseInitial}); err != nil {
		return nil, err
	}
	if err := s.appendEventLocked(Event{Type: "user_message", Round: round.Number, Phase: PhaseInitial, From: "user", Content: question}); err != nil {
		return nil, err
	}
	copy := *round
	return &copy, nil
}

func (s *Store) SetPhase(phase string, reviewWave int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.State.Current == nil {
		return fmt.Errorf("team thread has no current round")
	}
	s.State.Current.Phase = strings.TrimSpace(phase)
	s.State.Current.ReviewWave = reviewWave
	s.Thread.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return s.saveLocked()
}

func (s *Store) SetCollaboration(state CollaborationState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.State.Current == nil {
		return fmt.Errorf("team thread has no current round")
	}
	copy := cloneCollaborationState(state)
	s.State.Current.Collaboration = &copy
	s.Thread.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return s.saveLocked()
}

func (s *Store) Collaboration() (*CollaborationState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.State.Current == nil {
		return nil, fmt.Errorf("team thread has no current round")
	}
	if s.State.Current.Collaboration == nil {
		return nil, nil
	}
	copy := cloneCollaborationState(*s.State.Current.Collaboration)
	return &copy, nil
}

func (s *Store) AppendEvent(event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.appendEventLocked(event)
}

func (s *Store) CompleteRound(answer string, memoryVersion int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.State.Current == nil {
		return fmt.Errorf("team thread has no current round")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	s.State.Current.Status = RoundStatusCompleted
	s.State.Current.Phase = PhaseComplete
	s.State.Current.FinalAnswer = strings.TrimSpace(answer)
	s.State.Current.MemoryVersion = memoryVersion
	s.State.Current.FinishedAt = now
	s.State.Current.Error = ""
	s.Thread.Status = ThreadStatusIdle
	s.Thread.LastAnswer = strings.TrimSpace(answer)
	s.Thread.UpdatedAt = now
	s.Thread.Error = ""
	if err := s.saveLocked(); err != nil {
		return err
	}
	roundDir := filepath.Join(s.Dir, "rounds", fmt.Sprintf("%04d", s.State.Current.Number))
	if err := atomicWriteFile(filepath.Join(roundDir, "answer.md"), []byte(strings.TrimSpace(answer)+"\n"), 0o644); err != nil {
		return err
	}
	return s.appendEventLocked(Event{
		Type:    "round_completed",
		Round:   s.State.Current.Number,
		Phase:   PhaseComplete,
		Content: strings.TrimSpace(answer),
	})
}

func (s *Store) MarkInterrupted(err error) error {
	return s.markStopped(RoundStatusInterrupted, ThreadStatusInterrupted, err)
}

func (s *Store) MarkFailed(err error) error {
	return s.markStopped(RoundStatusFailed, ThreadStatusFailed, err)
}

func (s *Store) markStopped(roundStatus, threadStatus string, stopErr error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.State.Current == nil {
		return fmt.Errorf("team thread has no current round")
	}
	message := ""
	if stopErr != nil {
		message = stopErr.Error()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	s.State.Current.Status = roundStatus
	s.State.Current.Error = message
	s.State.Current.FinishedAt = now
	s.Thread.Status = threadStatus
	s.Thread.Error = message
	s.Thread.UpdatedAt = now
	if err := s.saveLocked(); err != nil {
		return err
	}
	return s.appendEventLocked(Event{
		Type:  "round_" + roundStatus,
		Round: s.State.Current.Number,
		Phase: s.State.Current.Phase,
		Error: message,
	})
}

func (s *Store) PrepareResume() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.State.Current == nil {
		return fmt.Errorf("team thread has no current round")
	}
	if s.State.Current.Status == RoundStatusCompleted {
		return fmt.Errorf("team round %d is already completed", s.State.Current.Number)
	}
	s.State.Current.Status = RoundStatusRunning
	s.State.Current.FinishedAt = ""
	s.State.Current.Error = ""
	if s.State.Current.Collaboration != nil && s.State.Current.Collaboration.StopReason == stopReasonMaxWallTime {
		s.State.Current.Collaboration.StopReason = ""
	}
	s.Thread.Status = ThreadStatusRunning
	s.Thread.Error = ""
	s.Thread.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := s.saveLocked(); err != nil {
		return err
	}
	return s.appendEventLocked(Event{
		Type:  "round_resumed",
		Round: s.State.Current.Number,
		Phase: s.State.Current.Phase,
	})
}

func (s *Store) Events() ([]Event, error) {
	if s == nil {
		return nil, fmt.Errorf("team store is required")
	}
	return loadEvents(filepath.Join(s.Dir, "events.jsonl"))
}

func (s *Store) Snapshot() (Thread, State, []Event, error) {
	if s == nil {
		return Thread{}, State{}, nil, fmt.Errorf("team store is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	thread := s.Thread
	state := s.State
	if s.State.Current != nil {
		current := *s.State.Current
		if s.State.Current.Collaboration != nil {
			collaboration := cloneCollaborationState(*s.State.Current.Collaboration)
			current.Collaboration = &collaboration
		}
		state.Current = &current
	}
	events, err := loadEvents(filepath.Join(s.Dir, "events.jsonl"))
	if err != nil {
		return Thread{}, State{}, nil, err
	}
	return thread, state, events, nil
}

func cloneCollaborationState(state CollaborationState) CollaborationState {
	copy := state
	copy.Pending = append([]Activation(nil), state.Pending...)
	copy.Objections = make([]Objection, len(state.Objections))
	for i, objection := range state.Objections {
		copy.Objections[i] = objection
		copy.Objections[i].Targets = append([]string(nil), objection.Targets...)
	}
	return copy
}

func (s *Store) saveDefinition(definition *Definition) error {
	return writeJSONAtomic(filepath.Join(s.Dir, "team.json"), definition)
}

func (s *Store) saveLocked() error {
	if err := writeJSONAtomic(filepath.Join(s.Dir, "thread.json"), s.Thread); err != nil {
		return err
	}
	return writeJSONAtomic(filepath.Join(s.Dir, "state.json"), s.State)
}

func (s *Store) appendEventLocked(event Event) error {
	if strings.TrimSpace(event.Type) == "" {
		return fmt.Errorf("team event type is required")
	}
	if event.ID == 0 {
		event.ID = s.State.NextEventID
		s.State.NextEventID++
	}
	if event.At == "" {
		event.At = time.Now().UTC().Format(time.RFC3339)
	}
	if event.ThreadID == "" {
		event.ThreadID = s.Thread.ID
	}
	if event.Round == 0 && s.State.Current != nil {
		event.Round = s.State.Current.Number
	}
	if event.Phase == "" && s.State.Current != nil {
		event.Phase = s.State.Current.Phase
	}
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal team event: %w", err)
	}
	path := filepath.Join(s.Dir, "events.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open team event log: %w", err)
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		_ = file.Close()
		return fmt.Errorf("append team event: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close team event log: %w", err)
	}
	return writeJSONAtomic(filepath.Join(s.Dir, "state.json"), s.State)
}

func loadEvents(path string) ([]Event, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open team event log: %w", err)
	}
	defer file.Close()
	var events []Event
	scanner := bufio.NewScanner(file)
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, 4*1024*1024)
	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("parse team event log: %w", err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read team event log: %w", err)
	}
	return events, nil
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", filepath.Base(path), err)
	}
	data = append(data, '\n')
	return atomicWriteFile(path, data, 0o644)
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}
	if err := temp.Chmod(mode); err != nil {
		cleanup()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

func newThreadID(now time.Time) string {
	var raw [3]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return now.UTC().Format("20060102-150405.000000000")
	}
	return now.UTC().Format("20060102-150405") + "-" + hex.EncodeToString(raw[:])
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastDash = false
		case !lastDash:
			builder.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return "team"
	}
	return result
}
