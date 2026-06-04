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
		extra  []string
		want   []string
	}{
		{name: "jcode", driver: "jcode", model: "m", resume: "s", want: []string{"jcode", "run", "--json", "--model", "m", "--resume", "s"}},
		{name: "bl routes through jcode", driver: "bl", model: "m", resume: "s", want: []string{"jcode", "run", "--json", "--provider", "bl", "--model", "m", "--resume", "s"}},
		{name: "codex exec subcommand", driver: "codex", model: "m", want: []string{"codex", "exec", "--model", "m"}},
		{name: "codex resume subcommand", driver: "codex", model: "m", resume: "s", want: []string{"codex", "exec", "resume", "--model", "m", "s"}},
		{name: "codex resume extra before session", driver: "codex", model: "m", resume: "s", extra: []string{"--json"}, want: []string{"codex", "exec", "resume", "--model", "m", "--json", "s"}},
		{name: "opencode session", driver: "opencode", model: "m", resume: "s", want: []string{"opencode", "run", "--model", "m", "--session", "s", "--format", "json"}},
		{name: "opencode keeps explicit format", driver: "opencode", extra: []string{"--format", "default"}, want: []string{"opencode", "run", "--format", "default"}},
		{name: "forge conversation", driver: "forge", model: "m", resume: "s", want: []string{"forge", "--conversation-id", "s"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildExternalAgentArgv(tt.driver, "", tt.model, "", tt.resume, tt.extra)
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
	if !strings.Contains(string(dumped), "args=run --json --model m --resume r hello") || !strings.Contains(string(dumped), "stdin=") || strings.Contains(string(dumped), "stdin=hello") {
		t.Fatalf("dump = %s", dumped)
	}
}

func TestExternalAgentCapabilityCapturesCodexLastMessage(t *testing.T) {
	tmp := t.TempDir()
	dump := filepath.Join(tmp, "dump.txt")
	bin := filepath.Join(tmp, "fake-codex")
	script := "#!/bin/sh\n" +
		"printf 'args=%s\\n' \"$*\" > " + shellQuote(dump) + "\n" +
		"last=''\n" +
		"prev=''\n" +
		"for arg in \"$@\"; do if [ \"$prev\" = '--output-last-message' ]; then last=\"$arg\"; fi; prev=\"$arg\"; done\n" +
		"if [ -n \"$last\" ]; then printf 'OK from file' > \"$last\"; fi\n" +
		"printf 'OpenAI Codex v0.130.0\\n--------\\nsession id: sess-codex\\n--------\\ncodex\\nnoisy stdout\\n'\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	cap := ExternalAgentCapability{Resolver: func(string) (string, error) { return bin, nil }}
	value, err := cap.RunExternalAgent(context.Background(), steps.ExternalAgentRequest{Driver: "codex", Model: "m", Prompt: "hello", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	var out externalAgentOutput
	if err := json.Unmarshal(value.Raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Text != "OK from file" || out.SessionID != "sess-codex" {
		t.Fatalf("out = %+v", out)
	}
	dumped, err := os.ReadFile(dump)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dumped), "--output-last-message") {
		t.Fatalf("missing output-last-message arg: %s", dumped)
	}
}

func TestAppendExternalAgentPrompt(t *testing.T) {
	if got := appendAgentPrompt([]string{"forge", "--conversation-id", "s"}, "forge", "hello"); strings.Join(got, " ") != "forge --conversation-id s" {
		t.Fatalf("forge prompt should be stdin, argv = %#v", got)
	}
	if !externalAgentPromptInArgv("codex", "hello") || externalAgentPromptInArgv("forge", "hello") || externalAgentPromptInArgv("codex", "") {
		t.Fatal("prompt argv detection mismatch")
	}
}

func TestExtractForgeTextStripsSpinnerOutput(t *testing.T) {
	raw := "\r\x1b[2K⠋ Migrating credentials 00s · Ctrl+C to interrupt\r\x1b[2K● [14:41:42] Initialize abc\n\r\x1b[2K⠹ Synthesizing 03s · Ctrl+C to interrupt\r\x1b[2KOK\n\r\x1b[2K● [14:41:47] Finished abc\n"
	if got := extractExternalAgentText("forge", raw); got != "OK" {
		t.Fatalf("forge text = %q", got)
	}
	spinnerOnly := "\r\x1b[2K⠋ Reasoning 00s · Ctrl+C to interrupt\r\x1b[2K⠹ Reasoning 01s · Ctrl+C to interrupt\r\x1b[2K"
	if got := cleanExternalAgentStderr("forge", spinnerOnly); got != "" {
		t.Fatalf("forge stderr = %q", got)
	}
}

func TestExtractOpenCodeTextFromJSONEvents(t *testing.T) {
	raw := strings.Join([]string{
		`{"type":"session","id":"sess-1"}`,
		`{"type":"message.part.updated","part":{"type":"text","text":"hel"}}`,
		`{"type":"message.part.updated","part":{"type":"text","delta":"lo"}}`,
		`{"type":"message.completed","message":{"parts":[{"type":"text","text":" ignored"}]}}`,
	}, "\n")
	if got := extractExternalAgentText("opencode", raw); got != "hello ignored" {
		t.Fatalf("opencode text = %q", got)
	}
	if got := extractExternalAgentSessionID("opencode", raw); got != "sess-1" {
		t.Fatalf("opencode session id = %q", got)
	}
	fallback := "plain formatted output"
	if got := extractExternalAgentText("opencode", fallback); got != fallback {
		t.Fatalf("opencode fallback = %q", got)
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
