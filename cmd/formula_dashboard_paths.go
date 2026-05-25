package cmd

import (
	"fmt"
	"path/filepath"
	"strings"
)

func renderFormulaPrompt(cwd, prompt string) string {
	if strings.TrimSpace(cwd) == "" {
		return prompt
	}
	return fmt.Sprintf("Project root: %s\n\n%s", cwd, prompt)
}

func waitForFormulaDashboardExit(d *formulaDashboardServer) {
	if d == nil {
		return
	}
	d.waitForInterrupt()
}

func formulaAgentWorkspace(cwd string) string {
	return formulaDashboardWorkspace(cwd)
}

func formulaDashboardWorkspace(cwd string) string {
	if cwd == "" {
		return ""
	}
	return filepath.Join(cwd, ".tt")
}
