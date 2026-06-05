package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

// FixReport captures what a StepFixer learned about a failed step and how the
// caller can communicate the fix to the formula author for the next run.
type FixReport struct {
	Reason            string   `json:"reason,omitempty"`
	FormulaUpdateHint string   `json:"formula_update_hint,omitempty"`
	NextAttemptHint   string   `json:"next_attempt_hint,omitempty"`
	Advice            string   `json:"advice,omitempty"`
	OriginalCommand   []string `json:"original_command,omitempty"`
	FixedCommand      []string `json:"fixed_command,omitempty"`
}

// FixContext bundles every input a StepFixer might need to produce a fix.
// Attempt is 1 for the first fix attempt (i.e. the original step has just
// failed and the executor is about to retry it). Capabilities mirrors the
// shape passed to the executor so a fixer can spawn a repair agent. Context
// is the executor's ContextStore, exposed so a fixer can record the repair
// in shared state (e.g. the formula_repairs.<step> keys consumed by the
// dashboard and the formula author).
type FixContext struct {
	NodeID        ir.NodeID
	Step          steps.Step
	Attempt       int
	RunErr        error
	ValidationErr error
	Output        steps.Value
	Capabilities  steps.Capabilities
	Context       *ContextStore
	Emit          func(nodeID string, eventType string, payload any)
}

// StepFixer is the abstraction the executor consults when a step has just
// failed (either the step's Run returned an error, or the produced output
// failed validation). Given a FixContext, Fix returns either a modified step
// ready to re-run, or a reason why no fix is possible.
//
// The executor decides whether to call Fix at all, based on the step's
// Idempotent flag and the configured max attempts. The fixer itself is
// stateless and idempotent: calling Fix twice with the same input should
// produce equivalent output.
type StepFixer interface {
	Kind() steps.Kind
	Fix(ctx context.Context, fc FixContext) (steps.Step, FixReport, error)
}

// FixerRegistry indexes StepFixer instances by the step kind they handle.
// Lookup for an unregistered kind returns (nil, false); it never panics.
type FixerRegistry struct {
	mu     sync.RWMutex
	fixers map[steps.Kind]StepFixer
}

func NewFixerRegistry() *FixerRegistry {
	return &FixerRegistry{fixers: map[steps.Kind]StepFixer{}}
}

func (r *FixerRegistry) Register(f StepFixer) {
	if r == nil || f == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fixers[f.Kind()] = f
}

func (r *FixerRegistry) Lookup(k steps.Kind) (StepFixer, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.fixers[k]
	return f, ok
}

var defaultFixers = func() *FixerRegistry {
	r := NewFixerRegistry()
	r.Register(agentFixer{})
	r.Register(scriptFixer{})
	return r
}()

type agentFixer struct{}

func (agentFixer) Kind() steps.Kind { return steps.KindAgent }

func (agentFixer) Fix(ctx context.Context, fc FixContext) (steps.Step, FixReport, error) {
	if fc.ValidationErr == nil {
		return nil, FixReport{}, fmt.Errorf("agentFixer requires a validation error")
	}
	switch s := fc.Step.(type) {
	case steps.AgentStep:
		cloned := s
		advice := fixerAgentAdvice(s, fc.Attempt, fc.ValidationErr, fc.Output)
		cloned.Prompt = strings.TrimSpace(s.Prompt) + advice
		if fc.Emit != nil {
			fc.Emit(string(fc.NodeID), "step.retry", map[string]any{
				"reason": "output_validation_failed",
				"error":  fc.ValidationErr.Error(),
			})
		}
		return cloned, FixReport{Reason: "appended validation advice to prompt", Advice: advice, FormulaUpdateHint: fixerAgentFormulaUpdateHint(s), NextAttemptHint: fixerAgentNextAttemptHint(fc.Attempt)}, nil
	case *steps.AgentStep:
		if s == nil {
			return nil, FixReport{}, fmt.Errorf("agentFixer received nil agent step")
		}
		cloned := *s
		advice := fixerAgentAdvice(cloned, fc.Attempt, fc.ValidationErr, fc.Output)
		cloned.Prompt = strings.TrimSpace(s.Prompt) + advice
		if fc.Emit != nil {
			fc.Emit(string(fc.NodeID), "step.retry", map[string]any{
				"reason": "output_validation_failed",
				"error":  fc.ValidationErr.Error(),
			})
		}
		return &cloned, FixReport{Reason: "appended validation advice to prompt", Advice: advice, FormulaUpdateHint: fixerAgentFormulaUpdateHint(cloned), NextAttemptHint: fixerAgentNextAttemptHint(fc.Attempt)}, nil
	default:
		return nil, FixReport{}, fmt.Errorf("agentFixer does not support kind %s", fc.Step.Meta().Kind)
	}
}

type scriptFixer struct{}

func (scriptFixer) Kind() steps.Kind { return steps.KindScript }

func (scriptFixer) Fix(ctx context.Context, fc FixContext) (steps.Step, FixReport, error) {
	script, ok := fc.Step.(steps.ScriptStep)
	if !ok {
		if sp, ok := fc.Step.(*steps.ScriptStep); ok && sp != nil {
			script = *sp
		} else {
			return nil, FixReport{}, fmt.Errorf("scriptFixer requires a ScriptStep")
		}
	}
	if len(script.Command) == 0 {
		return nil, FixReport{}, fmt.Errorf("scriptFixer requires non-empty command")
	}
	if fc.Capabilities.Agents == nil {
		return nil, FixReport{}, fmt.Errorf("scriptFixer requires agent capability")
	}
	if fc.Emit != nil {
		fc.Emit(string(fc.NodeID), "step.repair.started", map[string]any{
			"reason": "script_failed",
			"error":  fixerErrorString(fc.RunErr, nil),
		})
	}
	prompt := fixerScriptRepairPrompt(script, nil, fc.RunErr)
	agentOut, agentErr := fc.Capabilities.Agents.RunAgent(ctx, steps.AgentRequest{
		NodeID: string(fc.NodeID) + ".repair",
		Agent:  "coder",
		Prompt: prompt,
	})
	if agentErr != nil {
		if fc.Emit != nil {
			fc.Emit(string(fc.NodeID), "step.repair.failed", map[string]any{
				"stage": "agent",
				"error": agentErr.Error(),
			})
		}
		return nil, FixReport{Reason: agentErr.Error()}, nil
	}
	plan, err := fixerDecodeScriptRepairPlan(agentOut)
	if err != nil {
		if fc.Emit != nil {
			fc.Emit(string(fc.NodeID), "step.repair.failed", map[string]any{
				"stage": "decode",
				"error": err.Error(),
			})
		}
		return nil, FixReport{Reason: err.Error()}, nil
	}
	if len(plan.FixedCommand) == 0 {
		if fc.Emit != nil {
			fc.Emit(string(fc.NodeID), "step.repair.failed", map[string]any{
				"stage": "plan",
				"error": "fixed_command is empty",
			})
		}
		return nil, FixReport{Reason: "fixed_command is empty"}, nil
	}
	repaired := script
	repaired.Command = plan.FixedCommand
	if fc.Context != nil {
		fixerRecordFormulaRepair(fc.Context, fc.NodeID, script.Command, plan)
	}
	if fc.Emit != nil {
		fc.Emit(string(fc.NodeID), "step.repair.completed", map[string]any{
			"reason":              plan.Reason,
			"formula_update_hint": plan.FormulaUpdateHint,
		})
	}
	return repaired, FixReport{
		Reason:            plan.Reason,
		FormulaUpdateHint: plan.FormulaUpdateHint,
		NextAttemptHint:   fixerScriptNextAttemptHint(fc.Attempt),
		OriginalCommand:   append([]string(nil), script.Command...),
		FixedCommand:      append([]string(nil), plan.FixedCommand...),
	}, nil
}

type fixerScriptRepairPlan struct {
	FixedCommand      []string `json:"fixed_command"`
	Reason            string   `json:"reason"`
	FormulaUpdateHint string   `json:"formula_update_hint"`
}

func fixerScriptRepairPrompt(script steps.ScriptStep, failed *steps.RunResult, runErr error) string {
	commandJSON, _ := json.Marshal(script.Command)
	envJSON, _ := json.Marshal(script.Env)
	return strings.TrimSpace(fmt.Sprintf(`A formula script step failed. Diagnose the script/command and return a corrected command to retry once.

Return ONLY compact JSON with this shape:
{"fixed_command":["..."],"reason":"what was wrong","formula_update_hint":"what should be changed in the formula file"}

Rules:
- Preserve the original step intent.
- Prefer the smallest safe command fix.
- Do not ask the user questions.
- Do not include Markdown fences or prose outside JSON.
- Formula step execution guard: do not merely acknowledge project rules or system constraints; perform this repair task and return the JSON now.

Original command JSON:
%s

Original cwd: %s
Original env JSON:
%s

Failure: %s
Output preview:
%s`, string(commandJSON), script.Cwd, string(envJSON), fixerErrorString(runErr, failed), fixerTruncateOutput(fixerOutputRaw(failed), 3000)))
}

func fixerDecodeScriptRepairPlan(out steps.Value) (fixerScriptRepairPlan, error) {
	var plan fixerScriptRepairPlan
	candidates, err := decodedOutputCandidates(out.Raw, &steps.OutputValidationSpec{Format: "json", Required: []string{"fixed_command", "reason", "formula_update_hint"}})
	if err != nil {
		return plan, err
	}
	for _, candidate := range candidates {
		raw, _ := json.Marshal(candidate)
		if err := json.Unmarshal(raw, &plan); err == nil && len(plan.FixedCommand) > 0 {
			return plan, nil
		}
	}
	return plan, fmt.Errorf("repair agent output did not include fixed_command")
}

func fixerRecordFormulaRepair(ctx *ContextStore, nodeID ir.NodeID, original []string, plan fixerScriptRepairPlan) {
	if ctx == nil {
		return
	}
	payload := map[string]any{
		"step_id":             string(nodeID),
		"original_command":    original,
		"fixed_command":       plan.FixedCommand,
		"reason":              plan.Reason,
		"formula_update_hint": plan.FormulaUpdateHint,
		"user_notice":         "A formula script was repaired at runtime. Please update the formula document with the fixed command.",
	}
	raw, _ := json.Marshal(payload)
	_ = ctx.Set("formula_repairs."+string(nodeID), steps.Value{Type: "json", Raw: raw})
}

func fixerErrorString(err error, res *steps.RunResult) string {
	if err != nil {
		return err.Error()
	}
	if res != nil && res.Error != nil {
		return res.Error.Error()
	}
	return "script failed"
}

func fixerOutputRaw(res *steps.RunResult) []byte {
	if res == nil {
		return nil
	}
	return res.Output.Raw
}

func fixerTruncateOutput(raw []byte, limit int) string {
	text := strings.TrimSpace(string(raw))
	var decoded string
	if err := json.Unmarshal(raw, &decoded); err == nil {
		text = strings.TrimSpace(decoded)
	}
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "\n...(truncated)"
}

func fixerAgentAdvice(step steps.AgentStep, attempt int, validationErr error, output steps.Value) string {
	b := strings.Builder{}
	b.WriteString("\n\n## Previous output validation failed\n")
	b.WriteString("Your previous answer did not match this step's required output schema. Retry the task now.\n")
	b.WriteString("Return ONLY the required final output, with no explanation, no Markdown fences, and no extra prose.\n\n")
	b.WriteString("Validation error: ")
	b.WriteString(validationErr.Error())
	b.WriteString("\n\nPrevious output preview:\n")
	b.WriteString(fixerTruncateOutput(output.Raw, 2000))
	if attempt >= 2 {
		if shape := fixerAgentValidationShape(step.Validation); shape != "" {
			b.WriteString("\n\nRequired JSON shape:\n")
			b.WriteString(shape)
		}
		b.WriteString("\n\nSelf-check before returning: the JSON must parse cleanly and include every required key. Do not omit keys; use empty strings, empty arrays, or empty objects if needed.\n")
	}
	if attempt >= 3 {
		b.WriteString("\nFinal escalation: output one compact JSON value only, with no prose before or after it. If the schema expects an object, return exactly one object. If it expects an array, return exactly one array.\n")
	}
	return b.String()
}

func fixerAgentFormulaUpdateHint(step steps.AgentStep) string {
	shape := fixerAgentValidationShape(step.Validation)
	if shape == "" {
		return "Tighten this agent step prompt so it explicitly demands machine-parseable output with no prose or Markdown fences."
	}
	return "Tighten this agent step prompt so it explicitly demands machine-parseable output matching this JSON shape: " + shape
}

func fixerAgentNextAttemptHint(attempt int) string {
	if attempt >= 3 {
		return "No further automatic escalation remains after this attempt."
	}
	return fmt.Sprintf("If this attempt still fails, escalate to attempt %d with a stricter schema reminder.", attempt+1)
}

func fixerScriptNextAttemptHint(attempt int) string {
	if attempt >= 3 {
		return "No further automatic command repair attempts remain after this attempt."
	}
	return "If this command still fails, run another repair pass and validate the revised command before reusing it in the formula."
}

func fixerAgentValidationShape(spec *steps.OutputValidationSpec) string {
	if spec == nil {
		return ""
	}
	if len(spec.Required) > 0 {
		parts := make([]string, 0, len(spec.Required))
		for _, key := range spec.Required {
			parts = append(parts, fmt.Sprintf("\"%s\": \"\"", key))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	if spec.MinItems > 0 || len(spec.ItemRequired) > 0 {
		parts := make([]string, 0, len(spec.ItemRequired))
		for _, key := range spec.ItemRequired {
			parts = append(parts, fmt.Sprintf("\"%s\": \"\"", key))
		}
		return "[{" + strings.Join(parts, ", ") + "}]"
	}
	if strings.EqualFold(strings.TrimSpace(spec.Format), "json") {
		return "{}"
	}
	return ""
}
