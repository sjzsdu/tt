package ttconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFallsBackToGlobalPicoclawConfigWhenProjectConfigMissing(t *testing.T) {
	tmp := t.TempDir()
	globalConfigPath := filepath.Join(tmp, "global-config.json")
	projectDir := filepath.Join(tmp, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	globalConfig := `{
  "picoclaw": {
    "home": "/Users/test/.picoclaw",
    "config": "/Users/test/.picoclaw/config.json"
  },
  "agent": {
    "session": "tt:agent"
  }
}`
	if err := os.WriteFile(globalConfigPath, []byte(globalConfig), 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	prevGlobal, hadGlobal := os.LookupEnv(envConfigPath)
	prevProject, hadProject := os.LookupEnv(envProjectConfig)
	if err := os.Setenv(envConfigPath, globalConfigPath); err != nil {
		t.Fatalf("set %s: %v", envConfigPath, err)
	}
	if err := os.Unsetenv(envProjectConfig); err != nil {
		t.Fatalf("unset %s: %v", envProjectConfig, err)
	}
	defer func() {
		if hadGlobal {
			_ = os.Setenv(envConfigPath, prevGlobal)
		} else {
			_ = os.Unsetenv(envConfigPath)
		}
		if hadProject {
			_ = os.Setenv(envProjectConfig, prevProject)
		} else {
			_ = os.Unsetenv(envProjectConfig)
		}
	}()

	loaded, err := Load(projectDir)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	wantProjectPath := filepath.Join(projectDir, projectDirName, configFileName)
	if loaded.Sources.ProjectPath != wantProjectPath {
		t.Fatalf("project path = %q, want %q", loaded.Sources.ProjectPath, wantProjectPath)
	}
	if loaded.Project.Picoclaw.Config != "" {
		t.Fatalf("project picoclaw config = %q, want empty when project config is missing", loaded.Project.Picoclaw.Config)
	}
	if loaded.Merged.Picoclaw.Config != "/Users/test/.picoclaw/config.json" {
		t.Fatalf("merged picoclaw config = %q, want global fallback", loaded.Merged.Picoclaw.Config)
	}
	if loaded.Merged.Picoclaw.Home != "/Users/test/.picoclaw" {
		t.Fatalf("merged picoclaw home = %q, want global fallback", loaded.Merged.Picoclaw.Home)
	}
}
