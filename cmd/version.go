package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version is injected from the git tag at build time (see Makefile and the
// release workflow) so the version lives in exactly one place. The fallback
// keeps `go build`/`go run` usable outside a tagged checkout.
var version = "dev"

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
