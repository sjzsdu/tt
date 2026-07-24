package team

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseBlackboardOperations(t *testing.T) {
	content, operations := parseBlackboardOperations(`先核对接口边界。
[TEAM_BLACKBOARD] {"action":"UPSERT","kind":"FACT","key":" API Contract ","content":"Only HTTPS is supported."}
[TEAM_BLACKBOARD] {"action":"resolve","kind":"question","key":"transport"}
[TEAM_BLACKBOARD] {"action":"upsert","kind":"unknown","key":"ignored","content":"invalid"}
[TEAM_SIGNAL:AGREE]`)

	if len(operations) != 2 {
		t.Fatalf("operations = %+v", operations)
	}
	if operations[0].Action != BlackboardActionUpsert ||
		operations[0].Kind != BlackboardFact ||
		operations[0].Key != "api contract" ||
		operations[0].Content != "Only HTTPS is supported." {
		t.Fatalf("first operation = %+v", operations[0])
	}
	if operations[1].Action != BlackboardActionResolve || operations[1].Content != "" {
		t.Fatalf("second operation = %+v", operations[1])
	}
	if strings.Contains(content, `"transport"`) || !strings.Contains(content, `"unknown"`) {
		t.Fatalf("clean content = %q", content)
	}
}

func TestProjectBlackboardIsDeterministicAndRetainsProvenance(t *testing.T) {
	events := []Event{
		{
			ID:    5,
			Round: 1,
			From:  "expert",
			Ref:   4,
			At:    "2026-07-24T02:00:05Z",
			Blackboard: &BlackboardOperation{
				Action: BlackboardActionResolve,
				Kind:   BlackboardQuestion,
				Key:    "transport",
			},
		},
		{
			ID:    2,
			Round: 1,
			From:  "lead",
			Ref:   1,
			At:    "2026-07-24T02:00:02Z",
			Blackboard: &BlackboardOperation{
				Action:  BlackboardActionUpsert,
				Kind:    BlackboardFact,
				Key:     "api",
				Content: "HTTP is supported.",
			},
		},
		{
			ID:    4,
			Round: 1,
			From:  "observer",
			Ref:   3,
			At:    "2026-07-24T02:00:04Z",
			Blackboard: &BlackboardOperation{
				Action:  BlackboardActionUpsert,
				Kind:    BlackboardFact,
				Key:     "api",
				Content: "Only HTTPS is supported.",
			},
		},
		{
			ID:    3,
			Round: 1,
			From:  "lead",
			At:    "2026-07-24T02:00:03Z",
			Blackboard: &BlackboardOperation{
				Action:  BlackboardActionUpsert,
				Kind:    BlackboardQuestion,
				Key:     "transport",
				Content: "Which transport is allowed?",
			},
		},
		{
			ID:    6,
			Round: 2,
			From:  "lead",
			Blackboard: &BlackboardOperation{
				Action:  BlackboardActionUpsert,
				Kind:    BlackboardDecision,
				Key:     "other-round",
				Content: "Ignore this.",
			},
		},
	}

	first := ProjectBlackboard(events, 1)
	second := ProjectBlackboard([]Event{events[4], events[0], events[2], events[1], events[3]}, 1)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("projection depends on input order:\n%+v\n%+v", first, second)
	}
	if len(first.Entries) != 2 {
		t.Fatalf("entries = %+v", first.Entries)
	}
	fact := first.Entries[0]
	if fact.Kind != BlackboardFact || fact.Content != "Only HTTPS is supported." ||
		fact.UpdatedBy != "observer" || len(fact.Revisions) != 2 ||
		fact.Revisions[0].EventID != 2 || fact.Revisions[1].EventID != 4 {
		t.Fatalf("fact = %+v", fact)
	}
	question := first.Entries[1]
	if question.Status != BlackboardStatusResolved || question.UpdatedAtEventID != 5 ||
		len(question.Revisions) != 2 {
		t.Fatalf("question = %+v", question)
	}
}

func TestBlackboardProjectionRebuildsAfterStoreReopen(t *testing.T) {
	store, err := NewStore(t.TempDir(), testDefinition(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartRound("Review the transport."); err != nil {
		t.Fatal(err)
	}
	operation := BlackboardOperation{
		Action:  BlackboardActionUpsert,
		Kind:    BlackboardDecision,
		Key:     "transport",
		Content: "Use HTTPS.",
	}
	if err := store.AppendEvent(Event{
		Type:       "blackboard_upsert",
		From:       "lead",
		Blackboard: &operation,
	}); err != nil {
		t.Fatal(err)
	}
	beforeEvents, err := store.Events()
	if err != nil {
		t.Fatal(err)
	}
	before := ProjectBlackboard(beforeEvents, 1)

	reopened, err := OpenStore(store.Dir)
	if err != nil {
		t.Fatal(err)
	}
	afterEvents, err := reopened.Events()
	if err != nil {
		t.Fatal(err)
	}
	after := ProjectBlackboard(afterEvents, 1)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("projection after reopen = %+v, want %+v", after, before)
	}
}

func TestRuntimeSharesBlackboardProjectionWithReviewAndFinalizer(t *testing.T) {
	definition := adaptiveTestDefinition(t, 10)
	definition.Coordination.ReviewWaves = 1
	processor := &adaptiveProcessor{}
	processor.response = func(call AgentCall, _ int) (string, error) {
		switch promptPhase(call.Prompt) {
		case "initial":
			if call.MemberID == "lead" {
				return `先记录初始事实。
[TEAM_BLACKBOARD] {"action":"upsert","kind":"fact","key":"transport","content":"HTTP may be supported."}`, nil
			}
			return "[TEAM_SIGNAL:YIELD]", nil
		case "review":
			if !strings.Contains(call.Prompt, "[fact/transport] active") ||
				!strings.Contains(call.Prompt, "HTTP may be supported.") {
				return "", errors.New("review prompt missing projected blackboard")
			}
			if call.MemberID == "expert" {
				return `更正协议约束。
[TEAM_BLACKBOARD] {"action":"upsert","kind":"fact","key":"transport","content":"Only HTTPS is supported."}
[TEAM_SIGNAL:AGREE]`, nil
			}
			return "[TEAM_SIGNAL:YIELD]", nil
		case "final":
			if !strings.Contains(call.Prompt, "Only HTTPS is supported.") {
				return "", errors.New("final prompt missing updated blackboard")
			}
			return "最终采用 HTTPS。", nil
		default:
			return "", errors.New("unexpected prompt phase")
		}
	}

	store := runAdaptiveRound(t, definition, processor)
	events, err := store.Events()
	if err != nil {
		t.Fatal(err)
	}
	projection := ProjectBlackboard(events, 1)
	if len(projection.Entries) != 1 || projection.Entries[0].Content != "Only HTTPS is supported." ||
		len(projection.Entries[0].Revisions) != 2 {
		t.Fatalf("projection = %+v", projection)
	}
	for _, event := range events {
		if event.Blackboard != nil && event.Ref == 0 {
			t.Fatalf("blackboard event lacks source message provenance: %+v", event)
		}
	}
}

func TestRuntimeResumeRebuildsBlackboardFromPersistedEvents(t *testing.T) {
	definition := adaptiveTestDefinition(t, 10)
	processor := &adaptiveProcessor{}
	processor.response = func(call AgentCall, ordinal int) (string, error) {
		switch promptPhase(call.Prompt) {
		case "initial":
			if call.MemberID == "lead" {
				return `@expert 请复核传输协议。
[TEAM_BLACKBOARD] {"action":"upsert","kind":"fact","key":"transport","content":"HTTPS is required."}`, nil
			}
			return "[TEAM_SIGNAL:YIELD]", nil
		case "review":
			if ordinal == 1 {
				return "", errors.New("temporary review failure")
			}
			if !strings.Contains(call.Prompt, "HTTPS is required.") {
				return "", errors.New("resumed prompt missing projected blackboard")
			}
			return "恢复后确认。 \n[TEAM_SIGNAL:AGREE]", nil
		case "final":
			if !strings.Contains(call.Prompt, "HTTPS is required.") {
				return "", errors.New("final prompt missing resumed blackboard")
			}
			return "最终采用 HTTPS。", nil
		default:
			return "", errors.New("unexpected prompt phase")
		}
	}

	store, err := NewStore(t.TempDir(), definition)
	if err != nil {
		t.Fatal(err)
	}
	engine := &Engine{Definition: definition, Store: store, Processor: processor}
	if _, err := engine.RunRound(context.Background(), "选择传输协议。"); err == nil {
		t.Fatal("expected temporary review failure")
	}

	reopened, err := OpenStore(store.Dir)
	if err != nil {
		t.Fatal(err)
	}
	resumed := &Engine{Definition: definition, Store: reopened, Processor: processor}
	if _, err := resumed.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	events, err := reopened.Events()
	if err != nil {
		t.Fatal(err)
	}
	projection := ProjectBlackboard(events, 1)
	if len(projection.Entries) != 1 || projection.Entries[0].Content != "HTTPS is required." {
		t.Fatalf("projection after resume = %+v", projection)
	}
}

func TestNormalizeBlackboardOperationRejectsOversizedContent(t *testing.T) {
	_, err := normalizeBlackboardOperation(BlackboardOperation{
		Action:  BlackboardActionUpsert,
		Kind:    BlackboardArtifact,
		Key:     "large",
		Content: strings.Repeat("界", 4001),
	})
	if err == nil {
		t.Fatal("expected oversized content error")
	}
}
