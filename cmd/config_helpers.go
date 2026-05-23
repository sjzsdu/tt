package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ttconfig "github.com/sjzsdu/tt/internal/ttconfig"
)

func loadTTConfig() (ttconfig.Loaded, error) {
	return ttconfig.Load("")
}

func mustLoadTTConfig() ttconfig.Loaded {
	loaded, err := loadTTConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: load tt config failed: %v\n", err)
		return ttconfig.Loaded{}
	}
	return loaded
}

func projectRootFromConfig(loaded ttconfig.Loaded) string {
	if loaded.Sources.ProjectPath != "" {
		return filepath.Dir(filepath.Dir(loaded.Sources.ProjectPath))
	}
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

func resolveFormulaDir(loaded ttconfig.Loaded) string {
	if v := strings.TrimSpace(loaded.Merged.Paths.FormulaDir); v != "" {
		return resolvePathAgainstProject(loaded, v)
	}
	return filepath.Join(projectRootFromConfig(loaded), ".tt", "formulas")
}

func resolveAgentDir(loaded ttconfig.Loaded) string {
	if v := strings.TrimSpace(loaded.Merged.Paths.AgentDir); v != "" {
		return resolvePathAgainstProject(loaded, v)
	}
	return filepath.Join(projectRootFromConfig(loaded), ".tt", "agents")
}

func resolveFormulaRunDir(loaded ttconfig.Loaded) string {
	if v := strings.TrimSpace(loaded.Merged.Paths.FormulaRunDir); v != "" {
		return resolvePathAgainstProject(loaded, v)
	}
	return filepath.Join(projectRootFromConfig(loaded), ".tt", "runs", "formula")
}

func resolvePathAgainstProject(loaded ttconfig.Loaded, p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(projectRootFromConfig(loaded), p))
}
