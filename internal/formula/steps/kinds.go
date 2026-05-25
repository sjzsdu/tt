package steps

import (
	"context"
	"encoding/json"

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
	Agent     string
	Model     string
	Prompt    string
	OutputKey string
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
	out, err := req.Capabilities.Agents.RunAgent(ctx, AgentRequest{NodeID: req.NodeID, Agent: s.Agent, Model: s.Model, Prompt: s.Prompt})
	if err != nil {
		return &RunResult{Status: StatusFailed, Error: &StepError{Message: "agent step failed", Cause: err}}, err
	}
	return &RunResult{Status: StatusCompleted, Output: out}, nil
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
