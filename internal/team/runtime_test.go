package team

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

type fakeProcessor struct {
	mu             sync.Mutex
	calls          []AgentCall
	failFinalOnce  bool
	failMemoryOnce bool
}

func (f *fakeProcessor) Process(_ context.Context, call AgentCall) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	switch {
	case strings.HasSuffix(call.Session, ":memory"):
		if f.failMemoryOnce {
			f.failMemoryOnce = false
			return "", errors.New("temporary memory failure")
		}
		return "# Team Memory\n\n- The team prefers incremental delivery.\n- Decisions should remain auditable.", nil
	case strings.Contains(call.Prompt, promptMarkerFinal):
		if f.failFinalOnce {
			f.failFinalOnce = false
			return "", errors.New("temporary finalizer failure")
		}
		return "Start with a persistent event store, then add adaptive collaboration.", nil
	case strings.Contains(call.Prompt, promptMarkerReview):
		return "[YIELD]", nil
	default:
		return "Assessment from @" + call.MemberID, nil
	}
}

func TestEngineRunsRoundAndUpgradesMemory(t *testing.T) {
	workspace := t.TempDir()
	definition := testDefinition(t)
	store, err := NewStore(workspace, definition)
	if err != nil {
		t.Fatal(err)
	}
	processor := &fakeProcessor{}
	engine := &Engine{
		Definition:    definition,
		Store:         store,
		Processor:     processor,
		SessionPrefix: "test:team",
		Model:         "test-model",
	}
	result, err := engine.RunRound(context.Background(), "How should team work?")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Answer, "event store") {
		t.Fatalf("answer = %q", result.Answer)
	}
	if result.Memory.Version != 1 {
		t.Fatalf("memory version = %d", result.Memory.Version)
	}
	events, err := store.Events()
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, event := range events {
		counts[event.Type]++
		if (event.Type == "agent_message" || event.Type == "final_answer") &&
			(event.Metrics == nil || event.Metrics.Model == "" || event.Metrics.InputChars == 0 || event.Metrics.OutputChars == 0) {
			t.Fatalf("missing non-secret execution metrics: %+v", event)
		}
	}
	if counts["agent_message"] != 2 || counts["agent_yield"] != 1 || counts["final_answer"] != 1 || counts["memory_updated"] != 1 {
		t.Fatalf("event counts = %+v, events = %+v", counts, events)
	}
	sessions := map[string]bool{}
	processor.mu.Lock()
	for _, call := range processor.calls {
		if !strings.HasSuffix(call.Session, ":memory") {
			sessions[call.MemberID+"="+call.Session] = true
		}
	}
	processor.mu.Unlock()
	if len(sessions) < 2 {
		t.Fatalf("sessions = %+v", sessions)
	}
}

func TestEngineRetriesMemoryWithoutRerunningCollaboration(t *testing.T) {
	workspace := t.TempDir()
	definition := testDefinition(t)
	store, err := NewStore(workspace, definition)
	if err != nil {
		t.Fatal(err)
	}
	processor := &fakeProcessor{failMemoryOnce: true}
	engine := &Engine{Definition: definition, Store: store, Processor: processor, Model: "test-model"}
	result, err := engine.RunRound(context.Background(), "Remember this.")
	if err != nil {
		t.Fatal(err)
	}
	if result.MemoryWarning == nil || result.Memory.Version != 0 {
		t.Fatalf("expected isolated memory failure: %+v", result)
	}
	processor.mu.Lock()
	before := len(processor.calls)
	processor.mu.Unlock()
	updated, err := engine.RetryMemory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 1 || len(updated.SourceEvents) == 0 {
		t.Fatalf("updated memory = %+v", updated)
	}
	processor.mu.Lock()
	afterCalls := append([]AgentCall(nil), processor.calls[before:]...)
	processor.mu.Unlock()
	if len(afterCalls) != 1 || !strings.HasSuffix(afterCalls[0].Session, ":memory") {
		t.Fatalf("retry reran collaboration: %+v", afterCalls)
	}
	events, err := store.Events()
	if err != nil {
		t.Fatal(err)
	}
	if countPhaseMessages(events, PhaseInitial) != 2 {
		t.Fatalf("initial collaboration repeated: %+v", events)
	}
}

func TestEngineResumeDoesNotRepeatCompletedWaves(t *testing.T) {
	workspace := t.TempDir()
	definition := testDefinition(t)
	store, err := NewStore(workspace, definition)
	if err != nil {
		t.Fatal(err)
	}
	first := &fakeProcessor{failFinalOnce: true}
	engine := &Engine{Definition: definition, Store: store, Processor: first}
	if _, err := engine.RunRound(context.Background(), "How should team work?"); err == nil {
		t.Fatal("expected finalizer failure")
	}
	before, err := store.Events()
	if err != nil {
		t.Fatal(err)
	}
	initialBefore := countPhaseMessages(before, PhaseInitial)
	reviewBefore := countPhaseMessages(before, PhaseReview)

	second := &fakeProcessor{}
	engine.Processor = second
	result, err := engine.Resume(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Answer == "" {
		t.Fatal("resume returned empty answer")
	}
	after, err := store.Events()
	if err != nil {
		t.Fatal(err)
	}
	if countPhaseMessages(after, PhaseInitial) != initialBefore {
		t.Fatal("resume repeated initial wave")
	}
	if countPhaseMessages(after, PhaseReview) != reviewBefore {
		t.Fatal("resume repeated review wave")
	}
}

func TestEngineFollowupReusesThreadAndUpgradesMemory(t *testing.T) {
	workspace := t.TempDir()
	definition := testDefinition(t)
	store, err := NewStore(workspace, definition)
	if err != nil {
		t.Fatal(err)
	}
	processor := &fakeProcessor{}
	engine := &Engine{Definition: definition, Store: store, Processor: processor}
	if _, err := engine.RunRound(context.Background(), "Choose an architecture."); err != nil {
		t.Fatal(err)
	}
	result, err := engine.RunRound(context.Background(), "Now give the migration order.")
	if err != nil {
		t.Fatal(err)
	}
	if result.Round != 2 || result.Memory.Version != 2 {
		t.Fatalf("result = %+v", result)
	}
	processor.mu.Lock()
	defer processor.mu.Unlock()
	foundPriorContext := false
	for _, call := range processor.calls {
		if strings.Contains(call.Prompt, promptMarkerInitial) &&
			strings.Contains(call.Prompt, "Choose an architecture.") {
			foundPriorContext = true
			break
		}
	}
	if !foundPriorContext {
		t.Fatal("follow-up prompts did not include earlier thread context")
	}
}

func countPhaseMessages(events []Event, phase string) int {
	count := 0
	for _, event := range events {
		if event.Phase == phase && (event.Type == "agent_message" || event.Type == "agent_yield") {
			count++
		}
	}
	return count
}
