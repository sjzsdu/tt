package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestNormalizeCompletionShell(t *testing.T) {
	tests := map[string]string{
		"/bin/bash":       "bash",
		"/usr/bin/zsh":    "zsh",
		"fish":            "fish",
		"pwsh":            "powershell",
		"powershell.exe":  "powershell",
		"/usr/bin/false":  "",
		"":                "",
	}

	for input, want := range tests {
		if got := normalizeCompletionShell(input); got != want {
			t.Fatalf("normalizeCompletionShell(%q) = %q, want %q", input, got, want)
		}
	}
}


func TestDetectCompletionShellUsesOverride(t *testing.T) {
	prevOverride, hadOverride := os.LookupEnv("TT_COMPLETION_SHELL")
	prevShell, hadShell := os.LookupEnv("SHELL")
	defer func() {
		if hadOverride {
			_ = os.Setenv("TT_COMPLETION_SHELL", prevOverride)
		} else {
			_ = os.Unsetenv("TT_COMPLETION_SHELL")
		}
		if hadShell {
			_ = os.Setenv("SHELL", prevShell)
		} else {
			_ = os.Unsetenv("SHELL")
		}
	}()

	if err := os.Setenv("TT_COMPLETION_SHELL", "pwsh"); err != nil {
		t.Fatalf("set TT_COMPLETION_SHELL: %v", err)
	}
	if err := os.Setenv("SHELL", "/bin/bash"); err != nil {
		t.Fatalf("set SHELL: %v", err)
	}

	if got := detectCompletionShell(); got != "powershell" {
		t.Fatalf("detectCompletionShell() = %q, want powershell", got)
	}
}


func TestRunCompletionScriptRequiresKnownShell(t *testing.T) {
	var buf bytes.Buffer
	_ = buf
	if err := runCompletionScript(rootCmd, "not-a-shell"); err == nil || !strings.Contains(err.Error(), "unsupported shell type") {
		t.Fatalf("runCompletionScript() error = %v, want unsupported shell type", err)
	}
}
