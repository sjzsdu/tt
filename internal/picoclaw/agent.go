package picoclaw

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	if workspace := resolveRunWorkspace(opt.Workspace); workspace != "" {
		configureProjectWorkspace(cfg, workspace)
	}
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
			if isEmptyDirectResponseError(err) {
				resp, err = recoverEmptyDirectResponse(loop, resolved.Session, str(resolved.Agent), DefaultAgent(cfg))
				if err == nil {
					callBeforeOutput(opt.BeforeOutput)
					fmt.Fprintln(os.Stdout, resp)
					return nil
				}
			}
			return err
		}
		callBeforeOutput(opt.BeforeOutput)
		fmt.Fprintln(os.Stdout, resp)
		return nil
	}

	return runInteractive(loop, resolved.Session)
}

func callBeforeOutput(fn func()) {
	if fn != nil {
		fn()
	}
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

// configureProjectWorkspace points picoclaw agents at the requested workspace
// while still allowing tools to access the surrounding project when needed.
// Formula runs use <project>/.tt as the agent workspace, but must retain read
// and write access to the project root for real code changes.
func configureProjectWorkspace(cfg *pcconfig.Config, workspace string) {
	if cfg == nil || workspace == "" {
		return
	}
	setAgentWorkspaces(cfg, workspace)
	cfg.Agents.Defaults.RestrictToWorkspace = false
	cfg.Agents.Defaults.AllowReadOutsideWorkspace = true
	for _, path := range workspaceAccessPaths(workspace) {
		cfg.Tools.AllowReadPaths = append(cfg.Tools.AllowReadPaths, path)
		cfg.Tools.AllowWritePaths = append(cfg.Tools.AllowWritePaths, path)
	}
}

func workspaceAccessPaths(workspace string) []string {
	workspace = filepath.Clean(strings.TrimSpace(workspace))
	if workspace == "" {
		return nil
	}
	paths := []string{workspace}
	if filepath.Base(workspace) == ".tt" {
		parent := filepath.Dir(workspace)
		if parent != "" && parent != workspace {
			paths = append(paths, parent)
		}
	}
	return paths
}

func resolveRunWorkspace(workspace string) string {
	if workspace = str(workspace); workspace != "" {
		return workspace
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return ""
}

// setAgentWorkspaces overrides all agent workspaces to the given directory.
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
			if isEmptyDirectResponseError(err) {
				resp, err = recoverEmptyDirectResponse(loop, sessionKey, "", "")
				if err == nil {
					fmt.Fprintln(os.Stdout, resp)
					continue
				}
			}
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			continue
		}
		fmt.Fprintln(os.Stdout, resp)
	}
}
