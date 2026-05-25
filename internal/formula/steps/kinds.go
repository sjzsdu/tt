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
	DynamicForm bool
	OutputKey   string
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
	prompt := s.Prompt
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

func appendDynamicHumanInputProtocol(prompt string) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(prompt))
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString("## Dynamic human input\n\n")
	b.WriteString("If you need user clarification before completing this step, output ONLY a fenced `tt-human-input` JSON block using this shape:\n\n")
	b.WriteString("```tt-human-input json\n")
	b.WriteString(`{"reason":"why input is needed","form":{"title":"Short title","description":"What to provide","fields":[{"name":"field_name","label":"Field label","type":"input|textarea|radio|checkbox|select","required":true,"options":["only for radio/checkbox/select"],"placeholder":"optional"}]}}`)
	b.WriteString("\n```\n\n")
	b.WriteString("Use field names matching ^[a-z][a-z0-9_]*$. If no clarification is needed, do not include a tt-human-input block and complete the normal task output.\n")
	return b.String()
}

var dynamicHumanInputBlockPattern = regexp.MustCompile("(?s)```tt-human-input(?:\\s+json)?\\s*\\n(.*?)\\n```")

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
	Command   []string
	Cwd       string
	Env       map[string]string
	OutputKey string
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
	out, err := req.Capabilities.Scripts.RunScript(ctx, ScriptRequest{Command: s.Command, Cwd: s.Cwd, Env: s.Env})
	if err != nil {
		return &RunResult{Status: StatusFailed, Output: out, Error: &StepError{Message: "script step failed", Cause: err}}, err
	}
	return &RunResult{Status: StatusCompleted, Output: out}, nil
}

type HumanInputStep struct {
	Base
	Reason    string
	Form      any
	OutputKey string
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
}
type LoopDecoder struct{}

func (LoopDecoder) Kind() Kind { return KindLoop }
func (LoopDecoder) Decode(decl ast.StepDecl) (Step, error) {
	return LoopStep{Base: Base{metadataFromDecl(decl, KindLoop)}}, nil
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
