package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var helloName string

var helloCmd = &cobra.Command{
	Use:   "hello",
	Short: "Print a basic greeting",
	Run: func(cmd *cobra.Command, args []string) {
		if helloName == "" {
			helloName = "world"
		}

		fmt.Printf("hello, %s\n", helloName)
	},
}

func init() {
	helloCmd.Flags().StringVarP(&helloName, "name", "n", "world", "name to greet")
	rootCmd.AddCommand(helloCmd)
}
