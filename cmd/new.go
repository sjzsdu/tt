package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var newCmd = &cobra.Command{
	Use:   "new [name]",
	Short: "Show the standard plan for adding a new subcommand",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := strings.TrimSpace(args[0])
		fmt.Printf("subcommand plan for %q\n", name)
		fmt.Println("1. create cmd/" + name + ".go")
		fmt.Println("2. declare a *cobra.Command with Use/Short/RunE")
		fmt.Println("3. add flags in init()")
		fmt.Println("4. register with rootCmd.AddCommand(...) or parentCmd.AddCommand(...)")
		fmt.Println("5. run gofmt ./... && go build ./...")
	},
}

func init() {
	rootCmd.AddCommand(newCmd)
}
