package cmd

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	teamruntime "github.com/sjzsdu/tt/internal/team"
)

type interruptibleTeamProcessor struct {
	once    sync.Once
	started chan struct{}
}

func (p *interruptibleTeamProcessor) Process(ctx context.Context, call teamruntime.AgentCall) (string, error) {
	block := false
	p.once.Do(func() {
		block = true
		close(p.started)
	})
	if block {
		<-ctx.Done()
		return "", ctx.Err()
	}
	switch {
	case strings.Contains(call.Prompt, "[TEAM_PHASE:INITIAL]"):
		return "Initial assessment.", nil
	case strings.Contains(call.Prompt, "[TEAM_PHASE:REVIEW]"):
		return "[TEAM_SIGNAL:YIELD]", nil
	case strings.Contains(call.Prompt, "[TEAM_PHASE:FINAL]"):
		return "Final answer.", nil
	case strings.Contains(call.Prompt, "[TEAM_PHASE:MEMORY]"):
		return "# Team Memory\n\nRemember the final answer.", nil
	default:
		return "", errors.New("unexpected team prompt")
	}
}

func TestTeamRunControllerStopResumeAndFollowUpTransitions(t *testing.T) {
	definition, err := teamruntime.Parse([]byte(starterTeamDefinition("controller-test")))
	if err != nil {
		t.Fatal(err)
	}
	store, err := teamruntime.NewStore(t.TempDir(), definition)
	if err != nil {
		t.Fatal(err)
	}
	processor := &interruptibleTeamProcessor{started: make(chan struct{})}
	engine := &teamruntime.Engine{
		Definition:    definition,
		Store:         store,
		Processor:     processor,
		DisableMemory: true,
	}
	controller := newTeamRunController(engine, context.Background())
	if controls := controller.Controls(); !controls.CanFollowUp || controls.Busy {
		t.Fatalf("initial controls = %+v", controls)
	}
	if err := controller.FollowUp("Start a round."); err != nil {
		t.Fatal(err)
	}
	select {
	case <-processor.started:
	case <-time.After(2 * time.Second):
		t.Fatal("team processor did not start")
	}
	if controls := controller.Controls(); !controls.Busy || !controls.CanStop || controls.CanResume {
		t.Fatalf("running controls = %+v", controls)
	}
	if err := controller.Resume(); err == nil {
		t.Fatal("expected resume conflict while running")
	}
	if err := controller.Stop(); err != nil {
		t.Fatal(err)
	}
	controller.Wait()
	if store.Thread.Status != teamruntime.ThreadStatusInterrupted {
		t.Fatalf("thread status after stop = %s", store.Thread.Status)
	}
	if controls := controller.Controls(); !controls.CanResume || controls.CanStop {
		t.Fatalf("interrupted controls = %+v", controls)
	}
	if err := controller.Resume(); err != nil {
		t.Fatal(err)
	}
	controller.Wait()
	if store.Thread.Status != teamruntime.ThreadStatusIdle {
		t.Fatalf("thread status after resume = %s", store.Thread.Status)
	}
	if controls := controller.Controls(); !controls.CanFollowUp || controls.CanResume {
		t.Fatalf("completed controls = %+v", controls)
	}
	if err := controller.Stop(); err == nil {
		t.Fatal("expected stop conflict without active run")
	}
}
