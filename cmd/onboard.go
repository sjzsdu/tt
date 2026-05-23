package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var onboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "Initialize .tt workspace with full config and standard directories",
	RunE: func(cmd *cobra.Command, args []string) error {
		loaded, err := loadTTConfig()
		if err != nil {
			return err
		}
		projectRoot := projectRootFromConfig(loaded)
		ttDir := filepath.Join(projectRoot, ".tt")

		dirs := []string{
			ttDir,
			resolveFormulaDir(loaded),
			resolveAgentDir(loaded),
			resolveFormulaRunDir(loaded),
			filepath.Join(ttDir, "sessions"),
			filepath.Join(ttDir, "picoclaw"),
		}
		for _, dir := range dirs {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("create directory %s failed: %w", dir, err)
			}
		}

		if err := initTTConfigFile(loaded.Sources.ProjectPath); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "onboard completed at %s\n", ttDir)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(onboardCmd)
}
