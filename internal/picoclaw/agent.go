package picoclaw

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	pcagent "github.com/sipeed/picoclaw/pkg/agent"
	pcbus "github.com/sipeed/picoclaw/pkg/bus"
	pcconfig "github.com/sipeed/picoclaw/pkg/config"
	pclogger "github.com/sipeed/picoclaw/pkg/logger"
	pcproviders "github.com/sipeed/picoclaw/pkg/providers"
)

func (rt *Runtime) Run(opt RunOptions) error {
	if rt == nil || rt.Config == nil {
		return fmt.Errorf("picoclaw runtime not loaded")
	}

	resolved, err := rt.ResolveRunOptions(opt)
	if err != nil {
		return err
	}

	cfg := cloneConfig(rt.Config)
	cfg = prepareConfigForRun(cfg, resolved)
	embeddedAgents := opt.embeddedAgents()
	if err := applyEmbeddedAgentConfigs(cfg, embeddedAgents, resolved.Model); err != nil {
		return err
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
		return fmt.Errorf("create picoclaw provider failed: %w", err)
	}
	if modelID != "" {
		cfg.Agents.Defaults.ModelName = modelID
	}
	if stateful, ok := provider.(pcproviders.StatefulProvider); ok {
		defer stateful.Close()
	}

	msgBus := pcbus.NewMessageBus()
	defer msgBus.Close()

	loop := pcagent.NewAgentLoop(cfg, msgBus, provider)
	defer loop.Close()
	if err := registerEmbeddedAgentPrompts(loop, embeddedAgents); err != nil {
		return err
	}

	if text := str(resolved.Message); text != "" {
		var resp string
		if str(resolved.Agent) != "" && !strings.EqualFold(str(resolved.Agent), DefaultAgent(cfg)) {
			resp, err = loop.ProcessDirectForAgent(context.Background(), text, resolved.Session, resolved.Agent)
		} else {
			resp, err = loop.ProcessDirect(context.Background(), text, resolved.Session)
		}
		if err != nil {
			return fmt.Errorf("process picoclaw message failed: %w", err)
		}
		resp, err = normalizeDirectResponse(resp, nil)
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, resp)
		return nil
	}

	return runInteractive(loop, resolved.Session)
}

func cloneConfig(cfg *pcconfig.Config) *pcconfig.Config {
	if cfg == nil {
		return nil
	}
	cp := *cfg
	if len(cfg.Agents.List) > 0 {
		cp.Agents.List = append([]pcconfig.AgentConfig(nil), cfg.Agents.List...)
	}
	return &cp
}

// setAgentWorkspaces overrides all agent workspaces to the given directory.
// This is used by the formula runner so agents can access the project files.
func setAgentWorkspaces(cfg *pcconfig.Config, workspace string) {
	if cfg == nil || workspace == "" {
		return
	}
	cfg.Agents.Defaults.Workspace = workspace
	for i := range cfg.Agents.List {
		cfg.Agents.List[i].Workspace = workspace
	}
}

func prepareConfigForRun(cfg *pcconfig.Config, opt ResolvedRunOptions) *pcconfig.Config {
	if cfg == nil {
		return nil
	}
	if str(opt.Model) != "" {
		cfg.Agents.Defaults.ModelName = str(opt.Model)
		for i := range cfg.Agents.List {
			cfg.Agents.List[i].Model = &pcconfig.AgentModelConfig{Primary: str(opt.Model)}
		}
	}
	selected := str(opt.Agent)
	if selected == "" {
		selected = DefaultAgent(cfg)
	}
	if strings.EqualFold(selected, DefaultAgent(cfg)) {
		return cfg
	}
	for i := range cfg.Agents.List {
		item := &cfg.Agents.List[i]
		if !strings.EqualFold(str(item.ID), selected) {
			continue
		}
		item.Default = true
		if str(opt.Model) != "" {
			item.Model = &pcconfig.AgentModelConfig{Primary: str(opt.Model)}
		}
		cfg.Agents.List = []pcconfig.AgentConfig{*item}
		cfg.Agents.Dispatch = nil
		return cfg
	}
	return cfg
}

func runInteractive(loop *pcagent.AgentLoop, sessionKey string) error {
	fmt.Fprintln(os.Stdout, "Interactive mode. Type exit or quit to leave.")
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprint(os.Stdout, "> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Fprintln(os.Stdout)
				return nil
			}
			return fmt.Errorf("read input failed: %w", err)
		}
		input := str(line)
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			return nil
		}
		resp, err := loop.ProcessDirect(context.Background(), input, sessionKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			continue
		}
		resp, err = normalizeDirectResponse(resp, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			continue
		}
		fmt.Fprintln(os.Stdout, resp)
	}
}
