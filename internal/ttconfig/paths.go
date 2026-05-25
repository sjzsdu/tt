package ttconfig

import (
	"path/filepath"
	"strings"
)

// ProjectRoot returns the project root associated with a loaded tt config.
// If no project config was discovered, it falls back to the current working directory.
func ProjectRoot(loaded Loaded, wd string) string {
	if loaded.Sources.ProjectPath != "" {
		return filepath.Dir(filepath.Dir(loaded.Sources.ProjectPath))
	}
	if strings.TrimSpace(wd) != "" {
		return wd
	}
	return "."
}

// ResolvePath resolves p against the loaded project root unless p is already absolute.
func ResolvePath(loaded Loaded, p, wd string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(ProjectRoot(loaded, wd), p))
}

// FormulaDir returns the configured formula directory or the project default.
func FormulaDir(loaded Loaded, wd string) string {
	if v := strings.TrimSpace(loaded.Merged.Paths.FormulaDir); v != "" {
		return ResolvePath(loaded, v, wd)
	}
	return filepath.Join(ProjectRoot(loaded, wd), ".tt", "formulas")
}

// AgentDir returns the configured agent directory or the project default.
func AgentDir(loaded Loaded, wd string) string {
	if v := strings.TrimSpace(loaded.Merged.Paths.AgentDir); v != "" {
		return ResolvePath(loaded, v, wd)
	}
	return filepath.Join(ProjectRoot(loaded, wd), ".tt", "agents")
}

// FormulaRunDir returns the configured formula run directory or the project default.
func FormulaRunDir(loaded Loaded, wd string) string {
	if v := strings.TrimSpace(loaded.Merged.Paths.FormulaRunDir); v != "" {
		return ResolvePath(loaded, v, wd)
	}
	return filepath.Join(ProjectRoot(loaded, wd), ".tt", "runs", "formula")
}
