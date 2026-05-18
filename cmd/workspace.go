package cmd

import (
	"fmt"
	"os"
	"path/filepath"
)

func ensureTTWorkspace() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get current directory: %w", err)
	}
	workspace := filepath.Join(cwd, ".tt")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return "", fmt.Errorf("create tt workspace: %w", err)
	}
	return workspace, nil
}
