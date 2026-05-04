package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	ttconfig "github.com/sjzsdu/tt/internal/ttconfig"
)

var agentInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show resolved agent runtime information",
	RunE: func(cmd *cobra.Command, args []string) error {
		loaded, err := loadTTConfig()
		if err != nil {
			return err
		}
		merged := loaded.Merged
		cli := ttconfig.Config{}
		if cmd.Flags().Changed("picoclaw-home") {
			cli.Picoclaw.Home = agentHome
		}
		if cmd.Flags().Changed("picoclaw-config") {
			cli.Picoclaw.Config = agentConfig
		}
		merged = ttconfig.Merge(merged, cli)

		rt, err := pcwrap.Load(pcwrap.Options{
			Home:      merged.Picoclaw.Home,
			Config:    merged.Picoclaw.Config,
			TTConfig:  merged,
			TTSources: loaded.Sources,
		})
		if err != nil {
			return err
		}

		payload, err := json.MarshalIndent(rt.Summary(), "", "  ")
		if err != nil {
			return fmt.Errorf("marshal picoclaw summary failed: %w", err)
		}

		fmt.Fprintln(cmd.OutOrStdout(), string(payload))
		return nil
	},
}

func init() {
	agentCmd.AddCommand(agentInfoCmd)
}
