package team

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestParseCollaborationResponse(t *testing.T) {
	definition := adaptiveTestDefinition(t, 8)
	content, signal, targets := parseCollaborationResponse(
		"需要 @expert 和 @observer 复核。\n[TEAM_SIGNAL:OBJECT]",
		definition,
	)
	if content != "需要 @expert 和 @observer 复核。" || signal != SignalObject {
		t.Fatalf("content/signal = %q, %q", content, signal)
	}
	if strings.Join(targets, ",") != "expert,observer" {
		t.Fatalf("targets = %v", targets)
	}

	content, signal, targets = parseCollaborationResponse("[YIELD]", definition)
	if content != "" || signal != SignalYield || len(targets) != 0 {
		t.Fatalf("yield parse = %q, %q, %v", content, signal, targets)
	}

	_, _, targets = parseCollaborationResponse(
		"请 @test‑engineer 验证。",
		&Definition{Agents: []Agent{{ID: "test-engineer"}}},
	)
	if len(targets) != 1 || targets[0] != "test-engineer" {
		t.Fatalf("unicode hyphen mention targets = %v", targets)
	}
}

type adaptiveProcessor struct {
	mu       sync.Mutex
	calls    []AgentCall
	counts   map[string]int
	response func(AgentCall, int) (string, error)
}

type timeoutOnceProcessor struct {
	mu      sync.Mutex
	blocked bool
}

func (p *timeoutOnceProcessor) Process(ctx context.Context, call AgentCall) (string, error) {
	switch promptPhase(call.Prompt) {
	case "initial":
		if call.MemberID == "lead" {
			return "@expert 请继续检查。", nil
		}
		return "[TEAM_SIGNAL:YIELD]", nil
	case "review":
		p.mu.Lock()
		shouldBlock := !p.blocked
		if shouldBlock {
			p.blocked = true
		}
		p.mu.Unlock()
		if shouldBlock {
			<-ctx.Done()
			return "", ctx.Err()
		}
		return "检查完成。\n[TEAM_SIGNAL:AGREE]", nil
	case "final":
		return "超时恢复后的最终方案", nil
	default:
		return "", errors.New("unexpected prompt phase")
	}
}

func (p *adaptiveProcessor) Process(_ context.Context, call AgentCall) (string, error) {
	p.mu.Lock()
	if p.counts == nil {
		p.counts = map[string]int{}
	}
	phase := promptPhase(call.Prompt)
	key := phase + "|" + strings.ToLower(call.MemberID)
	p.counts[key]++
	ordinal := p.counts[key]
	p.calls = append(p.calls, call)
	p.mu.Unlock()
	return p.response(call, ordinal)
}

func TestAdaptiveRoutingActivatesSimultaneousMentions(t *testing.T) {
	definition := adaptiveTestDefinition(t, 8)
	processor := &adaptiveProcessor{}
	processor.response = func(call AgentCall, _ int) (string, error) {
		switch promptPhase(call.Prompt) {
		case "initial":
			if call.MemberID == "lead" {
				return "@expert 和 @observer 请分别复核风险。", nil
			}
			return "[TEAM_SIGNAL:YIELD]", nil
		case "review":
			return "复核完成，没有阻塞问题。\n[TEAM_SIGNAL:AGREE]", nil
		case "final":
			return "最终方案", nil
		default:
			return "", errors.New("unexpected prompt phase")
		}
	}

	store := runAdaptiveRound(t, definition, processor)
	events, err := store.Events()
	if err != nil {
		t.Fatal(err)
	}
	var activated []string
	for _, event := range events {
		if event.Type == "agent_activated" && len(event.To) == 1 {
			activated = append(activated, event.To[0])
		}
	}
	if strings.Join(activated, ",") != "expert,observer" {
		t.Fatalf("activated = %v, events = %+v", activated, events)
	}
	if countEventType(events, "convergence_reached") != 1 || countEventType(events, "forced_stop") != 0 {
		t.Fatalf("convergence events = %+v", events)
	}
}

func TestInitialHandoffIgnoresInitialMentionStorm(t *testing.T) {
	definition := adaptiveTestDefinition(t, 8)
	definition.Coordination.InitialHandoff = "observer"
	processor := &adaptiveProcessor{}
	processor.response = func(call AgentCall, _ int) (string, error) {
		switch promptPhase(call.Prompt) {
		case "initial":
			if call.MemberID == "lead" {
				return "@expert 和 @observer 请继续讨论。", nil
			}
			return "[TEAM_SIGNAL:YIELD]", nil
		case "review":
			return "交接任务完成。\n[TEAM_SIGNAL:AGREE]", nil
		case "final":
			return "最终方案", nil
		default:
			return "", errors.New("unexpected prompt phase")
		}
	}

	store := runAdaptiveRound(t, definition, processor)
	events, err := store.Events()
	if err != nil {
		t.Fatal(err)
	}
	var activated []string
	for _, event := range events {
		if event.Type == "agent_activated" && len(event.To) == 1 {
			activated = append(activated, event.To[0])
		}
	}
	if strings.Join(activated, ",") != "observer" {
		t.Fatalf("activated = %v, want only initial handoff", activated)
	}
}

func TestLimitPendingReviewTurns(t *testing.T) {
	pending := []Activation{{MemberID: "expert"}, {MemberID: "observer"}}
	events := []Event{
		{Round: 1, Phase: PhaseReview, Type: "agent_message", From: "expert"},
		{Round: 1, Phase: PhaseReview, Type: "agent_yield", From: "expert"},
		{Round: 1, Phase: PhaseReview, Type: "agent_message", From: "observer"},
	}
	filtered := limitPendingReviewTurns(pending, events, 1, 2)
	if len(filtered) != 1 || filtered[0].MemberID != "observer" {
		t.Fatalf("filtered = %+v", filtered)
	}
}

func TestSoftwareDevelopmentUsesDeliveryPipeline(t *testing.T) {
	definition, err := Load("software-development")
	if err != nil {
		t.Fatal(err)
	}
	disabled := false
	definition.Memory.Enabled = &disabled
	processor := &adaptiveProcessor{}
	processor.response = func(call AgentCall, _ int) (string, error) {
		switch promptPhase(call.Prompt) {
		case "initial":
			return "[TEAM_SIGNAL:YIELD]", nil
		case "review":
			switch call.MemberID {
			case "implementer":
				return "实现已完成，请 @code-reviewer 检查；通过后由 @test-engineer 验证。\n[TEAM_SIGNAL:AGREE]", nil
			case "code-reviewer":
				return "审查通过，请 @test‑engineer 验证。\n[TEAM_SIGNAL:AGREE]", nil
			case "test-engineer":
				return "测试通过，请 @delivery-lead 交付。\n[TEAM_VERIFICATION] {\"commands\":[[\"true\"]]}\n[TEAM_SIGNAL:AGREE]", nil
			case "delivery-lead":
				return "交付证据齐全。\n[TEAM_SIGNAL:AGREE]", nil
			default:
				return "[TEAM_SIGNAL:YIELD]", nil
			}
		case "final":
			return "已完成实现、审查和测试。", nil
		default:
			return "", errors.New("unexpected prompt phase")
		}
	}

	store := runAdaptiveRound(t, definition, processor)
	events, err := store.Events()
	if err != nil {
		t.Fatal(err)
	}
	var activated []string
	for _, event := range events {
		if event.Type == "agent_activated" && len(event.To) == 1 {
			activated = append(activated, event.To[0])
		}
	}
	if strings.Join(activated, ",") != "implementer,code-reviewer,test-engineer,delivery-lead" {
		t.Fatalf("activation pipeline = %v", activated)
	}
	if countEventType(events, "forced_stop") != 0 || countEventType(events, "final_answer") != 1 {
		t.Fatalf("pipeline did not finish cleanly: %+v", events)
	}
}

func TestAdaptiveRoutingKeepsObjectionOpenUntilOwnerResolves(t *testing.T) {
	definition := adaptiveTestDefinition(t, 9)
	processor := &adaptiveProcessor{}
	processor.response = func(call AgentCall, ordinal int) (string, error) {
		switch promptPhase(call.Prompt) {
		case "initial":
			if call.MemberID == "expert" {
				return "这个边界条件仍未处理，@lead 请回应。\n[TEAM_SIGNAL:OBJECT]", nil
			}
			return "初始判断", nil
		case "review":
			switch call.MemberID {
			case "lead":
				return "边界条件已经补入方案，@expert 请确认。\n[TEAM_SIGNAL:AGREE]", nil
			case "expert":
				if ordinal == 1 {
					return "确认已经解决。\n[TEAM_SIGNAL:RESOLVED]", nil
				}
			}
			return "[TEAM_SIGNAL:YIELD]", nil
		case "final":
			return "包含边界条件的最终方案", nil
		default:
			return "", errors.New("unexpected prompt phase")
		}
	}

	store := runAdaptiveRound(t, definition, processor)
	events, err := store.Events()
	if err != nil {
		t.Fatal(err)
	}
	opened := firstEventIndex(events, "objection_opened")
	resolved := firstEventIndex(events, "objection_resolved")
	final := firstEventIndex(events, "final_answer")
	if opened < 0 || resolved <= opened || final <= resolved {
		t.Fatalf("objection/final event order = %d, %d, %d; events = %+v", opened, resolved, final, events)
	}
	if store.State.Current.Collaboration == nil || hasOpenObjections(*store.State.Current.Collaboration) {
		t.Fatalf("collaboration = %+v", store.State.Current.Collaboration)
	}
}

func TestAdaptiveRoutingOpensObjectionWindowForFinalProposal(t *testing.T) {
	definition := adaptiveTestDefinition(t, 8)
	processor := &adaptiveProcessor{}
	processor.response = func(call AgentCall, _ int) (string, error) {
		switch promptPhase(call.Prompt) {
		case "initial":
			if call.MemberID == "lead" {
				return "信息已经足够，可以进入总结。\n[TEAM_SIGNAL:PROPOSE_FINAL]", nil
			}
			return "[TEAM_SIGNAL:YIELD]", nil
		case "review":
			return "没有阻塞性异议。\n[TEAM_SIGNAL:AGREE]", nil
		case "final":
			return "通过异议窗口后的最终方案", nil
		default:
			return "", errors.New("unexpected prompt phase")
		}
	}

	store := runAdaptiveRound(t, definition, processor)
	events, err := store.Events()
	if err != nil {
		t.Fatal(err)
	}
	var windowMembers []string
	for _, event := range events {
		if event.Type == "agent_activated" && event.Content == activationObjectionWindow && len(event.To) == 1 {
			windowMembers = append(windowMembers, event.To[0])
		}
	}
	if strings.Join(windowMembers, ",") != "expert,observer" {
		t.Fatalf("objection window members = %v", windowMembers)
	}
	if firstEventIndex(events, "convergence_reached") < 0 {
		t.Fatalf("events = %+v", events)
	}
}

func TestAdaptiveRoutingPersistsForcedStopAtTurnLimit(t *testing.T) {
	definition := adaptiveTestDefinition(t, 5)
	processor := &adaptiveProcessor{}
	processor.response = func(call AgentCall, _ int) (string, error) {
		switch promptPhase(call.Prompt) {
		case "initial":
			if call.MemberID == "expert" {
				return "仍有风险，@lead 回应。\n[TEAM_SIGNAL:OBJECT]", nil
			}
			return "初始判断", nil
		case "review":
			return "已回应，@expert 再判断。", nil
		case "final":
			return "在预算边界内强制总结，并保留未解决风险。", nil
		default:
			return "", errors.New("unexpected prompt phase")
		}
	}

	store := runAdaptiveRound(t, definition, processor)
	events, err := store.Events()
	if err != nil {
		t.Fatal(err)
	}
	forced := eventByType(events, "forced_stop")
	if forced == nil || forced.Content != stopReasonMaxAgentTurns {
		t.Fatalf("forced stop = %+v", forced)
	}
	state := store.State.Current.Collaboration
	if state == nil || state.StopReason != stopReasonMaxAgentTurns || !hasOpenObjections(*state) {
		t.Fatalf("collaboration = %+v", state)
	}
	if state.TurnCount != definition.Limits.MaxAgentTurns {
		t.Fatalf("turn count = %d, want %d", state.TurnCount, definition.Limits.MaxAgentTurns)
	}
}

func TestAdaptiveRoutingRetriesPendingActivationWithoutFailingRound(t *testing.T) {
	definition := adaptiveTestDefinition(t, 8)
	processor := &adaptiveProcessor{}
	processor.response = func(call AgentCall, ordinal int) (string, error) {
		switch promptPhase(call.Prompt) {
		case "initial":
			if call.MemberID == "lead" {
				return "@expert 请进行定向复核。", nil
			}
			return "[TEAM_SIGNAL:YIELD]", nil
		case "review":
			if call.MemberID == "expert" && ordinal == 1 {
				return "", errors.New("temporary expert failure")
			}
			return "复核完成。\n[TEAM_SIGNAL:AGREE]", nil
		case "final":
			return "恢复后的最终方案", nil
		default:
			return "", errors.New("unexpected prompt phase")
		}
	}

	store, err := NewStore(t.TempDir(), definition)
	if err != nil {
		t.Fatal(err)
	}
	engine := &Engine{Definition: definition, Store: store, Processor: processor}
	if _, err := engine.RunRound(context.Background(), "请复核。"); err != nil {
		t.Fatal(err)
	}
	events, err := store.Events()
	if err != nil {
		t.Fatal(err)
	}
	if countEventType(events, "agent_error") != 1 {
		t.Fatalf("agent errors = %+v", events)
	}
	if countEventType(events, "convergence_reached") != 1 ||
		countEventType(events, "round_failed") != 0 ||
		countEventType(events, "final_answer") != 1 {
		t.Fatalf("events = %+v", events)
	}
	if processor.counts["review|expert"] != 2 {
		t.Fatalf("expert review attempts = %d", processor.counts["review|expert"])
	}
}

func TestAdaptiveRoutingCapsPersistentFailureAndFinalizesWithEvidence(t *testing.T) {
	definition := adaptiveTestDefinition(t, 8)
	definition.Limits.MaxReviewTurnsPerAgent = 2
	processor := &adaptiveProcessor{}
	processor.response = func(call AgentCall, _ int) (string, error) {
		switch promptPhase(call.Prompt) {
		case "initial":
			if call.MemberID == "lead" {
				return "@expert 请进行复核。", nil
			}
			return "[TEAM_SIGNAL:YIELD]", nil
		case "review":
			return "", errors.New("provider unavailable")
		case "final":
			if !strings.Contains(call.Prompt, "@expert: [ERROR] provider unavailable") {
				return "", errors.New("final prompt missing review failure evidence")
			}
			return "复核模型不可用；保留风险后交付。", nil
		default:
			return "", errors.New("unexpected prompt phase")
		}
	}

	store := runAdaptiveRound(t, definition, processor)
	events, err := store.Events()
	if err != nil {
		t.Fatal(err)
	}
	if countEventType(events, "agent_error") != 2 ||
		countEventType(events, "round_failed") != 0 ||
		countEventType(events, "final_answer") != 1 {
		t.Fatalf("persistent failure lifecycle = %+v", events)
	}
}

func TestAdaptiveRoutingPersistsWallTimeStopAndResumes(t *testing.T) {
	definition := adaptiveTestDefinition(t, 8)
	definition.Limits.MaxWallTime = "150ms"
	processor := &timeoutOnceProcessor{}
	store, err := NewStore(t.TempDir(), definition)
	if err != nil {
		t.Fatal(err)
	}
	engine := &Engine{Definition: definition, Store: store, Processor: processor}
	if _, err := engine.RunRound(context.Background(), "请限时复核。"); err == nil {
		t.Fatal("expected wall-time interruption")
	}
	events, err := store.Events()
	if err != nil {
		t.Fatal(err)
	}
	forced := eventByType(events, "forced_stop")
	if forced == nil || forced.Content != stopReasonMaxWallTime {
		t.Fatalf("forced stop = %+v", forced)
	}
	if store.Thread.Status != ThreadStatusInterrupted {
		t.Fatalf("thread status = %s", store.Thread.Status)
	}

	if _, err := engine.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	state := store.State.Current.Collaboration
	if state == nil || state.StopReason != stopReasonConverged {
		t.Fatalf("collaboration after resume = %+v", state)
	}
}

func adaptiveTestDefinition(t *testing.T, maxTurns int) *Definition {
	t.Helper()
	definition, err := Parse([]byte(fmt.Sprintf(`team = "adaptive"
title = "Adaptive Team"
version = 1

[coordination]
facilitator = "lead"
finalizer = "lead"
review_waves = 0
max_concurrency = 3

[limits]
max_agent_turns = %d
max_wall_time = "2m"

[memory]
enabled = false
maintainer = "lead"
max_chars = 5000

[[agents]]
id = "lead"
role = "Lead"

[[agents]]
id = "expert"
role = "Expert"

[[agents]]
id = "observer"
role = "Observer"
`, maxTurns)))
	if err != nil {
		t.Fatal(err)
	}
	return definition
}

func runAdaptiveRound(t *testing.T, definition *Definition, processor Processor) *Store {
	t.Helper()
	store, err := NewStore(t.TempDir(), definition)
	if err != nil {
		t.Fatal(err)
	}
	engine := &Engine{
		Definition:    definition,
		Store:         store,
		Processor:     processor,
		SessionPrefix: "test:adaptive",
	}
	if _, err := engine.RunRound(context.Background(), "请给出方案。"); err != nil {
		t.Fatal(err)
	}
	return store
}

func promptPhase(prompt string) string {
	switch {
	case strings.Contains(prompt, promptMarkerInitial):
		return "initial"
	case strings.Contains(prompt, promptMarkerReview):
		return "review"
	case strings.Contains(prompt, promptMarkerFinal):
		return "final"
	case strings.Contains(prompt, promptMarkerMemory):
		return "memory"
	default:
		return ""
	}
}

func countEventType(events []Event, eventType string) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

func firstEventIndex(events []Event, eventType string) int {
	for i, event := range events {
		if event.Type == eventType {
			return i
		}
	}
	return -1
}

func eventByType(events []Event, eventType string) *Event {
	for i := range events {
		if events[i].Type == eventType {
			return &events[i]
		}
	}
	return nil
}
