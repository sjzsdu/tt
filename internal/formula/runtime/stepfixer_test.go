package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

func TestRegistryLookupByKind(t *testing.T) {
	if f, ok := defaultFixers.Lookup(steps.KindAgent); !ok || f == nil {
		t.Fatalf("defaultFixers.Lookup(KindAgent) = %v, %v; want agentFixer", f, ok)
	}
	if f, ok := defaultFixers.Lookup(steps.KindScript); !ok || f == nil {
		t.Fatalf("defaultFixers.Lookup(KindScript) = %v, %v; want scriptFixer", f, ok)
	}
	if f, ok := defaultFixers.Lookup(steps.KindLoop); ok || f != nil {
		t.Fatalf("defaultFixers.Lookup(KindLoop) = %v, %v; want nil, false", f, ok)
	}
}

func TestRegistryRegisterAndNilSafety(t *testing.T) {
	r := NewFixerRegistry()
	r.Register(nil)
	if _, ok := r.Lookup(steps.KindAgent); ok {
		t.Fatal("Register(nil) should not panic and Lookup should still return false")
	}
	r.Register(agentFixer{})
	if _, ok := r.Lookup(steps.KindAgent); !ok {
		t.Fatal("expected agentFixer to be registered")
	}
}

func TestAgentFixerAppendsValidationAdvice(t *testing.T) {
	original := steps.AgentStep{
		Base:   steps.Base{Metadata: steps.Metadata{ID: "analyze", Kind: steps.KindAgent}},
		Prompt: "do the thing",
	}
	var emitted []map[string]any
	fc := FixContext{
		NodeID:        ir.NodeID("analyze"),
		Step:          original,
		Attempt:       1,
		ValidationErr: errors.New("missing field foo"),
		Output:        steps.Value{Type: "json", Raw: json.RawMessage(`{"partial":1}`)},
		Emit: func(_ string, eventType string, payload any) {
			emitted = append(emitted, map[string]any{"type": eventType, "payload": payload})
		},
	}
	fixed, report, err := agentFixer{}.Fix(context.Background(), fc)
	if err != nil {
		t.Fatalf("Fix returned err: %v", err)
	}
	if report.Reason == "" {
		t.Fatal("report.Reason should be non-empty")
	}
	fixedStep, ok := fixed.(steps.AgentStep)
	if !ok {
		t.Fatalf("fixed step is not AgentStep: %T", fixed)
	}
	if !strings.Contains(fixedStep.Prompt, "do the thing") {
		t.Fatalf("prompt lost original text: %q", fixedStep.Prompt)
	}
	if !strings.Contains(fixedStep.Prompt, "Previous output validation failed") {
		t.Fatalf("prompt missing validation advice header: %q", fixedStep.Prompt)
	}
	if !strings.Contains(fixedStep.Prompt, "missing field foo") {
		t.Fatalf("prompt missing validation error text: %q", fixedStep.Prompt)
	}
	if len(emitted) != 1 || emitted[0]["type"] != "step.retry" {
		t.Fatalf("expected single step.retry event, got %#v", emitted)
	}
}

func TestAgentFixerRejectsNonAgentStep(t *testing.T) {
	fc := FixContext{
		NodeID:        ir.NodeID("noop"),
		Step:          steps.NoopStep{Base: steps.Base{Metadata: steps.Metadata{ID: "noop", Kind: steps.KindNoop}}},
		ValidationErr: errors.New("x"),
	}
	_, _, err := agentFixer{}.Fix(context.Background(), fc)
	if err == nil {
		t.Fatal("expected error for non-agent step")
	}
}

func TestAgentFixerRejectsMissingValidationError(t *testing.T) {
	step := steps.AgentStep{Base: steps.Base{Metadata: steps.Metadata{ID: "a", Kind: steps.KindAgent}}, Prompt: "p"}
	fc := FixContext{NodeID: ir.NodeID("a"), Step: step}
	_, _, err := agentFixer{}.Fix(context.Background(), fc)
	if err == nil {
		t.Fatal("expected error when ValidationErr is nil")
	}
}

type captureScriptRunner struct{}

func (captureScriptRunner) RunScript(_ context.Context, _ steps.ScriptRequest) (steps.Value, error) {
	return steps.Value{Type: "json", Raw: json.RawMessage(`{"ok":true}`)}, nil
}

type recordingRepairAgent struct {
	prompts []string
}

func (a *recordingRepairAgent) RunAgent(_ context.Context, req steps.AgentRequest) (steps.Value, error) {
	a.prompts = append(a.prompts, req.Prompt)
	return steps.Value{Type: "json", Raw: json.RawMessage(`{"fixed_command":["fixed"],"reason":"original command was wrong","formula_update_hint":"replace bad with fixed"}`)}, nil
}

func TestScriptFixerInvokesRepairAgent(t *testing.T) {
	agent := &recordingRepairAgent{}
	script := steps.ScriptStep{
		Base:    steps.Base{Metadata: steps.Metadata{ID: "script", Kind: steps.KindScript}},
		Command: []string{"bad"},
	}
	var emitted []string
	fc := FixContext{
		NodeID:       ir.NodeID("script"),
		Step:         script,
		RunErr:       errors.New("exit 1"),
		Capabilities: steps.Capabilities{Agents: agent, Scripts: captureScriptRunner{}},
		Context:      NewContextStore(),
		Emit:         func(_ string, eventType string, _ any) { emitted = append(emitted, eventType) },
	}
	fixed, report, err := scriptFixer{}.Fix(context.Background(), fc)
	if err != nil {
		t.Fatalf("Fix returned err: %v", err)
	}
	fixedStep, ok := fixed.(steps.ScriptStep)
	if !ok {
		t.Fatalf("fixed step is not ScriptStep: %T", fixed)
	}
	if len(fixedStep.Command) != 1 || fixedStep.Command[0] != "fixed" {
		t.Fatalf("fixed command = %v, want [fixed]", fixedStep.Command)
	}
	if report.FormulaUpdateHint != "replace bad with fixed" {
		t.Fatalf("FormulaUpdateHint = %q, want %q", report.FormulaUpdateHint, "replace bad with fixed")
	}
	if len(agent.prompts) != 1 {
		t.Fatalf("agent.prompts length = %d, want 1", len(agent.prompts))
	}
	if !strings.Contains(agent.prompts[0], "Formula step execution guard") {
		t.Fatalf("repair prompt missing execution guard: %q", agent.prompts[0])
	}
	if !strings.Contains(agent.prompts[0], "bad") {
		t.Fatalf("repair prompt missing original command: %q", agent.prompts[0])
	}
	want := []string{"step.repair.started", "step.repair.completed"}
	if len(emitted) != len(want) {
		t.Fatalf("emitted events = %v, want %v", emitted, want)
	}
	for i, e := range want {
		if emitted[i] != e {
			t.Fatalf("emitted[%d] = %q, want %q", i, emitted[i], e)
		}
	}
	if _, ok := fc.Context.Get("formula_repairs.script"); !ok {
		t.Fatal("missing formula_repairs.script in context")
	}
}

func TestScriptFixerRejectsEmptyCommand(t *testing.T) {
	step := steps.ScriptStep{Base: steps.Base{Metadata: steps.Metadata{ID: "s", Kind: steps.KindScript}}}
	fc := FixContext{Step: step, Capabilities: steps.Capabilities{Agents: &recordingRepairAgent{}}}
	_, _, err := scriptFixer{}.Fix(context.Background(), fc)
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestScriptFixerRejectsNonScriptStep(t *testing.T) {
	step := steps.NoopStep{Base: steps.Base{Metadata: steps.Metadata{ID: "n", Kind: steps.KindNoop}}}
	fc := FixContext{Step: step}
	_, _, err := scriptFixer{}.Fix(context.Background(), fc)
	if err == nil {
		t.Fatal("expected error for non-script step")
	}
}
