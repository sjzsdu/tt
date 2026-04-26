package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	pcwrap "tt/internal/picoclaw"
)

var agentInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show resolved agent runtime information",
	RunE: func(cmd *cobra.Command, args []string) error {
		rt, err := pcwrap.Load(pcwrap.Options{
			Home:   agentHome,
			Config: agentConfig,
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
