package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUseTTAgentStorageKeepsPicoclawHomeAndConfig(t *testing.T) {
	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}

	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir project dir: %v", err)
	}
	defer func() { _ = os.Chdir(prevWD) }()

	prevHome, hadHome := os.LookupEnv("PICOCLAW_HOME")
	prevConfig, hadConfig := os.LookupEnv("PICOCLAW_CONFIG")
	prevSessions, hadSessions := os.LookupEnv("PICOCLAW_SESSIONS_DIR")
	prevMemory, hadMemory := os.LookupEnv("PICOCLAW_MEMORY_DIR")
	prevState, hadState := os.LookupEnv("PICOCLAW_STATE_DIR")
	defer func() {
		if hadHome {
			_ = os.Setenv("PICOCLAW_HOME", prevHome)
		} else {
			_ = os.Unsetenv("PICOCLAW_HOME")
		}
		if hadConfig {
			_ = os.Setenv("PICOCLAW_CONFIG", prevConfig)
		} else {
			_ = os.Unsetenv("PICOCLAW_CONFIG")
		}
		if hadSessions {
			_ = os.Setenv("PICOCLAW_SESSIONS_DIR", prevSessions)
		} else {
			_ = os.Unsetenv("PICOCLAW_SESSIONS_DIR")
		}
		if hadMemory {
			_ = os.Setenv("PICOCLAW_MEMORY_DIR", prevMemory)
		} else {
			_ = os.Unsetenv("PICOCLAW_MEMORY_DIR")
		}
		if hadState {
			_ = os.Setenv("PICOCLAW_STATE_DIR", prevState)
		} else {
			_ = os.Unsetenv("PICOCLAW_STATE_DIR")
		}
	}()

	if err := os.Setenv("PICOCLAW_HOME", "/env/home"); err != nil {
		t.Fatalf("set PICOCLAW_HOME: %v", err)
	}
	if err := os.Setenv("PICOCLAW_CONFIG", "/env/config.json"); err != nil {
		t.Fatalf("set PICOCLAW_CONFIG: %v", err)
	}

	workspace, resolvedHome, resolvedConfig, restore, err := useTTAgentStorage("", "")
	if err != nil {
		t.Fatalf("useTTAgentStorage: %v", err)
	}
	defer restore()

	wantWorkspace := filepath.Join(projectDir, ".tt")
	if got, want := filepath.Clean(workspace), filepath.Clean(wantWorkspace); got != want {
		resolvedGot, gotErr := filepath.EvalSymlinks(got)
		resolvedWant, wantErr := filepath.EvalSymlinks(want)
		if gotErr != nil || wantErr != nil || resolvedGot != resolvedWant {
			t.Fatalf("workspace = %q, want %q", workspace, wantWorkspace)
		}
	}
	if resolvedHome != "/env/home" {
		t.Fatalf("resolvedHome = %q, want original env home", resolvedHome)
	}
	if resolvedConfig != "/env/config.json" {
		t.Fatalf("resolvedConfig = %q, want original env config", resolvedConfig)
	}
	if got := os.Getenv("PICOCLAW_HOME"); got != "/env/home" {
		t.Fatalf("PICOCLAW_HOME = %q, want unchanged original value", got)
	}
	if got := os.Getenv("PICOCLAW_CONFIG"); got != "/env/config.json" {
		t.Fatalf("PICOCLAW_CONFIG = %q, want unchanged original value", got)
	}
	wantSessions := filepath.Join(projectDir, ".tt", "sessions")
	if got, want := filepath.Clean(os.Getenv("PICOCLAW_SESSIONS_DIR")), filepath.Clean(wantSessions); got != want {
		resolvedGot, gotErr := filepath.EvalSymlinks(got)
		resolvedWant, wantErr := filepath.EvalSymlinks(want)
		if gotErr != nil || wantErr != nil || resolvedGot != resolvedWant {
			t.Fatalf("PICOCLAW_SESSIONS_DIR = %q, want %q", os.Getenv("PICOCLAW_SESSIONS_DIR"), wantSessions)
		}
	}
	if _, err := os.Stat(wantSessions); err != nil {
		t.Fatalf("sessions dir missing: %v", err)
	}
	for envName, dirName := range map[string]string{"PICOCLAW_MEMORY_DIR": "memory", "PICOCLAW_STATE_DIR": "state"} {
		wantDir := filepath.Join(projectDir, ".tt", dirName)
		if got, want := filepath.Clean(os.Getenv(envName)), filepath.Clean(wantDir); got != want {
			resolvedGot, gotErr := filepath.EvalSymlinks(got)
			resolvedWant, wantErr := filepath.EvalSymlinks(want)
			if gotErr != nil || wantErr != nil || resolvedGot != resolvedWant {
				t.Fatalf("%s = %q, want %q", envName, os.Getenv(envName), wantDir)
			}
		}
		if _, err := os.Stat(wantDir); err != nil {
			t.Fatalf("%s dir missing: %v", dirName, err)
		}
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".tt", "picoclaw")); !os.IsNotExist(err) {
		t.Fatalf("unexpected .tt/picoclaw dir state, err = %v", err)
	}
}
