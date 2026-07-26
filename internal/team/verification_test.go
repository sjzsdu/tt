package team

import (
	"context"
	"strings"
	"testing"
)

func TestParseVerificationRequest(t *testing.T) {
	content, request := parseVerificationRequest("证据\n[TEAM_VERIFICATION] {\"commands\":[[\"go\",\"test\",\"./...\"]]}")
	if content != "证据" || request == nil || len(request.Commands) != 1 {
		t.Fatalf("content=%q request=%+v", content, request)
	}
}

func TestRuntimeVerificationUsesRealExitCode(t *testing.T) {
	definition := testDefinition(t)
	definition.Verification = VerificationConfig{
		Enabled: true, Verifier: definition.Agents[1].ID, MaxCommands: 2, Timeout: "5s",
	}
	store, err := NewStore(t.TempDir(), definition)
	if err != nil {
		t.Fatal(err)
	}
	engine := &Engine{Definition: definition, Store: store}
	verifier := definition.Agents[1]
	results, err := engine.runVerification(context.Background(), verifier, &VerificationRequest{
		Commands: [][]string{{"sh", "-c", "exit 7"}},
	})
	if err == nil || !strings.Contains(err.Error(), "exit code 7") {
		t.Fatalf("err=%v results=%+v", err, results)
	}
	if len(results) != 1 || results[0].ExitCode != 7 {
		t.Fatalf("results=%+v", results)
	}
}

func TestInitialVerifierDoesNotRequireDeliveryCommands(t *testing.T) {
	definition := testDefinition(t)
	definition.Verification = VerificationConfig{
		Enabled: true, Verifier: definition.Agents[1].ID, MaxCommands: 2, Timeout: "5s",
	}
	store, err := NewStore(t.TempDir(), definition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartRound("question"); err != nil {
		t.Fatal(err)
	}
	engine := &Engine{Definition: definition, Store: store, Processor: &fakeProcessor{}}
	response, err := engine.callMember(context.Background(), definition.Agents[1], promptMarkerInitial)
	if err != nil || strings.Contains(response.Content, "独立验证失败") {
		t.Fatalf("initial verifier was treated as delivery verification: response=%+v err=%v", response, err)
	}
}

func TestVerificationMustFollowLatestImplementation(t *testing.T) {
	events := []Event{
		{ID: 1, Round: 1, Type: "agent_message", From: "implementer"},
		{ID: 2, Round: 1, Type: "verification_passed"},
	}
	if !verificationSatisfied(events, 1) {
		t.Fatal("verification after implementation should pass")
	}
	events = append(events, Event{ID: 3, Round: 1, Type: "agent_message", From: "implementer"})
	if verificationSatisfied(events, 1) {
		t.Fatal("new implementation must invalidate prior verification")
	}
	events = append(events, Event{ID: 4, Round: 1, Type: "verification_failed"})
	if verificationSatisfied(events, 1) {
		t.Fatal("failed verification must not satisfy the gate")
	}
}
