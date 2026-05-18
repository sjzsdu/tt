package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var version = "0.1.4"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print tt version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
