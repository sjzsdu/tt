package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type Kind string

const (
	KindNoop       Kind = "noop"
	KindAgent      Kind = "agent"
	KindScript     Kind = "script"
	KindHumanInput Kind = "human_input"
	KindLoop       Kind = "loop"
	KindCondition  Kind = "condition"
	KindGate       Kind = "gate"
	KindRetry      Kind = "retry"
	KindEmbed      Kind = "embed"
	KindExpand     Kind = "expand"
)

type ID string

type Metadata struct {
	ID        ID
	Kind      Kind
	Title     string
	DependsOn []ID
	Labels    []string
	Condition string
}

type Step interface {
	Meta() Metadata
	Validate(ValidationContext) error
}

type Executable interface {
	Step
	Run(context.Context, RunRequest) (*RunResult, error)
}

type ValidationContext interface{}

type Value struct {
	Type string
	Raw  json.RawMessage
}

type ContextView interface {
	Get(path string) (Value, bool)
}

type OutputSink interface {
	Set(path string, value Value) error
}

type RunRequest struct {
	RunID        string
	NodeID       string
	Step         Step
	Context      ContextView
	Outputs      OutputSink
	Capabilities Capabilities
	Emit         func(nodeID string, eventType string, payload any)
}

type Status string

const (
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusSkipped   Status = "skipped"
	StatusWaiting   Status = "waiting"
)

type RunResult struct {
	Status Status
	Output Value
	Await  *AwaitRequest
	Error  *StepError
}

type AwaitRequest struct {
	Type   string
	Reason string
	Form   any
}

type StepError struct {
	Message string
	Cause   error
}

func (e *StepError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

type Capabilities struct {
	Agents  AgentRunner
	Scripts ScriptRunner
	Clock   Clock
}

type AgentRunner interface {
	RunAgent(context.Context, AgentRequest) (Value, error)
}

type AgentRequest struct {
	NodeID string
	Agent  string
	Model  string
	Prompt string
}

type ScriptRunner interface {
	RunScript(context.Context, ScriptRequest) (Value, error)
}

type ScriptRequest struct {
	Command []string
	Cwd     string
	Env     map[string]string
	Timeout time.Duration
}

type Clock interface {
	Now() time.Time
}

type Base struct {
	Metadata Metadata
}

func (b Base) Meta() Metadata                   { return b.Metadata }
func (b Base) Validate(ValidationContext) error { return nil }
