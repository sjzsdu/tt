package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	configShowMerged  bool
	configInitGlobal  bool
	configInitProject bool
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Inspect and initialize tt configuration",
	Long:  "Inspect resolved tt configuration sources and initialize global or project tt config files.",
	RunE: func(cmd *cobra.Command, args []string) error {
		loaded, err := loadTTConfig()
		if err != nil {
			return err
		}

		switch {
		case configInitGlobal:
			return initTTConfigFile(loaded.Sources.GlobalPath)
		case configInitProject:
			return initTTConfigFile(loaded.Sources.ProjectPath)
		case configShowMerged:
			payload, err := json.MarshalIndent(loaded.Merged, "", "  ")
			if err != nil {
				return fmt.Errorf("marshal merged tt config failed: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(payload))
			return nil
		default:
			payload, err := json.MarshalIndent(loaded.Sources, "", "  ")
			if err != nil {
				return fmt.Errorf("marshal tt config sources failed: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(payload))
			return nil
		}
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.Flags().BoolVar(&configShowMerged, "show", false, "show merged tt configuration")
	configCmd.Flags().BoolVar(&configInitGlobal, "init-global", false, "create ~/.tt/config.json if missing")
	configCmd.Flags().BoolVar(&configInitProject, "init-project", false, "create project .tt/config.json if missing")
}

func initTTConfigFile(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("config path is empty")
	}
	if _, err := os.Stat(path); err == nil {
		fmt.Printf("tt config already exists: %s\n", path)
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat tt config failed: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create tt config directory failed: %w", err)
	}
	content := "{\n  \"picoclaw\": {\n    \"home\": \"\",\n    \"config\": \"\"\n  },\n  \"agent\": {\n    \"session\": \"cli:default\"\n  },\n  \"debate\": {\n    \"rounds\": 3,\n    \"output\": \"text\"\n  },\n  \"conversation\": {\n    \"port\": 9680\n  }\n}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write tt config failed: %w", err)
	}
	fmt.Printf("created tt config: %s\n", path)
	return nil
}
