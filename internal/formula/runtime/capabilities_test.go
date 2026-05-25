package runtime

import (
	"context"
	"encoding/json"
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
