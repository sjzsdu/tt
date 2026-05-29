package formulacmd

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
	return strings.TrimSpace(cwd)
}

func formulaDashboardWorkspace(cwd string) string {
	return formulaAgentWorkspace(cwd)
}

func formulaCodeWorkspace(workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return ""
	}
	clean := filepath.Clean(workspace)
	if filepath.Base(clean) == ".tt" {
		parent := filepath.Dir(clean)
		if parent != "." && parent != clean {
			return parent
		}
	}
	return clean
}
