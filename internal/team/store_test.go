package team

import (
	"errors"
	"path/filepath"
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
