package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/sjzsdu/tt/internal/formula/ast"
)

type NoopStep struct{ Base }
type NoopDecoder struct{}

func (NoopDecoder) Kind() Kind { return KindNoop }
func (NoopDecoder) Decode(decl ast.StepDecl) (Step, error) {
	return NoopStep{Base{metadataFromDecl(decl, KindNoop)}}, nil
}
func (s NoopStep) Run(context.Context, RunRequest) (*RunResult, error) {
	return &RunResult{Status: StatusCompleted}, nil
}

type AgentStep struct {
	Base
	Agent       string
	Model       string
	Prompt      string
	InputCtx    []string
	DynamicForm bool
	OutputKey   string
	Validation  *OutputValidationSpec `json:"validate,omitempty"`
}
type AgentDecoder struct{}

func (AgentDecoder) Kind() Kind { return KindAgent }
func (AgentDecoder) Decode(decl ast.StepDecl) (Step, error) {
	var s AgentStep
	_ = json.Unmarshal(decl.Raw, &s)
	s.Base = Base{metadataFromDecl(decl, KindAgent)}
	return s, nil
}
func (s AgentStep) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	if req.Capabilities.Agents == nil {
		err := &StepError{Message: "agent capability is required"}
		return &RunResult{Status: StatusFailed, Error: err}, err
	}
	prompt := renderContextTemplates(s.Prompt, req.Context)
	prompt = appendInputContext(prompt, s.InputCtx, req.Context)
	if s.DynamicForm {
		prompt = appendDynamicHumanInputProtocol(prompt)
	}
	out, err := req.Capabilities.Agents.RunAgent(ctx, AgentRequest{NodeID: req.NodeID, Agent: s.Agent, Model: s.Model, Prompt: prompt})
	if err != nil {
		return &RunResult{Status: StatusFailed, Error: &StepError{Message: "agent step failed", Cause: err}}, err
	}
	if s.DynamicForm {
		request, found, parseErr := parseDynamicHumanInputRequest(out)
		if parseErr != nil {
			err := &StepError{Message: "agent produced invalid dynamic human input request", Cause: parseErr}
			return &RunResult{Status: StatusFailed, Output: out, Error: err}, parseErr
		}
		if found {
			return &RunResult{Status: StatusWaiting, Output: out, Await: request}, nil
		}
	}
	return &RunResult{Status: StatusCompleted, Output: out}, nil
}

func appendInputContext(prompt string, keys []string, ctx ContextView) string {
	if len(keys) == 0 || ctx == nil {
		return prompt
	}
	var b strings.Builder
	b.WriteString(strings.TrimSpace(prompt))
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString("## Input context\n\n")
	b.WriteString("The following values are outputs from upstream steps. A plain step id contains the complete JSON output for that step.\n\n")
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		b.WriteString("### ")
		b.WriteString(key)
		b.WriteString("\n\n")
		if value, ok := ctx.Get(key); ok {
			b.WriteString(valueForPrompt(value))
		} else {
			b.WriteString("(not available)")
		}
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}

func valueForPrompt(out Value) string {
	var text string
	if err := json.Unmarshal(out.Raw, &text); err == nil {
		return text
	}
	var decoded any
	if err := json.Unmarshal(out.Raw, &decoded); err == nil {
		pretty, err := json.MarshalIndent(decoded, "", "  ")
		if err == nil {
			return string(pretty)
		}
	}
	return strings.TrimSpace(string(out.Raw))
}

func appendDynamicHumanInputProtocol(prompt string) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(prompt))
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString("## Dynamic human input\n\n")
	b.WriteString("This step has dynamic clarification enabled. Before completing the normal task, decide whether missing user information blocks safe progress.\n\n")
	b.WriteString("If clarification is required, output ONLY a fenced `tt-human-input` JSON block using this shape:\n\n")
	b.WriteString("```tt-human-input json\n")
	b.WriteString(`{"reason":"why input is needed","form":{"title":"Short title","description":"What to provide","fields":[{"name":"field_name","label":"Field label","type":"input|textarea|radio|checkbox|select","required":true,"options":["only for radio/checkbox/select"],"placeholder":"optional"}]}}`)
	b.WriteString("\n```\n\n")
	b.WriteString("Clarification rules:\n")
	b.WriteString("- Ask only for information that blocks this step or downstream work; do not ask for nice-to-have details.\n")
	b.WriteString("- Generate the minimum necessary form at runtime. Do not assume fixed fields.\n")
	b.WriteString("- Prefer 1-5 fields. Use field names matching ^[a-z][a-z0-9_]*$.\n")
	b.WriteString("- For radio, checkbox, and select fields, include options.\n")
	b.WriteString("- If no clarification is needed, do not include a tt-human-input block and complete the normal task output exactly as requested by the step.\n")
	return b.String()
}

var dynamicHumanInputBlockPattern = regexp.MustCompile("(?s)```tt-human-input(?:\\s+json)?\\s*\\n(.*?)\\n```")
var runtimeTemplatePattern = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_-]*(?:\.[a-zA-Z_][a-zA-Z0-9_-]*)*)\s*\}\}`)

func renderContextTemplates(input string, ctx ContextView) string {
	if input == "" || ctx == nil {
		return input
	}
	return runtimeTemplatePattern.ReplaceAllStringFunc(input, func(match string) string {
		parts := runtimeTemplatePattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		value, ok := ctx.Get(parts[1])
		if !ok {
			return match
		}
		return valueForPrompt(value)
	})
}

func parseDynamicHumanInputRequest(out Value) (*AwaitRequest, bool, error) {
	text := valueText(out)
	m := dynamicHumanInputBlockPattern.FindStringSubmatch(text)
	if m == nil {
		return nil, false, nil
	}
	var req AwaitRequest
	if err := json.Unmarshal([]byte(strings.TrimSpace(m[1])), &req); err != nil {
		return nil, true, fmt.Errorf("tt-human-input block must be valid JSON: %w", err)
	}
	if strings.TrimSpace(req.Type) == "" {
		req.Type = string(KindHumanInput)
	}
	if strings.TrimSpace(req.Reason) == "" && req.Form == nil {
		return nil, true, fmt.Errorf("tt-human-input request must include reason or form")
	}
	return &req, true, nil
}

func valueText(out Value) string {
	var text string
	if err := json.Unmarshal(out.Raw, &text); err == nil {
		return text
	}
	return strings.TrimSpace(string(out.Raw))
}

type ScriptStep struct {
	Base
	Command    []string
	Cwd        string
	Env        map[string]string
	OutputKey  string
	Validation *OutputValidationSpec `json:"validate,omitempty"`
}
type ScriptDecoder struct{}

func (ScriptDecoder) Kind() Kind { return KindScript }
func (ScriptDecoder) Decode(decl ast.StepDecl) (Step, error) {
	var s ScriptStep
	_ = json.Unmarshal(decl.Raw, &s)
	s.Base = Base{metadataFromDecl(decl, KindScript)}
	return s, nil
}
func (s ScriptStep) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	if req.Capabilities.Scripts == nil {
		err := &StepError{Message: "script capability is required"}
		return &RunResult{Status: StatusFailed, Error: err}, err
	}
	out, err := req.Capabilities.Scripts.RunScript(ctx, ScriptRequest{Command: renderContextTemplateSlice(s.Command, req.Context), Cwd: renderContextTemplates(s.Cwd, req.Context), Env: renderContextTemplateMap(s.Env, req.Context)})
	if err != nil {
		return &RunResult{Status: StatusFailed, Output: out, Error: &StepError{Message: "script step failed", Cause: err}}, err
	}
	return &RunResult{Status: StatusCompleted, Output: out}, nil
}

func renderContextTemplateSlice(values []string, ctx ContextView) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = renderContextTemplates(value, ctx)
	}
	return out
}

func renderContextTemplateMap(values map[string]string, ctx ContextView) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = renderContextTemplates(value, ctx)
	}
	return out
}

type HumanInputStep struct {
	Base
	Reason     string
	Form       any
	OutputKey  string
	Validation *OutputValidationSpec `json:"validate,omitempty"`
}
type HumanInputDecoder struct{}

func (HumanInputDecoder) Kind() Kind { return KindHumanInput }
func (HumanInputDecoder) Decode(decl ast.StepDecl) (Step, error) {
	var s HumanInputStep
	_ = json.Unmarshal(decl.Raw, &s)
	s.Base = Base{metadataFromDecl(decl, KindHumanInput)}
	return s, nil
}
func (s HumanInputStep) Run(context.Context, RunRequest) (*RunResult, error) {
	return &RunResult{Status: StatusWaiting, Await: &AwaitRequest{Type: string(KindHumanInput), Reason: s.Reason, Form: s.Form}}, nil
}

type LoopStep struct {
	Base
	Body           []Step
	Parallel       bool
	MaxConcurrency int
	Until          string
	Max            int
}
type LoopDecoder struct{}

func (LoopDecoder) Kind() Kind { return KindLoop }
func (LoopDecoder) Decode(decl ast.StepDecl) (Step, error) {
	return LoopStep{Base: Base{metadataFromDecl(decl, KindLoop)}}, nil
}

func (s LoopStep) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	max := s.Max
	if max <= 0 {
		max = 1
	}
	var last Value
	for i := 1; i <= max; i++ {
		if req.Outputs != nil {
			raw, _ := json.Marshal(i)
			_ = req.Outputs.Set("iteration", Value{Type: "json", Raw: raw})
		}
		for _, child := range s.Body {
			if child == nil {
				continue
			}
			if !stepConditionMatches(child.Meta().Condition, req.Context) {
				continue
			}
			exec, ok := child.(Executable)
			if !ok {
				continue
			}
			childNodeID := fmt.Sprintf("%s.iter%d.%s", req.NodeID, i, child.Meta().ID)
			if req.Emit != nil {
				req.Emit(childNodeID, "step.started", nil)
			}
			res, err := exec.Run(ctx, RunRequest{RunID: req.RunID, NodeID: childNodeID, Step: child, Context: req.Context, Outputs: req.Outputs, Capabilities: req.Capabilities, Emit: req.Emit})
			if res == nil {
				res = &RunResult{}
			}
			if err != nil || res.Status == StatusFailed {
				if req.Emit != nil {
					req.Emit(childNodeID, "step.failed", res)
				}
				return res, err
			}
			if res.Status == StatusWaiting {
				if req.Emit != nil {
					req.Emit(childNodeID, "step.waiting", res.Await)
				}
				return res, nil
			}
			if req.Emit != nil {
				req.Emit(childNodeID, "step.completed", res)
			}
			if len(res.Output.Raw) > 0 {
				last = res.Output
				if req.Outputs != nil {
					_ = req.Outputs.Set(string(child.Meta().ID), res.Output)
				}
			}
		}
		if stepConditionMatches(s.Until, req.Context) {
			return &RunResult{Status: StatusCompleted, Output: last}, nil
		}
	}
	return &RunResult{Status: StatusCompleted, Output: last}, nil
}

func stepConditionMatches(condition string, ctx ContextView) bool {
	condition = strings.TrimSpace(condition)
	if condition == "" {
		return true
	}
	for _, op := range []string{"==", "!="} {
		parts := strings.SplitN(condition, op, 2)
		if len(parts) != 2 {
			continue
		}
		left := strings.TrimSpace(parts[0])
		expected := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		actual := ""
		if ctx != nil {
			if value, ok := ctx.Get(left); ok {
				actual = valueForPrompt(value)
			}
		}
		if op == "==" {
			return actual == expected
		}
		return actual != expected
	}
	return false
}

type RetryStep struct {
	Base
	MaxAttempts int
	Child       Step
}
type RetryDecoder struct{}

func (RetryDecoder) Kind() Kind { return KindRetry }
func (RetryDecoder) Decode(decl ast.StepDecl) (Step, error) {
	var s RetryStep
	_ = json.Unmarshal(decl.Raw, &s)
	s.Base = Base{metadataFromDecl(decl, KindRetry)}
	return s, nil
}

func metadataFromDecl(decl ast.StepDecl, kind Kind) Metadata {
	deps := make([]ID, 0, len(decl.DependsOn))
	for _, dep := range decl.DependsOn {
		deps = append(deps, ID(dep))
	}
	return Metadata{ID: ID(decl.ID), Kind: kind, Title: decl.Title, DependsOn: deps}
}
