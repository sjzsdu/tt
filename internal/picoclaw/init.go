package picoclaw

import (
	"context"
	"fmt"
	"strings"

	pcagent "github.com/sipeed/picoclaw/pkg/agent"
	pcbus "github.com/sipeed/picoclaw/pkg/bus"
	pcconfig "github.com/sipeed/picoclaw/pkg/config"
	pclogger "github.com/sipeed/picoclaw/pkg/logger"
	pcproviders "github.com/sipeed/picoclaw/pkg/providers"
)

type initRuntimeResult struct {
	cfg              *pcconfig.Config
	modelOverride    string
	defaultAgent     string
	msgBus           *pcbus.MessageBus
	loop             *pcagent.AgentLoop
	closeProvider    func()
	embeddedAgents   []EmbeddedAgent
}

func (rt *Runtime) initRuntime(opt RunOptions) (*initRuntimeResult, error) {
	if rt == nil || rt.Config == nil {
		return nil, fmt.Errorf("picoclaw runtime not loaded")
	}

	resolved, err := rt.ResolveRunOptions(opt)
	if err != nil {
		return nil, err
	}

	cfg := cloneConfig(rt.Config)
	cfg = prepareConfigForRun(cfg, resolved)
	embeddedAgents := opt.embeddedAgents()
	if err := applyEmbeddedAgentConfigs(cfg, embeddedAgents, resolved.Model); err != nil {
		return nil, err
	}

	pclogger.ConfigureFromEnv()
	if opt.Quiet && !opt.Debug {
		pclogger.DisableConsole()
	}
	if opt.Debug {
		pclogger.SetLevel(pclogger.DEBUG)
	}

	if str(resolved.Model) != "" {
		cfg.Agents.Defaults.ModelName = str(resolved.Model)
	}

	provider, modelID, err := pcproviders.CreateProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("create picoclaw provider failed: %w", err)
	}
	if modelID != "" {
		cfg.Agents.Defaults.ModelName = modelID
	}

	closeProvider := func() {}
	if stateful, ok := provider.(pcproviders.StatefulProvider); ok {
		closeProvider = stateful.Close
	}

	msgBus := pcbus.NewMessageBus()
	loop := pcagent.NewAgentLoop(cfg, msgBus, provider)
	if err := registerEmbeddedAgentPrompts(loop, embeddedAgents); err != nil {
		loop.Close()
		msgBus.Close()
		closeProvider()
		return nil, err
	}

	return &initRuntimeResult{
		cfg:            cfg,
		modelOverride:   str(opt.Model),
		defaultAgent:   DefaultAgent(cfg),
		msgBus:         msgBus,
		loop:           loop,
		closeProvider:  closeProvider,
		embeddedAgents: embeddedAgents,
	}, nil
}

func (r *initRuntimeResult) isClosed() bool {
	return r == nil || r.loop == nil
}

func (r *initRuntimeResult) processDirect(ctx context.Context, message, session, agentID string) (string, error) {
	if r.isClosed() {
		return "", fmt.Errorf("picoclaw runtime not initialized")
	}

	text := str(message)
	if str(agentID) != "" && !strings.EqualFold(str(agentID), r.defaultAgent) {
		resp, err := r.loop.ProcessDirectForAgent(ctx, text, session, agentID)
		if err != nil {
			return "", fmt.Errorf("process picoclaw message failed: %w", err)
		}
		return resp, nil
	}

	resp, err := r.loop.ProcessDirect(ctx, text, session)
	if err != nil {
		return "", fmt.Errorf("process picoclaw message failed: %w", err)
	}
	return resp, nil
}

func (r *initRuntimeResult) close() {
	if r == nil {
		return
	}
	if r.loop != nil {
		r.loop.Close()
		r.loop = nil
	}
	if r.msgBus != nil {
		r.msgBus.Close()
		r.msgBus = nil
	}
	if r.closeProvider != nil {
		r.closeProvider()
		r.closeProvider = nil
	}
}
