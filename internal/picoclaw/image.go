package picoclaw

import (
	"context"
	"fmt"
	"strings"

	pclogger "github.com/sipeed/picoclaw/pkg/logger"
	pcproviders "github.com/sipeed/picoclaw/pkg/providers"
)

type ImageOptions struct {
	Prompt string
	Model  string
	Debug  bool
	Quiet  bool
}

func (rt *Runtime) GenerateImage(ctx context.Context, opt ImageOptions) (string, error) {
	if rt == nil || rt.Config == nil {
		return "", fmt.Errorf("picoclaw runtime not loaded")
	}
	prompt := strings.TrimSpace(opt.Prompt)
	if prompt == "" {
		return "", fmt.Errorf("image prompt is required")
	}

	cfg := cloneConfig(rt.Config)
	if model := strings.TrimSpace(opt.Model); model != "" {
		cfg.Agents.Defaults.ModelName = model
	}

	pclogger.ConfigureFromEnv()
	if opt.Quiet && !opt.Debug {
		pclogger.DisableConsole()
	}
	if opt.Debug {
		pclogger.SetLevel(pclogger.DEBUG)
	}

	provider, modelID, err := pcproviders.CreateProvider(cfg)
	if err != nil {
		return "", fmt.Errorf("create picoclaw provider failed: %w", err)
	}
	if stateful, ok := provider.(pcproviders.StatefulProvider); ok {
		defer stateful.Close()
	}
	if modelID == "" {
		modelID = cfg.Agents.Defaults.GetModelName()
	}

	resp, err := provider.Chat(ctx, []pcproviders.Message{{Role: "user", Content: prompt}}, nil, modelID, nil)
	if err != nil {
		return "", fmt.Errorf("generate image failed: %w", err)
	}
	if resp == nil || strings.TrimSpace(resp.Content) == "" {
		return "", fmt.Errorf("generate image returned an empty response")
	}
	return strings.TrimSpace(resp.Content), nil
}
