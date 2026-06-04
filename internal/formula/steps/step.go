package steps

import (
	"context"
	"encoding/json"
	"errors"
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
	KindTool       Kind = "tool"
	KindAggregate     Kind = "aggregate"
	KindWriteFiles    Kind = "write_files"
	KindExternalAgent Kind = "external_agent"
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

type stepErrorJSON struct {
	Message string          `json:"Message,omitempty"`
	Cause   json.RawMessage `json:"Cause,omitempty"`
}

func (e StepError) MarshalJSON() ([]byte, error) {
	type encodedStepError struct {
		Message string `json:"Message,omitempty"`
		Cause   string `json:"Cause,omitempty"`
	}
	cause := ""
	if e.Cause != nil {
		cause = e.Cause.Error()
	}
	return json.Marshal(encodedStepError{Message: e.Message, Cause: cause})
}

func (e *StepError) UnmarshalJSON(data []byte) error {
	if e == nil {
		return nil
	}
	var decoded stepErrorJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	e.Message = decoded.Message
	causeText := decodeStepErrorCause(decoded.Cause)
	if causeText != "" {
		e.Cause = errors.New(causeText)
	} else {
		e.Cause = nil
	}
	return nil
}

func decodeStepErrorCause(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		for _, key := range []string{"message", "Message", "error", "Error"} {
			if value, ok := obj[key].(string); ok && value != "" {
				return value
			}
		}
	}
	return string(raw)
}

// OutputValidationSpec describes lightweight runtime checks for step output.
type OutputValidationSpec struct {
	Format       string
	Required     []string
	ItemRequired []string
	MinItems     int
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
	Agents         AgentRunner
	Scripts        ScriptRunner
	ExternalAgents ExternalAgentRunner
	Clock          Clock
}

type AgentRunner interface {
	RunAgent(context.Context, AgentRequest) (Value, error)
}

type AgentRequest struct {
	NodeID    string
	Agent     string
	Model     string
	Workspace string
	Prompt    string
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

// ExternalAgentRunner runs a step by invoking an external agent CLI such as
// jcode, codex, opencode, or forge. The runner is responsible for spawning the
// binary, forwarding the prompt, and parsing the response into a Value.
type ExternalAgentRunner interface {
	RunExternalAgent(context.Context, ExternalAgentRequest) (Value, error)
}

// ExternalAgentRequest is the contract between the formula step and the
// external-agent runner. Driver selects the binary (e.g. "jcode", "codex",
// "opencode", "forge"); Provider/Model are passed through as CLI flags where
// the driver supports them. Resume continues an existing session; Mode is a
// runner-specific mode hint such as "ambient", "plan", or "normal".
type ExternalAgentRequest struct {
	NodeID     string
	Driver     string
	Provider   string
	Model      string
	Mode       string
	Resume     string
	Workspace  string
	Prompt     string
	ExtraArgs  []string
	Timeout    time.Duration
}

var (
	// DefaultExternalAgentDriver is used when a step omits `driver` and no
	// runner-level default is configured. Routed to the jcode adapter.
	DefaultExternalAgentDriver = "jcode"

	// SupportedExternalAgentDrivers enumerates drivers the bundled
	// ExternalAgentCapability can spawn. Other drivers can be registered by
	// callers supplying their own ExternalAgentRunner implementation.
	SupportedExternalAgentDrivers = map[string]bool{
		"jcode":    true,
		"codex":    true,
		"opencode": true,
		"forge":    true,
		"bl":       true, // routed via the jcode-style subprocess; opt-in.
	}
)

type Clock interface {
	Now() time.Time
}

type Base struct {
	Metadata Metadata
}

func (b Base) Meta() Metadata                   { return b.Metadata }
func (b Base) Validate(ValidationContext) error { return nil }
