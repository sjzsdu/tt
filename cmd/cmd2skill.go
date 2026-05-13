package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	cmd2skillpkg "github.com/sjzsdu/tt/internal/cmd2skill"
	"github.com/spf13/cobra"
)

var (
	cmd2skillTargetDir   string
	cmd2skillDryRun      bool
	cmd2skillExamples    bool
	cmd2skillDepth       int
	cmd2skillMarkdown    bool
	cmd2skillTimeout     time.Duration
	cmd2skillMaxCommands int
)

var cmd2skillCmd = &cobra.Command{
	Use:   "cmd2skill [command...]",
	Short: "Convert a CLI command into agent-oriented skill files",
	Long: `Parse a CLI command and its subcommands into a structured command tree,
then generate agent-oriented skill files with usage, command references, safety
guidance, and deterministic output.`,
	Example: `tt cmd2skill git
tt cmd2skill git --depth 2
tt cmd2skill docker --examples
tt cmd2skill kubectl --target-dir ./.forge/skills
tt cmd2skill git --markdown`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCmd2Skill(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(cmd2skillCmd)
	cmd2skillCmd.Flags().StringVar(&cmd2skillTargetDir, "target-dir", "~/.agents/skills", "directory to write skill files")
	cmd2skillCmd.Flags().BoolVar(&cmd2skillDryRun, "dry-run", false, "print skill content to stdout instead of writing files")
	cmd2skillCmd.Flags().BoolVar(&cmd2skillExamples, "examples", false, "include examples extracted from help output")
	cmd2skillCmd.Flags().IntVarP(&cmd2skillDepth, "depth", "d", 2, "recursion depth for subcommand help (0 = top-level only)")
	cmd2skillCmd.Flags().BoolVar(&cmd2skillMarkdown, "markdown", false, "open generated skill content directly with markdown command instead of writing files")
	cmd2skillCmd.Flags().DurationVar(&cmd2skillTimeout, "timeout", 5*time.Second, "timeout for each help command")
	cmd2skillCmd.Flags().IntVar(&cmd2skillMaxCommands, "max-commands", 200, "maximum number of command help pages to discover")
}

func runCmd2Skill(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("command name required")
	}
	opts := cmd2skillpkg.Options{
		TargetDir:   cmd2skillTargetDir,
		DryRun:      cmd2skillDryRun,
		Examples:    cmd2skillExamples,
		Depth:       cmd2skillDepth,
		Markdown:    cmd2skillMarkdown,
		Timeout:     cmd2skillTimeout,
		MaxCommands: cmd2skillMaxCommands,
	}
	return cmd2skillpkg.Run(strings.Join(args, " "), opts, os.Stdout)
}
