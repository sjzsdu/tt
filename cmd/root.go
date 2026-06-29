package cmd

import "github.com/spf13/cobra"

const rootLongBase = `tt is a collection-style CLI toolkit.

It is designed as a root command that can host many subcommands over time,
making it suitable for task grouping, automation, and tool aggregation.`

var rootCmd = &cobra.Command{
	Use:           "tt",
	Short:         "tt is a collection-style CLI toolkit",
	Long:          rootLongBase,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() error {
	registerEmbeddedAgentShortcutCommands()
	registerFormulaShortcutCommands()
	return rootCmd.Execute()
}
