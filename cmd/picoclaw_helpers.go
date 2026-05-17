package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
)

func picoclawUnavailableError(err error, home, configPath string) error {
	if err == nil {
		return nil
	}
	if !shouldWrapPicoclawError(err) {
		return err
	}
	resolvedHome, resolvedConfig := resolvePicoclawPaths(home, configPath)

	return fmt.Errorf(`picoclaw is required for this command but is not available or not configured correctly.

Checked:
  PICOCLAW_HOME:   %s
  PICOCLAW_CONFIG: %s

How to fix:
  1. Ensure picoclaw has a valid config at ~/.picoclaw/config.json, including at least one usable model/provider.
  2. Or pass overrides:
     --picoclaw-home /path/to/.picoclaw
     --picoclaw-config /path/to/config.json
  3. You can also set PICOCLAW_HOME or PICOCLAW_CONFIG in the environment.

Underlying error: %w`, resolvedHome, resolvedConfig, err)
}

func ensurePicoclawConfigAvailable(home, configPath string) error {
	_, resolvedConfig := resolvePicoclawPaths(home, configPath)
	if _, err := os.Stat(resolvedConfig); err != nil {
		if os.IsNotExist(err) {
			return picoclawUnavailableError(fmt.Errorf("picoclaw config file does not exist"), home, configPath)
		}
		return picoclawUnavailableError(fmt.Errorf("cannot access picoclaw config file: %w", err), home, configPath)
	}
	return nil
}

func shouldWrapPicoclawError(err error) bool {
	if err == nil {
		return false
	}
	if pcwrap.IsEmptyDirectResponseError(err) {
		return false
	}
	msg := strings.ToLower(err.Error())
	wrapIndicators := []string{
		"picoclaw config file does not exist",
		"cannot access picoclaw config file",
		"load picoclaw config failed",
		"create picoclaw provider failed",
		"no model specified and no default model configured",
		"agent \"",
		"not found",
	}
	for _, indicator := range wrapIndicators {
		if strings.Contains(msg, indicator) {
			return true
		}
	}
	return false
}

func resolvePicoclawPaths(home, configPath string) (string, string) {
	resolvedHome := strings.TrimSpace(home)
	if resolvedHome == "" {
		if envHome := strings.TrimSpace(os.Getenv("PICOCLAW_HOME")); envHome != "" {
			resolvedHome = envHome
		}
	}
	if resolvedHome == "" {
		if userHome, homeErr := os.UserHomeDir(); homeErr == nil {
			resolvedHome = filepath.Join(userHome, ".picoclaw")
		} else {
			resolvedHome = "~/.picoclaw"
		}
	}

	resolvedConfig := strings.TrimSpace(configPath)
	if resolvedConfig == "" {
		if envConfig := strings.TrimSpace(os.Getenv("PICOCLAW_CONFIG")); envConfig != "" {
			resolvedConfig = envConfig
		}
	}
	if resolvedConfig == "" {
		resolvedConfig = filepath.Join(resolvedHome, "config.json")
	}
	return resolvedHome, resolvedConfig
}
