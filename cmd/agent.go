package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	pcwrap "tt/internal/picoclaw"
)

var (
	agentMessage string
	agentSession string
	agentName    string
	agentModel   string
	agentDebug   bool
	agentHome    string
	agentConfig  string
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
tt agent --session cli:tt --model gpt-5.4 -m "review this idea"
tt agent --picoclaw-home ~/.picoclaw-dev -m "list available skills"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAgent(args)
	},
}

func init() {
	rootCmd.AddCommand(agentCmd)
	agentCmd.Flags().StringVarP(&agentMessage, "message", "m", "", "send a single message to the agent")
	agentCmd.Flags().StringVarP(&agentSession, "session", "s", "", "session key; defaults to cli:default")
	agentCmd.Flags().StringVar(&agentName, "agent", "", "agent id or name to use")
	agentCmd.Flags().StringVar(&agentModel, "model", "", "model to use; defaults to the selected agent model or config default")
	agentCmd.Flags().BoolVarP(&agentDebug, "debug", "d", false, "enable debug logging")
	agentCmd.Flags().StringVar(&agentHome, "picoclaw-home", "", "override PICOCLAW_HOME for this run")
	agentCmd.Flags().StringVar(&agentConfig, "picoclaw-config", "", "override PICOCLAW_CONFIG for this run")
}

func runAgent(args []string) error {
	msg := strings.TrimSpace(agentMessage)
	if msg == "" && len(args) > 0 {
		msg = strings.TrimSpace(strings.Join(args, " "))
	}

	rt, err := pcwrap.Load(pcwrap.Options{
		Home:   agentHome,
		Config: agentConfig,
	})
	if err != nil {
		return err
	}

	return rt.Run(pcwrap.RunOptions{
		Message: msg,
		Session: agentSession,
		Agent:   agentName,
		Model:   agentModel,
		Debug:   agentDebug,
	})
}
