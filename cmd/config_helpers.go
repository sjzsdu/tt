package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	ttconfig "tt/internal/ttconfig"
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
