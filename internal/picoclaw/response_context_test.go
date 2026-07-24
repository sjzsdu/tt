package picoclaw

import (
	"context"
	"errors"
	"testing"
)

type cancellationAwareDirectProcessor struct {
	received context.Context
}

func (p *cancellationAwareDirectProcessor) ProcessDirect(ctx context.Context, _, _ string) (string, error) {
	p.received = ctx
	<-ctx.Done()
	return "", ctx.Err()
}

func (p *cancellationAwareDirectProcessor) ProcessDirectForAgent(ctx context.Context, _, _, _ string) (string, error) {
	return p.ProcessDirect(ctx, "", "")
}

func TestProcessDirectPropagatesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	processor := &cancellationAwareDirectProcessor{}

	_, err := processDirect(ctx, processor, "question", "session", "", "assistant")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("processDirect() error = %v, want context.Canceled", err)
	}
	if processor.received != ctx {
		t.Fatal("processor did not receive the caller context")
	}
}
