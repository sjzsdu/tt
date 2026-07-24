package team

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testDefinition(t *testing.T) *Definition {
	t.Helper()
	definition, err := Parse([]byte(validTeamTOML))
	if err != nil {
		t.Fatal(err)
	}
	return definition
}

func TestStoreRoundLifecycleAndResolve(t *testing.T) {
	workspace := t.TempDir()
	store, err := NewStore(workspace, testDefinition(t))
	if err != nil {
		t.Fatal(err)
	}
	round, err := store.StartRound("What should we build?")
	if err != nil {
		t.Fatal(err)
	}
	if round.Number != 1 || store.Thread.Status != ThreadStatusRunning {
		t.Fatalf("round/thread = %+v %+v", round, store.Thread)
	}
	if err := store.AppendEvent(Event{Type: "agent_message", Phase: PhaseInitial, From: "expert", Content: "Build the smallest slice."}); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkInterrupted(errors.New("stopped")); err != nil {
		t.Fatal(err)
	}
	if err := store.PrepareResume(); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteRound("Ship an MVP.", 1); err != nil {
		t.Fatal(err)
	}
	resolved, err := ResolveStore(workspace, filepath.Base(store.Thread.ID))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Thread.Status != ThreadStatusIdle || resolved.State.Current.FinalAnswer != "Ship an MVP." {
		t.Fatalf("resolved = %+v %+v", resolved.Thread, resolved.State.Current)
	}
	events, err := resolved.Events()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) < 6 {
		t.Fatalf("events = %+v", events)
	}
}

func TestStorePinsDefinition(t *testing.T) {
	store, err := NewStore(t.TempDir(), testDefinition(t))
	if err != nil {
		t.Fatal(err)
	}
	definition, err := store.LoadDefinition()
	if err != nil {
		t.Fatal(err)
	}
	if definition.Team != "review" || definition.DefinitionHash != store.Thread.DefinitionHash {
		t.Fatalf("definition/thread = %+v %+v", definition, store.Thread)
	}
}

func TestStoreSnapshotClonesCollaborationState(t *testing.T) {
	store, err := NewStore(t.TempDir(), testDefinition(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartRound("Review this."); err != nil {
		t.Fatal(err)
	}
	state := CollaborationState{
		TurnCount: 2,
		Pending:   []Activation{{MemberID: "expert", Reason: activationDirectMention}},
		Objections: []Objection{{
			EventID: 7,
			From:    "expert",
			Targets: []string{"lead"},
		}},
	}
	if err := store.SetCollaboration(state); err != nil {
		t.Fatal(err)
	}

	_, snapshot, _, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Current.Collaboration.Pending[0].MemberID = "changed"
	snapshot.Current.Collaboration.Objections[0].Targets[0] = "changed"
	if store.State.Current.Collaboration.Pending[0].MemberID != "expert" ||
		store.State.Current.Collaboration.Objections[0].Targets[0] != "lead" {
		t.Fatalf("snapshot mutation leaked into store: %+v", store.State.Current.Collaboration)
	}
}

func TestStoreLeaseRejectsConcurrentWriterAndRecoversStaleOwner(t *testing.T) {
	store, err := NewStore(t.TempDir(), testDefinition(t))
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.AcquireLease()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AcquireLease(); err == nil || !strings.Contains(err.Error(), "locked by process") {
		t.Fatalf("expected lock contention, got %v", err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	stale := threadLeaseRecord{PID: 99999999, Token: "stale", AcquiredAt: "2020-01-01T00:00:00Z"}
	data, _ := json.Marshal(stale)
	if err := os.WriteFile(filepath.Join(store.Dir, "thread.lock"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.AcquireLease()
	if err != nil {
		t.Fatalf("recover stale lease: %v", err)
	}
	if err := recovered.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenStoreMigratesAndReconcilesCommittedEvents(t *testing.T) {
	store, err := NewStore(t.TempDir(), testDefinition(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartRound("Recover me"); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(Event{Type: "agent_message", Phase: PhaseInitial, From: "expert", Content: "Committed response"}); err != nil {
		t.Fatal(err)
	}
	completed := Event{
		ID: 99, Type: "round_completed", At: "2026-07-24T12:00:00Z",
		ThreadID: store.Thread.ID, Round: 1, Phase: PhaseComplete, Content: "Recovered answer",
	}
	line, _ := json.Marshal(completed)
	eventPath := filepath.Join(store.Dir, "events.jsonl")
	file, err := os.OpenFile(eventPath, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(append(append(line, '\n'), []byte(`{"truncated":`)...)); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()

	var legacyThread Thread
	if err := readJSON(filepath.Join(store.Dir, "thread.json"), &legacyThread); err != nil {
		t.Fatal(err)
	}
	legacyThread.SchemaVersion = 0
	legacyThread.DefinitionVersion = 0
	if err := writeJSONAtomic(filepath.Join(store.Dir, "thread.json"), legacyThread); err != nil {
		t.Fatal(err)
	}
	var legacyState State
	if err := readJSON(filepath.Join(store.Dir, "state.json"), &legacyState); err != nil {
		t.Fatal(err)
	}
	legacyState.SchemaVersion = 0
	legacyState.NextEventID = 3
	if err := writeJSONAtomic(filepath.Join(store.Dir, "state.json"), legacyState); err != nil {
		t.Fatal(err)
	}

	recovered, err := OpenStore(store.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Thread.SchemaVersion != CurrentStoreSchemaVersion ||
		recovered.State.SchemaVersion != CurrentStoreSchemaVersion ||
		recovered.Thread.DefinitionVersion == 0 {
		t.Fatalf("migration failed: %+v %+v", recovered.Thread, recovered.State)
	}
	if recovered.State.NextEventID != 100 || recovered.State.Current.Status != RoundStatusCompleted ||
		recovered.State.Current.FinalAnswer != "Recovered answer" || recovered.Thread.LastAnswer != "Recovered answer" {
		t.Fatalf("reconciliation failed: %+v %+v", recovered.Thread, recovered.State)
	}
	events, err := recovered.Events()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) < 4 || events[len(events)-1].ID != 99 {
		t.Fatalf("committed events lost: %+v", events)
	}
}

func TestOpenStoreRejectsFutureSchema(t *testing.T) {
	store, err := NewStore(t.TempDir(), testDefinition(t))
	if err != nil {
		t.Fatal(err)
	}
	store.Thread.SchemaVersion = CurrentStoreSchemaVersion + 1
	if err := writeJSONAtomic(filepath.Join(store.Dir, "thread.json"), store.Thread); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenStore(store.Dir); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("expected future schema error, got %v", err)
	}
}

func TestExternalSessionsPersist(t *testing.T) {
	store, err := NewStore(t.TempDir(), testDefinition(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetExternalSession(" Implementer ", "session-123"); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(store.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.ExternalSession("implementer"); got != "session-123" {
		t.Fatalf("external session = %q", got)
	}
}
