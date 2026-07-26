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

type initialRetryProcessor struct {
	mu       sync.Mutex
	failures map[string]int
	calls    map[string]int
}

func (p *initialRetryProcessor) Process(_ context.Context, call AgentCall) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.calls == nil {
		p.calls = map[string]int{}
	}
	p.calls[call.MemberID]++
	if p.failures[call.MemberID] > 0 {
		p.failures[call.MemberID]--
		return "", errors.New("temporary cooldown")
	}
	return "initial response from " + call.MemberID, nil
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

func TestInitialWaveRetriesOnlyIncompleteMembers(t *testing.T) {
	definition := testDefinition(t)
	store, err := NewStore(t.TempDir(), definition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartRound("question"); err != nil {
		t.Fatal(err)
	}
	processor := &initialRetryProcessor{failures: map[string]int{definition.Agents[0].ID: 1}}
	engine := &Engine{Definition: definition, Store: store, Processor: processor}
	if err := engine.runInitialWave(context.Background(), MemoryDocument{}); err != nil {
		t.Fatal(err)
	}
	if processor.calls[definition.Agents[0].ID] != 2 {
		t.Fatalf("failed member calls=%d", processor.calls[definition.Agents[0].ID])
	}
	if processor.calls[definition.Agents[1].ID] != 1 {
		t.Fatalf("successful member reran %d times", processor.calls[definition.Agents[1].ID])
	}
	events, err := store.Events()
	if err != nil {
		t.Fatal(err)
	}
	if countEventType(events, "initial_retry") != 1 {
		t.Fatalf("events=%+v", events)
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

func TestEngineResolvesAgentAndDefaultModels(t *testing.T) {
	definition := testDefinition(t)
	definition.DefaultModel = "team-default"
	definition.Agents[0].Model = "lead-specific"
	definition.Agents[1].Model = ""
	definition.DefinitionHash = ""
	store, err := NewStore(t.TempDir(), definition)
	if err != nil {
		t.Fatal(err)
	}
	processor := &fakeProcessor{}
	engine := &Engine{Definition: definition, Store: store, Processor: processor}
	if _, err := engine.RunRound(context.Background(), "Verify model routing."); err != nil {
		t.Fatal(err)
	}
	processor.mu.Lock()
	defer processor.mu.Unlock()
	seenLead := false
	seenExpert := false
	seenMemory := false
	for _, call := range processor.calls {
		switch call.MemberID {
		case "lead":
			seenLead = true
			if call.Model != "lead-specific" {
				t.Fatalf("lead model = %q", call.Model)
			}
			if strings.HasSuffix(call.Session, ":memory") {
				seenMemory = true
			}
		case "expert":
			seenExpert = true
			if call.Model != "team-default" {
				t.Fatalf("expert model = %q", call.Model)
			}
		}
	}
	if !seenLead || !seenExpert || !seenMemory {
		t.Fatalf("model-routed calls incomplete: %+v", processor.calls)
	}

	engine.Model = "cli-fallback"
	if got := engine.modelFor(definition.Agents[0]); got != "lead-specific" {
		t.Fatalf("agent model should beat CLI fallback, got %q", got)
	}
	if got := engine.modelFor(definition.Agents[1]); got != "cli-fallback" {
		t.Fatalf("CLI fallback should beat team default, got %q", got)
	}
}

func TestEngineInjectsConfiguredLanguageIntoEveryPhase(t *testing.T) {
	definition := testDefinition(t)
	definition.Language = "简体中文"
	definition.DefinitionHash = ""
	store, err := NewStore(t.TempDir(), definition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartRound("Explain the change."); err != nil {
		t.Fatal(err)
	}
	engine := &Engine{Definition: definition, Store: store}
	member := definition.Agents[0]
	prompts := map[string]string{
		"initial": engine.initialPrompt(MemoryDocument{}, nil),
		"review":  engine.reviewPrompt(MemoryDocument{}, nil, 1, "review"),
		"final":   engine.finalPrompt(MemoryDocument{}, nil, member),
		"memory":  engine.memoryPrompt(MemoryDocument{}, nil, "answer", member),
	}
	for phase, prompt := range prompts {
		if !strings.Contains(prompt, "输出语言要求（强制）") ||
			!strings.Contains(prompt, "必须使用简体中文") ||
			!strings.Contains(prompt, "不要改变输出语言") {
			t.Fatalf("%s prompt lacks language contract:\n%s", phase, prompt)
		}
	}
}

func TestEngineMakesCurrentQuestionAuthoritativeOverMemory(t *testing.T) {
	definition := testDefinition(t)
	store, err := NewStore(t.TempDir(), definition)
	if err != nil {
		t.Fatal(err)
	}
	const question = "重构一下当前项目"
	if _, err := store.StartRound(question); err != nil {
		t.Fatal(err)
	}
	engine := &Engine{Definition: definition, Store: store}
	memory := MemoryDocument{Version: 3, Content: "继续删除没有免费模型的 provider"}
	member := definition.Agents[0]
	for phase, prompt := range map[string]string{
		"initial": engine.initialPrompt(memory, nil),
		"review":  engine.reviewPrompt(memory, nil, 1, "review"),
		"final":   engine.finalPrompt(memory, nil, member),
	} {
		questionIndex := strings.Index(prompt, question)
		memoryIndex := strings.Index(prompt, memory.Content)
		if questionIndex < 0 || memoryIndex < 0 || questionIndex >= memoryIndex {
			t.Fatalf("%s prompt does not place current question before stale memory:\n%s", phase, prompt)
		}
		if !strings.Contains(prompt, "唯一有效目标") || !strings.Contains(prompt, "旧任务") {
			t.Fatalf("%s prompt lacks stale-memory guard:\n%s", phase, prompt)
		}
	}
	memoryPrompt := engine.memoryPrompt(memory, nil, "answer", member)
	if !strings.Contains(memoryPrompt, "不得保存活动任务") {
		t.Fatalf("memory prompt lacks active-task retention guard:\n%s", memoryPrompt)
	}
}

func TestLimitTeamResponse(t *testing.T) {
	if got := limitTeamResponse("一二三四五", 3); got != "一二三\n\n[内容因达到 Team 单次回复长度限制而截断]" {
		t.Fatalf("limited response = %q", got)
	}
	if got := limitTeamResponse("short", 10); got != "short" {
		t.Fatalf("short response changed: %q", got)
	}
	if got := compact("一二三四", 2); got != "一二\n[truncated]" {
		t.Fatalf("compact = %q", got)
	}
	if got := compactTail("一二三四", 2); got != "[earlier discussion truncated]\n三四" {
		t.Fatalf("compactTail = %q", got)
	}
}

func TestDedupeResponseParagraphs(t *testing.T) {
	got := dedupeResponseParagraphs("结论一\n\n结论二\n\n结论一")
	if got != "结论一\n\n结论二" {
		t.Fatalf("deduped response = %q", got)
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
