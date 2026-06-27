package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/tt/internal/agents"
	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	ttconfig "github.com/sjzsdu/tt/internal/ttconfig"
)

var (
	agentMessage             string
	agentSession             string
	agentName                string
	agentModel               string
	agentDebug               bool
	agentHome                string
	agentConfig              string
	agentList                bool
	agentWeb                 bool
	agentWebPort             int
	agentShortcutsRegistered bool
)

type agentRunFlags struct {
	Message string
	Session string
	Agent   string
	Model   string
	Debug   bool
	Home    string
	Config  string
	List    bool
	Web     bool
	WebPort int
}

var agentCmd = &cobra.Command{
	Use:     "agent [message]",
	Aliases: []string{"pc"},
	Short:   "Run the embedded picoclaw agent runtime",
	Long: `Run the embedded picoclaw agent runtime and reuse the existing .picoclaw
configuration, models, sessions, and skills without invoking the picoclaw binary.`,
	Args: cobra.ArbitraryArgs,
	Example: `tt agent -m "summarize this project"
tt agent "explain the current directory"
tt agent --list
tt agent --web
tt agent --agent coder --web
tt agent --session cli:tt --model gpt-5.4 -m "review this idea"
tt agent --picoclaw-home ~/.picoclaw-dev -m "list available skills"
tt agent optimize --target ./repo --agent coder --output .tt/agents`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAgent(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(agentCmd)
	agentCmd.Flags().StringVarP(&agentMessage, "message", "m", "", "send a single message to the agent")
	agentCmd.Flags().StringVarP(&agentSession, "session", "s", "", "session key; defaults to cli:default")
	agentCmd.Flags().StringVar(&agentName, "agent", "", "agent id or name to use")
	agentCmd.Flags().StringVar(&agentModel, "model", "", "model to use; defaults to the selected agent model or config default")
	agentCmd.Flags().BoolVar(&agentList, "list", false, "list embedded agents and agents configured in picoclaw")
	agentCmd.Flags().BoolVar(&agentWeb, "web", false, "start a local web chat UI for agent conversations")
	agentCmd.Flags().IntVar(&agentWebPort, "web-port", 9710, "preferred port for the agent web UI")
	agentCmd.Flags().BoolVarP(&agentDebug, "debug", "d", false, "enable debug logging")
	agentCmd.Flags().StringVar(&agentHome, "picoclaw-home", "", "override PICOCLAW_HOME for this run")
	agentCmd.Flags().StringVar(&agentConfig, "picoclaw-config", "", "override PICOCLAW_CONFIG for this run")
}

func runAgent(cmd *cobra.Command, args []string) error {
	return runAgentWithFlags(cmd, args, agentRunFlags{
		Message: agentMessage,
		Session: agentSession,
		Agent:   agentName,
		Model:   agentModel,
		Debug:   agentDebug,
		Home:    agentHome,
		Config:  agentConfig,
		List:    agentList,
		Web:     agentWeb,
		WebPort: agentWebPort,
	})
}

func registerEmbeddedAgentShortcutCommands() {
	if agentShortcutsRegistered {
		return
	}
	agentShortcutsRegistered = true
	embeddedAgents, err := agents.Embedded()
	if err != nil {
		return
	}
	existing := map[string]bool{}
	for _, command := range rootCmd.Commands() {
		existing[command.Name()] = true
		for _, alias := range command.Aliases {
			existing[alias] = true
		}
	}
	for _, embeddedAgent := range embeddedAgents {
		agent := embeddedAgent
		for _, alias := range agent.Aliases {
			alias = strings.TrimSpace(alias)
			if alias == "" || existing[alias] {
				continue
			}
			flags := agentRunFlags{Agent: agent.ID}
			shortcut := &cobra.Command{
				Use:   alias + " [message]",
				Short: fmt.Sprintf("Run the %s embedded agent", agent.ID),
				Long:  fmt.Sprintf("Run the %s embedded agent. This is a shortcut for `tt agent --agent %s`.", agent.ID, agent.ID),
				Args:  cobra.ArbitraryArgs,
				RunE: func(cmd *cobra.Command, args []string) error {
					return runAgentWithFlags(cmd, args, flags)
				},
			}
			if description := strings.TrimSpace(agent.Description); description != "" {
				shortcut.Short = description
			}
			shortcut.Flags().StringVarP(&flags.Message, "message", "m", "", "send a single message to the agent")
			shortcut.Flags().StringVarP(&flags.Session, "session", "s", "", "session key; defaults to cli:default")
			shortcut.Flags().StringVar(&flags.Model, "model", "", "model to use; defaults to the selected agent model or config default")
			shortcut.Flags().BoolVar(&flags.Web, "web", false, "start a local web chat UI for agent conversations")
			shortcut.Flags().IntVar(&flags.WebPort, "web-port", 9710, "preferred port for the agent web UI")
			shortcut.Flags().BoolVarP(&flags.Debug, "debug", "d", false, "enable debug logging")
			shortcut.Flags().StringVar(&flags.Home, "picoclaw-home", "", "override PICOCLAW_HOME for this run")
			shortcut.Flags().StringVar(&flags.Config, "picoclaw-config", "", "override PICOCLAW_CONFIG for this run")
			rootCmd.AddCommand(shortcut)
			existing[alias] = true
		}
	}
}

func runAgentWithFlags(cmd *cobra.Command, args []string, flags agentRunFlags) error {
	msg := strings.TrimSpace(flags.Message)
	if msg == "" && len(args) > 0 {
		msg = strings.TrimSpace(strings.Join(args, " "))
	}

	loaded, err := loadTTConfig()
	if err != nil {
		return err
	}
	merged := loaded.Merged
	cli := ttconfig.Config{}
	if msg != "" {
		// message remains CLI-only and is not persisted in tt config.
	}
	if cmd.Flags().Changed("session") {
		cli.Agent.Session = flags.Session
	}
	if flags.Agent != "" || cmd.Flags().Changed("agent") {
		cli.Agent.Agent = flags.Agent
	}
	if cmd.Flags().Changed("model") {
		cli.Agent.Model = flags.Model
	}
	if cmd.Flags().Changed("debug") {
		cli.Agent.Debug = ttconfig.BoolPtr(flags.Debug)
	}
	if cmd.Flags().Changed("picoclaw-home") {
		cli.Picoclaw.Home = flags.Home
	}
	if cmd.Flags().Changed("picoclaw-config") {
		cli.Picoclaw.Config = flags.Config
	}
	merged = ttconfig.Merge(merged, cli)
	if flags.List {
		return runAgentList(cmd, merged, loaded.Sources)
	}
	if flags.Web {
		return runAgentWeb(cmd, merged, loaded.Sources, flags)
	}
	workspace, resolvedHome, resolvedConfig, restoreStorage, err := useTTAgentStorage(merged.Picoclaw.Home, merged.Picoclaw.Config)
	if err != nil {
		return err
	}
	defer restoreStorage()
	merged.Picoclaw.Home = resolvedHome
	merged.Picoclaw.Config = resolvedConfig
	if err := ensurePicoclawConfigAvailable(merged.Picoclaw.Home, merged.Picoclaw.Config); err != nil {
		return err
	}

	rt, err := pcwrap.Load(pcwrap.Options{
		Home:      merged.Picoclaw.Home,
		Config:    merged.Picoclaw.Config,
		TTConfig:  merged,
		TTSources: loaded.Sources,
	})
	if err != nil {
		return picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}

	debug := flags.Debug
	if merged.Agent.Debug != nil {
		debug = *merged.Agent.Debug
	}

	loading := startLLMLoading("正在等待 agent 回复", debug || msg == "")
	defer loading.Stop()

	allAgents, err := agents.All()
	if err != nil {
		return fmt.Errorf("load agents failed: %w", err)
	}

	if err := rt.Run(pcwrap.RunOptions{
		Message:        msg,
		Session:        merged.Agent.Session,
		Agent:          merged.Agent.Agent,
		Model:          merged.Agent.Model,
		Workspace:      workspace,
		Debug:          debug,
		Quiet:          !debug,
		EmbeddedAgents: allAgents,
		BeforeOutput:   loading.Stop,
	}); err != nil {
		return picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	return nil
}

func runAgentList(cmd *cobra.Command, cfg ttconfig.Config, sources ttconfig.Sources) error {
	out := cmd.OutOrStdout()
	embeddedAgents, err := agents.List()
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "Embedded agents:")
	if len(embeddedAgents) == 0 {
		fmt.Fprintln(out, "  (none)")
	} else {
		for _, agent := range embeddedAgents {
			description := strings.TrimSpace(agent.Description)
			if description == "" {
				fmt.Fprintf(out, "  %-24s %s\n", agent.ID, agent.Name)
				continue
			}
			fmt.Fprintf(out, "  %-24s %-24s %s\n", agent.ID, agent.Name, description)
		}
	}

	rt, err := pcwrap.Load(pcwrap.Options{
		Home:      cfg.Picoclaw.Home,
		Config:    cfg.Picoclaw.Config,
		TTConfig:  cfg,
		TTSources: sources,
	})
	if err != nil {
		return picoclawUnavailableError(err, cfg.Picoclaw.Home, cfg.Picoclaw.Config)
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "Picoclaw configured agents:")
	configured := rt.Summary().Agents
	if len(configured) == 0 {
		fmt.Fprintln(out, "  (none)")
	} else {
		defaultAgent := pcwrap.DefaultAgent(rt.Config)
		for _, name := range configured {
			marker := ""
			if strings.EqualFold(name, defaultAgent) {
				marker = " (default)"
			}
			fmt.Fprintf(out, "  %s%s\n", name, marker)
		}
	}
	return nil
}
