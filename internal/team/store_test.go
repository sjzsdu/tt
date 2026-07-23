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
