package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sjzsdu/tt/internal/dirmirror"
	"github.com/spf13/cobra"
)

var (
	mirrorSourceDir  string
	mirrorTargetDir  string
	mirrorConfigFile string
	mirrorSourceLv   int
	mirrorTargetLv   int
)

var mirrorCmd = &cobra.Command{
	Use:   "mirror",
	Short: "Mirror selected files from a source directory into this project",
	Long:  "Mirror keeps a project-local directory in sync with a fuller source directory. It is useful for sharing and selectively importing tool configs such as opencode agents and commands.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var mirrorSourceCmd = &cobra.Command{
	Use:   "source",
	Short: "Show source directory tree",
	RunE: func(cmd *cobra.Command, args []string) error {
		paths := resolveMirrorPaths()
		if !pathExists(paths.SourceDir) {
			return fmt.Errorf("source directory not found: %s", paths.SourceDir)
		}
		dd, err := dirmirror.BuildDirData(paths.SourceDir, paths.ConfigFile)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Source directory (%s):\n\n", paths.SourceDir)
		fmt.Fprint(cmd.OutOrStdout(), dirmirror.DisplayDirMapWithDepth(dd.Data, mirrorSourceLv))
		return nil
	},
}

var mirrorTargetCmd = &cobra.Command{
	Use:   "target",
	Short: "Show target directory tree",
	RunE: func(cmd *cobra.Command, args []string) error {
		paths := resolveMirrorPaths()
		if !pathExists(paths.TargetDir) {
			return fmt.Errorf("target directory not found: %s", paths.TargetDir)
		}
		dd, err := dirmirror.BuildDirData(paths.TargetDir, paths.ConfigFile)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Target directory (%s):\n\n", paths.TargetDir)
		fmt.Fprint(cmd.OutOrStdout(), dirmirror.DisplayDirMapWithDepth(dd.Data, mirrorTargetLv))
		return nil
	},
}

var mirrorApplyCmd = &cobra.Command{
	Use:   "apply [keys...]",
	Short: "Mirror all or selected keys from source to target",
	Long:  "With no keys, apply mirrors the entire source directory. With keys, it only copies matching entries such as agents.foo or commands.bar.md.",
	RunE: func(cmd *cobra.Command, args []string) error {
		paths := resolveMirrorPaths()
		if !pathExists(paths.SourceDir) {
			return fmt.Errorf("source directory not found: %s", paths.SourceDir)
		}
		source, err := dirmirror.BuildDirData(paths.SourceDir, paths.ConfigFile)
		if err != nil {
			return fmt.Errorf("read source directory failed: %w", err)
		}

		var target *dirmirror.DirData
		if pathExists(paths.TargetDir) {
			target, err = dirmirror.BuildDirData(paths.TargetDir, paths.ConfigFile)
			if err != nil {
				return fmt.Errorf("read target directory failed: %w", err)
			}
		} else {
			target = dirmirror.NewEmptyDirData(paths.ConfigFile)
		}

		if len(args) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "Mirroring entire source to target...")
			target.Data = source.Data
			target.DirKeys = source.DirKeys
			target.FileKeys = source.FileKeys
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "Mirroring selected keys to target...")
			for _, key := range args {
				if err := target.SyncFrom(source, key); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "skip %s: %v\n", key, err)
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Mirrored: %s\n", key)
			}
		}
		if schema, ok := source.Data["$schema"]; ok {
			target.Data["$schema"] = schema
		}
		if err := target.WriteTo(paths.TargetDir); err != nil {
			return fmt.Errorf("write target directory failed: %w", err)
		}
		if isGitRepository(".") {
			updateGitignoreForMirrorTarget(".", paths.TargetDir)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Mirror completed!")
		return nil
	},
}

var mirrorPruneCmd = &cobra.Command{
	Use:   "prune [keys...]",
	Short: "Remove mirrored target entries",
	Long:  "With no keys, prune removes the entire target directory. With keys, it removes only matching target entries.",
	RunE: func(cmd *cobra.Command, args []string) error {
		paths := resolveMirrorPaths()
		if !pathExists(paths.TargetDir) {
			return fmt.Errorf("target directory not found: %s", paths.TargetDir)
		}
		if len(args) == 0 {
			if err := os.RemoveAll(paths.TargetDir); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Target directory removed.")
			return nil
		}
		target, err := dirmirror.BuildDirData(paths.TargetDir, paths.ConfigFile)
		if err != nil {
			return err
		}
		for _, key := range args {
			if target.DeleteValue(key) {
				fmt.Fprintf(cmd.OutOrStdout(), "Removed: %s\n", key)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Key not found: %s\n", key)
			}
		}
		if err := os.RemoveAll(paths.TargetDir); err != nil {
			return err
		}
		return target.WriteTo(paths.TargetDir)
	},
}

var mirrorConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Show or set mirror paths in project tt config",
	RunE: func(cmd *cobra.Command, args []string) error {
		setSource, _ := cmd.Flags().GetString("set-source-dir")
		setTarget, _ := cmd.Flags().GetString("set-target-dir")
		setConfig, _ := cmd.Flags().GetString("set-config-file")
		if setSource != "" || setTarget != "" || setConfig != "" {
			return saveMirrorProjectConfig(setSource, setTarget, setConfig)
		}
		paths := resolveMirrorPaths()
		fmt.Fprintf(cmd.OutOrStdout(), "Mirror configuration:\n  Source directory: %s\n  Target directory: %s\n  Config file: %s\n", paths.SourceDir, paths.TargetDir, paths.ConfigFile)
		return nil
	},
}

type mirrorPaths struct{ SourceDir, TargetDir, ConfigFile string }

func init() {
	rootCmd.AddCommand(mirrorCmd)
	mirrorCmd.AddCommand(mirrorSourceCmd, mirrorTargetCmd, mirrorApplyCmd, mirrorPruneCmd, mirrorConfigCmd)

	mirrorCmd.PersistentFlags().StringVar(&mirrorSourceDir, "source-dir", "", "source directory path")
	mirrorCmd.PersistentFlags().StringVar(&mirrorTargetDir, "target-dir", "", "target directory path")
	mirrorCmd.PersistentFlags().StringVar(&mirrorConfigFile, "config-file", "", "config file name")
	mirrorSourceCmd.Flags().IntVarP(&mirrorSourceLv, "level", "l", 0, "show items up to this depth level, 0 for all")
	mirrorTargetCmd.Flags().IntVarP(&mirrorTargetLv, "level", "l", 0, "show items up to this depth level, 0 for all")
	mirrorConfigCmd.Flags().String("set-source-dir", "", "set source directory path")
	mirrorConfigCmd.Flags().String("set-target-dir", "", "set target directory path")
	mirrorConfigCmd.Flags().String("set-config-file", "", "set config file name")
}

func resolveMirrorPaths() mirrorPaths {
	home, _ := os.UserHomeDir()
	paths := mirrorPaths{
		SourceDir:  filepath.Join(home, ".config", "opencode-full"),
		TargetDir:  ".opencode",
		ConfigFile: "opencode.json",
	}
	loaded := mustLoadTTConfig()
	if v := strings.TrimSpace(loaded.Merged.Mirror.SourceDir); v != "" {
		paths.SourceDir = v
	}
	if v := strings.TrimSpace(loaded.Merged.Mirror.TargetDir); v != "" {
		paths.TargetDir = v
	}
	if v := strings.TrimSpace(loaded.Merged.Mirror.ConfigFile); v != "" {
		paths.ConfigFile = v
	}
	if mirrorSourceDir != "" {
		paths.SourceDir = mirrorSourceDir
	}
	if mirrorTargetDir != "" {
		paths.TargetDir = mirrorTargetDir
	}
	if mirrorConfigFile != "" {
		paths.ConfigFile = mirrorConfigFile
	}
	return paths
}

func saveMirrorProjectConfig(sourceDir, targetDir, configFile string) error {
	loaded, err := loadTTConfig()
	if err != nil {
		return err
	}
	path := loaded.Sources.ProjectPath
	data := map[string]any{}
	if b, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(b))) > 0 {
		if err := json.Unmarshal(b, &data); err != nil {
			return fmt.Errorf("parse project config failed: %w", err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	mirror, _ := data["mirror"].(map[string]any)
	if mirror == nil {
		mirror = map[string]any{}
	}
	if sourceDir != "" {
		mirror["source_dir"] = sourceDir
	}
	if targetDir != "" {
		mirror["target_dir"] = targetDir
	}
	if configFile != "" {
		mirror["config_file"] = configFile
	}
	data["mirror"] = mirror
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("saved mirror config to %s\n", path)
	return nil
}

func pathExists(path string) bool { _, err := os.Stat(path); return !os.IsNotExist(err) }
func isGitRepository(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info.IsDir()
}

func updateGitignoreForMirrorTarget(dir, targetDir string) {
	gitignorePath := filepath.Join(dir, ".gitignore")
	targetDirRel, err := filepath.Rel(dir, targetDir)
	if err != nil {
		targetDirRel = targetDir
	}
	if !strings.HasSuffix(targetDirRel, "/") {
		targetDirRel += "/"
	}
	content, err := os.ReadFile(gitignorePath)
	lines := []string{}
	if err == nil {
		lines = strings.Split(string(content), "\n")
	} else if !os.IsNotExist(err) {
		return
	}
	for _, line := range lines {
		if strings.TrimSpace(line) == targetDirRel {
			return
		}
	}
	lines = append(lines, "", targetDirRel)
	_ = os.WriteFile(gitignorePath, []byte(strings.Join(lines, "\n")), 0o644)
}
