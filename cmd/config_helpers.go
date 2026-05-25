package cmd

import (
	"fmt"
	"os"

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
	wd, err := os.Getwd()
	if err != nil {
		wd = "."
	}
	return ttconfig.ProjectRoot(loaded, wd)
}

func resolveFormulaDir(loaded ttconfig.Loaded) string {
	wd, err := os.Getwd()
	if err != nil {
		wd = "."
	}
	return ttconfig.FormulaDir(loaded, wd)
}

func resolveAgentDir(loaded ttconfig.Loaded) string {
	wd, err := os.Getwd()
	if err != nil {
		wd = "."
	}
	return ttconfig.AgentDir(loaded, wd)
}

func resolveFormulaRunDir(loaded ttconfig.Loaded) string {
	wd, err := os.Getwd()
	if err != nil {
		wd = "."
	}
	return ttconfig.FormulaRunDir(loaded, wd)
}

func resolvePathAgainstProject(loaded ttconfig.Loaded, p string) string {
	wd, err := os.Getwd()
	if err != nil {
		wd = "."
	}
	return ttconfig.ResolvePath(loaded, p, wd)
}
