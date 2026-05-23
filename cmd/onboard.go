package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var onboardGlobal bool

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
		if onboardGlobal {
			if err := initTTConfigFile(loaded.Sources.GlobalPath); err != nil {
				return err
			}
			globalTTDir := filepath.Dir(strings.TrimSpace(loaded.Sources.GlobalPath))
			if globalTTDir != "" {
				if err := os.MkdirAll(filepath.Join(globalTTDir, "agents"), 0o755); err != nil {
					return fmt.Errorf("create global agents directory failed: %w", err)
				}
				if err := os.MkdirAll(filepath.Join(globalTTDir, "formulas"), 0o755); err != nil {
					return fmt.Errorf("create global formulas directory failed: %w", err)
				}
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "onboard completed at %s\n", ttDir)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(onboardCmd)
	onboardCmd.Flags().BoolVar(&onboardGlobal, "global", false, "also initialize global ~/.tt config and directories")
}
