package picoclaw

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pcagent "github.com/sipeed/picoclaw/pkg/agent"
	pcbus "github.com/sipeed/picoclaw/pkg/bus"
	pclogger "github.com/sipeed/picoclaw/pkg/logger"
	pcproviders "github.com/sipeed/picoclaw/pkg/providers"
)

type DirectRunner struct {
	rt             *Runtime
	defaultAgent   string
	modelOverride  string
	msgBus         *pcbus.MessageBus
	loop           *pcagent.AgentLoop
	closeProvider  func()
	embeddedAgents []EmbeddedAgent
}

func (rt *Runtime) NewDirectRunner(opt RunOptions) (*DirectRunner, error) {
	if rt == nil || rt.Config == nil {
		return nil, fmt.Errorf("picoclaw runtime not loaded")
	}

	resolved, err := rt.ResolveRunOptions(opt)
	if err != nil {
		return nil, err
	}

	cfg := cloneConfig(rt.Config)
	cfg.Agents.Defaults.RestrictToWorkspace = false
	cfg.Agents.Defaults.AllowReadOutsideWorkspace = true
	if cwd, err := os.Getwd(); err == nil {
		ttDir := filepath.Join(cwd, ".tt")
		os.MkdirAll(ttDir, 0o755)
		setAgentWorkspaces(cfg, ttDir)
		cfg.Tools.AllowReadPaths = append(cfg.Tools.AllowReadPaths, cwd)
		cfg.Tools.AllowWritePaths = append(cfg.Tools.AllowWritePaths, cwd)
	}
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
	closeProvider := func() {}
	if stateful, ok := provider.(pcproviders.StatefulProvider); ok {
		closeProvider = stateful.Close
	}
	if modelID != "" {
		cfg.Agents.Defaults.ModelName = modelID
	}

	msgBus := pcbus.NewMessageBus()
	loop := pcagent.NewAgentLoop(cfg, msgBus, provider)
	if err := registerEmbeddedAgentPrompts(loop, embeddedAgents); err != nil {
		loop.Close()
		msgBus.Close()
		closeProvider()
		return nil, err
	}

	return &DirectRunner{
		rt:             rt,
		defaultAgent:   DefaultAgent(cfg),
		modelOverride:  str(opt.Model),
		msgBus:         msgBus,
		loop:           loop,
		closeProvider:  closeProvider,
		embeddedAgents: embeddedAgents,
	}, nil
}

func (dr *DirectRunner) Close() {
	if dr == nil {
		return
	}
	if dr.loop != nil {
		dr.loop.Close()
		dr.loop = nil
	}
	if dr.msgBus != nil {
		dr.msgBus.Close()
		dr.msgBus = nil
	}
	if dr.closeProvider != nil {
		dr.closeProvider()
		dr.closeProvider = nil
	}
}

func (dr *DirectRunner) ProcessDirect(opt RunOptions) (string, error) {
	if dr == nil || dr.rt == nil || dr.loop == nil {
		return "", fmt.Errorf("picoclaw direct runner not initialized")
	}

	if str(opt.Model) == "" {
		opt.Model = dr.modelOverride
	}
	if len(opt.EmbeddedAgents) == 0 && len(dr.embeddedAgents) > 0 {
		opt.EmbeddedAgents = dr.embeddedAgents
	}
	resolved, err := dr.rt.ResolveRunOptions(opt)
	if err != nil {
		return "", err
	}

	if str(resolved.Agent) != "" && !strings.EqualFold(str(resolved.Agent), dr.defaultAgent) {
		resp, err := dr.loop.ProcessDirectForAgent(context.Background(), resolved.Message, resolved.Session, resolved.Agent)
		if err != nil {
			return "", fmt.Errorf("process picoclaw message failed: %w", err)
		}
		resp, err = normalizeDirectResponse(resp, nil)
		if err != nil {
			return "", err
		}
		return resp, nil
	}

	resp, err := dr.loop.ProcessDirect(context.Background(), resolved.Message, resolved.Session)
	if err != nil {
		return "", fmt.Errorf("process picoclaw message failed: %w", err)
	}
	resp, err = normalizeDirectResponse(resp, nil)
	if err != nil {
		return "", err
	}
	return resp, nil
}
