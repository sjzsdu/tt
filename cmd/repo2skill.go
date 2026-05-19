package cmd

import (
	"fmt"
	"os"
	"time"

	repo2skillpkg "github.com/sjzsdu/tt/internal/repo2skill"
	"github.com/spf13/cobra"
)

var (
	repo2skillTargetDir   string
	repo2skillDryRun      bool
	repo2skillMarkdown    bool
	repo2skillIntent      string
	repo2skillLanguage    string
	repo2skillMaxFiles    int
	repo2skillMaxFileSize int64
	repo2skillTimeout     time.Duration
	repo2skillKeepTemp    bool
)

var repo2skillCmd = &cobra.Command{
	Use:   "repo2skill [repo-path-or-url]",
	Short: "Convert a repository into an agent-oriented library skill",
	Long: `Analyze a local or remote repository and generate skill files that help an
agent understand what the library solves, which public APIs to use, and how to
apply it in development tasks.`,
	Example: `tt repo2skill ./my-library
tt repo2skill https://github.com/colinhacks/zod
tt repo2skill github.com/gin-gonic/gin --dry-run
tt repo2skill ./repo --target-dir ./.agents/skills`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error { return runRepo2Skill(args) },
}

func init() {
	rootCmd.AddCommand(repo2skillCmd)
	repo2skillCmd.Flags().StringVar(&repo2skillTargetDir, "target-dir", "~/.agents/skills", "directory to write skill files")
	repo2skillCmd.Flags().BoolVar(&repo2skillDryRun, "dry-run", false, "print skill content to stdout instead of writing files")
	repo2skillCmd.Flags().BoolVar(&repo2skillMarkdown, "markdown", false, "open generated skill content with markdown command instead of writing files")
	repo2skillCmd.Flags().StringVar(&repo2skillIntent, "intent", "use-library", "skill intent: use-library, contribute, api-reference, architecture")
	repo2skillCmd.Flags().StringVar(&repo2skillLanguage, "language", "", "preferred output language hint for future agent analysis")
	repo2skillCmd.Flags().IntVar(&repo2skillMaxFiles, "max-files", 200, "maximum relevant files to collect")
	repo2skillCmd.Flags().Int64Var(&repo2skillMaxFileSize, "max-file-size", 256*1024, "maximum bytes per collected file")
	repo2skillCmd.Flags().DurationVar(&repo2skillTimeout, "timeout", 2*time.Minute, "timeout for git clone and analysis steps")
	repo2skillCmd.Flags().BoolVar(&repo2skillKeepTemp, "keep-temp", false, "keep cloned temporary repository for debugging")
}

func runRepo2Skill(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("repository path or URL required")
	}
	return repo2skillpkg.Run(args[0], repo2skillpkg.Options{TargetDir: repo2skillTargetDir, DryRun: repo2skillDryRun, Markdown: repo2skillMarkdown, Intent: repo2skillIntent, Language: repo2skillLanguage, MaxFiles: repo2skillMaxFiles, MaxFileSize: repo2skillMaxFileSize, Timeout: repo2skillTimeout, KeepTemp: repo2skillKeepTemp}, os.Stdout)
}
