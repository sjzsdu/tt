package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sjzsdu/tt/internal/formula/steps"
)

func TestScriptCapabilityRunsCommand(t *testing.T) {
	cap := ScriptCapability{DenyUnsafe: true, DefaultTimeout: time.Second}
	value, err := cap.RunScript(context.Background(), steps.ScriptRequest{Command: []string{"printf", "hello"}})
	if err != nil {
		t.Fatal(err)
	}
	var out scriptCapabilityOutput
	if err := json.Unmarshal(value.Raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Stdout != "hello" {
		t.Fatalf("stdout = %q", out.Stdout)
	}
	if out.ExitCode != 0 {
		t.Fatalf("exit = %d", out.ExitCode)
	}
}

func TestBuildExternalAgentArgvAdapters(t *testing.T) {
	tests := []struct {
		name   string
		driver string
		model  string
		resume string
		want   []string
	}{
		{name: "jcode", driver: "jcode", model: "m", resume: "s", want: []string{"jcode", "run", "--json", "--model", "m", "--resume", "s"}},
		{name: "bl routes through jcode", driver: "bl", model: "m", resume: "s", want: []string{"jcode", "run", "--json", "--provider", "bl", "--model", "m", "--resume", "s"}},
		{name: "codex exec subcommand", driver: "codex", model: "m", resume: "s", want: []string{"codex", "exec", "--model", "m", "--resume", "s"}},
		{name: "opencode session", driver: "opencode", model: "m", resume: "s", want: []string{"opencode", "run", "--model", "m", "--session", "s"}},
		{name: "forge resume", driver: "forge", model: "m", resume: "s", want: []string{"forge", "run", "--model", "m", "--resume", "s"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildExternalAgentArgv(tt.driver, "", tt.model, "", tt.resume, nil)
			if strings.Join(got, "\x00") != strings.Join(tt.want, "\x00") {
				t.Fatalf("argv = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestExternalAgentCapabilityRunsFakeBinary(t *testing.T) {
	tmp := t.TempDir()
	dump := filepath.Join(tmp, "dump.txt")
	bin := filepath.Join(tmp, "fake-agent")
	script := "#!/bin/sh\n" +
		"printf 'args=%s\\n' \"$*\" > " + shellQuote(dump) + "\n" +
		"printf 'stdin=' >> " + shellQuote(dump) + "\n" +
		"cat >> " + shellQuote(dump) + "\n" +
		"printf '{\"text\":\"ok\",\"session_id\":\"sess-1\"}'\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	cap := ExternalAgentCapability{Resolver: func(string) (string, error) { return bin, nil }}
	value, err := cap.RunExternalAgent(context.Background(), steps.ExternalAgentRequest{Driver: "jcode", Model: "m", Resume: "r", Prompt: "hello", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	var out externalAgentOutput
	if err := json.Unmarshal(value.Raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Text != "ok" || out.SessionID != "sess-1" || out.ExitCode != 0 {
		t.Fatalf("out = %+v", out)
	}
	dumped, err := os.ReadFile(dump)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dumped), "args=run --json --model m --resume r hello") || !strings.Contains(string(dumped), "stdin=hello") {
		t.Fatalf("dump = %s", dumped)
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func TestScriptCapabilityDeniesUnsafeCommand(t *testing.T) {
	cap := ScriptCapability{DenyUnsafe: true}
	_, err := cap.RunScript(context.Background(), steps.ScriptRequest{Command: []string{"rm", "-rf", "/tmp/nope"}})
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("err = %v", err)
	}
}

func TestDryRunCapabilities(t *testing.T) {
	agentValue, err := DryRunAgentCapability{}.RunAgent(context.Background(), steps.AgentRequest{Agent: "planner", Model: "m", Prompt: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(agentValue.Raw), "planner") {
		t.Fatalf("agent raw = %s", agentValue.Raw)
	}
	scriptValue, err := DryRunScriptCapability{}.RunScript(context.Background(), steps.ScriptRequest{Command: []string{"go", "test"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(scriptValue.Raw), "dry_run") {
		t.Fatalf("script raw = %s", scriptValue.Raw)
	}
}
