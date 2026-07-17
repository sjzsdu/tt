package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version is intentionally kept as a plain variable so release workflows can override it with ldflags.
var version = "0.2.6"

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
