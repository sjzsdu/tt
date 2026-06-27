package picoclaw

import (
	"context"
	"reflect"
	"testing"

	pcconfig "github.com/sipeed/picoclaw/pkg/config"
)

func TestDirectStreamerEmitsOnlyDeltas(t *testing.T) {
	var got []string
	streamer := &directStreamer{onDelta: func(delta string) { got = append(got, delta) }}
	if err := streamer.Update(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
	if err := streamer.Update(context.Background(), "hello world"); err != nil {
		t.Fatal(err)
	}
	if err := streamer.Finalize(context.Background(), "hello world"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"hello", " world"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("deltas = %#v, want %#v", got, want)
	}
}

func TestEnableDirectStreamingConfig(t *testing.T) {
	cfg := &pcconfig.Config{
		ModelList: pcconfig.SecureModelList{
			{ModelName: "test-model", Model: "openai/test-model"},
		},
	}
	enableDirectStreaming(cfg, "test-model")
	if cfg.Channels == nil || cfg.Channels["cli"] == nil {
		t.Fatalf("cli channel not configured: %#v", cfg.Channels)
	}
	if !cfg.Channels["cli"].Enabled || cfg.Channels["cli"].Type != "pico" {
		t.Fatalf("cli channel = %#v", cfg.Channels["cli"])
	}
	if !cfg.ModelList[0].Streaming.Enabled {
		t.Fatalf("model streaming not enabled")
	}
}
