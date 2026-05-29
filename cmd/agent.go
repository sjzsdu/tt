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
	agentMessage string
	agentSession string
	agentName    string
	agentModel   string
	agentDebug   bool
	agentHome    string
	agentConfig  string
	agentList    bool
)

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
	agentCmd.Flags().BoolVarP(&agentDebug, "debug", "d", false, "enable debug logging")
	agentCmd.Flags().StringVar(&agentHome, "picoclaw-home", "", "override PICOCLAW_HOME for this run")
	agentCmd.Flags().StringVar(&agentConfig, "picoclaw-config", "", "override PICOCLAW_CONFIG for this run")
}

func runAgent(cmd *cobra.Command, args []string) error {
	msg := strings.TrimSpace(agentMessage)
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
		cli.Agent.Session = agentSession
	}
	if cmd.Flags().Changed("agent") {
		cli.Agent.Agent = agentName
	}
	if cmd.Flags().Changed("model") {
		cli.Agent.Model = agentModel
	}
	if cmd.Flags().Changed("debug") {
		cli.Agent.Debug = ttconfig.BoolPtr(agentDebug)
	}
	if cmd.Flags().Changed("picoclaw-home") {
		cli.Picoclaw.Home = agentHome
	}
	if cmd.Flags().Changed("picoclaw-config") {
		cli.Picoclaw.Config = agentConfig
	}
	merged = ttconfig.Merge(merged, cli)
	if agentList {
		return runAgentList(cmd, merged, loaded.Sources)
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

	debug := agentDebug
	if merged.Agent.Debug != nil {
		debug = *merged.Agent.Debug
	}

	loading := startLLMLoading("正在等待 agent 回复", debug || msg == "")
	defer loading.Stop()
	if err := rt.Run(pcwrap.RunOptions{
		Message:        msg,
		Session:        merged.Agent.Session,
		Agent:          merged.Agent.Agent,
		Model:          merged.Agent.Model,
		Workspace:      workspace,
		Debug:          debug,
		Quiet:          !debug,
		EmbeddedAgents: agents.All(),
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
			fmt.Fprintf(out, "  %-24s %s\n", agent.ID, agent.Name)
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
