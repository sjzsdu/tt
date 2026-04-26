package picoclaw

import (
	"fmt"
	"strings"

	pcconfig "github.com/sipeed/picoclaw/pkg/config"
	pcproviders "github.com/sipeed/picoclaw/pkg/providers"
)

type RunOptions struct {
	Message string
	Session string
	Agent   string
	Model   string
	Debug   bool
}

type ResolvedRunOptions struct {
	Message string `json:"message,omitempty"`
	Session string `json:"session"`
	Agent   string `json:"agent,omitempty"`
	Model   string `json:"model"`
}

func (rt *Runtime) ResolveRunOptions(opt RunOptions) (ResolvedRunOptions, error) {
	if rt == nil || rt.Config == nil {
		return ResolvedRunOptions{}, fmt.Errorf("picoclaw runtime not loaded")
	}

	resolved := ResolvedRunOptions{
		Message: strings.TrimSpace(opt.Message),
		Session: strings.TrimSpace(opt.Session),
		Agent:   strings.TrimSpace(opt.Agent),
		Model:   strings.TrimSpace(opt.Model),
	}

	if resolved.Session == "" {
		resolved.Session = "cli:default"
	}

	agentCfg, err := rt.resolveAgentConfig(resolved.Agent)
	if err != nil {
		return ResolvedRunOptions{}, err
	}
	if agentCfg != nil {
		resolved.Agent = strings.TrimSpace(agentCfg.ID)
	}

	if resolved.Model == "" && agentCfg != nil && agentCfg.Model != nil {
		resolved.Model = strings.TrimSpace(agentCfg.Model.Primary)
	}
	if resolved.Model == "" {
		resolved.Model = rt.PreferredModelName()
	}
	if resolved.Model == "" {
		return ResolvedRunOptions{}, fmt.Errorf("no model specified and no default model configured")
	}

	if opt.Model == "" {
		resolved.Model = rt.safeModelForAgent(resolved.Model)
	}

	if _, err := rt.ResolveModel(resolved.Model); err != nil {
		return ResolvedRunOptions{}, err
	}

	return resolved, nil
}

func (rt *Runtime) safeModelForAgent(name string) string {
	model := strings.TrimSpace(name)
	if model == "" {
		return model
	}
	cfg, err := rt.ResolveModel(model)
	if err != nil || cfg == nil {
		return model
	}
	protocol, _ := pcproviders.ExtractProtocol(cfg)
	if protocol != "ollama" {
		return model
	}
	fallback := strings.TrimSpace(rt.PreferredModelName())
	if fallback == "" || strings.EqualFold(fallback, model) {
		return model
	}
	fallbackCfg, err := rt.ResolveModel(fallback)
	if err != nil || fallbackCfg == nil {
		return model
	}
	fallbackProtocol, _ := pcproviders.ExtractProtocol(fallbackCfg)
	if fallbackProtocol == "ollama" {
		return model
	}
	return fallback
}

func (rt *Runtime) resolveAgentConfig(name string) (*pcconfig.AgentConfig, error) {
	if rt == nil || rt.Config == nil {
		return nil, fmt.Errorf("picoclaw runtime not loaded")
	}
	want := strings.TrimSpace(name)
	if want == "" {
		return defaultAgentConfig(rt.Config), nil
	}
	for i := range rt.Config.Agents.List {
		item := &rt.Config.Agents.List[i]
		if strings.EqualFold(strings.TrimSpace(item.ID), want) {
			return item, nil
		}
		if strings.EqualFold(strings.TrimSpace(item.Name), want) {
			return item, nil
		}
	}
	return nil, fmt.Errorf("agent %q not found", want)
}

func defaultAgentConfig(cfg *pcconfig.Config) *pcconfig.AgentConfig {
	if cfg == nil {
		return nil
	}
	for i := range cfg.Agents.List {
		if cfg.Agents.List[i].Default {
			return &cfg.Agents.List[i]
		}
	}
	if len(cfg.Agents.List) > 0 {
		return &cfg.Agents.List[0]
	}
	return nil
}
