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

func useTTAgentStorage(home, configPath string) (workspace, resolvedHome, resolvedConfig string, restore func(), err error) {
	workspace, err = ensureTTWorkspace()
	if err != nil {
		return "", "", "", nil, err
	}
	resolvedHome = filepath.Join(workspace, "picoclaw")
	sessionsDir := filepath.Join(workspace, "sessions")
	if err := os.MkdirAll(resolvedHome, 0o755); err != nil {
		return "", "", "", nil, fmt.Errorf("create picoclaw workspace home: %w", err)
	}
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		return "", "", "", nil, fmt.Errorf("create picoclaw sessions dir: %w", err)
	}
	_, resolvedConfig = resolvePicoclawPaths(home, configPath)
	restore, err = setEnvMap(map[string]string{
		"PICOCLAW_HOME":         resolvedHome,
		"PICOCLAW_CONFIG":       resolvedConfig,
		"PICOCLAW_SESSIONS_DIR": sessionsDir,
	})
	if err != nil {
		return "", "", "", nil, err
	}
	return workspace, resolvedHome, resolvedConfig, restore, nil
}

func setEnvMap(values map[string]string) (func(), error) {
	type envState struct {
		value string
		ok    bool
	}
	previous := make(map[string]envState, len(values))
	for key, value := range values {
		prev, ok := os.LookupEnv(key)
		previous[key] = envState{value: prev, ok: ok}
		if err := os.Setenv(key, value); err != nil {
			for restoreKey, state := range previous {
				if state.ok {
					_ = os.Setenv(restoreKey, state.value)
				} else {
					_ = os.Unsetenv(restoreKey)
				}
			}
			return nil, err
		}
	}
	return func() {
		for key, state := range previous {
			if state.ok {
				_ = os.Setenv(key, state.value)
			} else {
				_ = os.Unsetenv(key)
			}
		}
	}, nil
}
