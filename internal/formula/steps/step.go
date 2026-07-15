package steps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

type Kind string

const (
	KindNoop          Kind = "noop"
	KindAgent         Kind = "agent"
	KindScript        Kind = "script"
	KindHumanInput    Kind = "human_input"
	KindLoop          Kind = "loop"
	KindCondition     Kind = "condition"
	KindGate          Kind = "gate"
	KindRetry         Kind = "retry"
	KindEmbed         Kind = "embed"
	KindExpand        Kind = "expand"
	KindTool          Kind = "tool"
	KindAggregate     Kind = "aggregate"
	KindWriteFiles    Kind = "write_files"
	KindExternalAgent Kind = "external_agent"
	KindFormula       Kind = "formula"
)

type ID string

type Metadata struct {
	ID         ID
	Kind       Kind
	Title      string
	DependsOn  []ID
	Labels     []string
	Condition  string
	Idempotent bool
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
	RunID  string
	NodeID string
	Step   Step
	Inputs InputMap
	// Context is retained while existing step implementations migrate to the
	// explicit Inputs map. New composite steps should read Inputs first.
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
	// Outputs is the canonical named output-port map for every step kind.
	Outputs map[string]Value
	// Output mirrors the primary output for artifact/UI compatibility. New
	// orchestration code should consume Outputs.
	Output Value
	Await  *AwaitRequest
	Error  *StepError
}

const (
	OutputResult = "result"
	OutputReport = "report"
)

// NormalizeOutputs keeps the transitional primary Output field and canonical
// Outputs map consistent. A legacy single output becomes the "result" port.
func (r *RunResult) NormalizeOutputs() {
	if r == nil {
		return
	}
	if len(r.Outputs) == 0 && len(r.Output.Raw) > 0 {
		r.Outputs = map[string]Value{OutputResult: r.Output}
	}
	if len(r.Output.Raw) == 0 {
		if value, ok := r.PrimaryOutput(); ok {
			r.Output = value
		}
	}
}

// PrimaryOutput selects the human-facing report first, then the conventional
// result/default port, then the only or lexicographically first named port.
func (r *RunResult) PrimaryOutput() (Value, bool) {
	if r == nil {
		return Value{}, false
	}
	for _, name := range []string{OutputReport, OutputResult, "default"} {
		if value, ok := r.Outputs[name]; ok && len(value.Raw) > 0 {
			return value, true
		}
	}
	if len(r.Output.Raw) > 0 {
		return r.Output, true
	}
	if len(r.Outputs) == 0 {
		return Value{}, false
	}
	names := make([]string, 0, len(r.Outputs))
	for name := range r.Outputs {
		names = append(names, name)
	}
	sort.Strings(names)
	value := r.Outputs[names[0]]
	return value, len(value.Raw) > 0
}

func (r *RunResult) SetPrimaryOutput(value Value) {
	if r == nil {
		return
	}
	name := OutputResult
	for _, candidate := range []string{OutputReport, OutputResult, "default"} {
		if _, ok := r.Outputs[candidate]; ok {
			name = candidate
			break
		}
	}
	if r.Outputs == nil {
		r.Outputs = map[string]Value{}
	}
	r.Outputs[name] = value
	r.Output = value
}

func ResultWithOutput(status Status, value Value) *RunResult {
	result := &RunResult{Status: status}
	result.SetPrimaryOutput(value)
	return result
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
	Workflows      WorkflowRunner
	Clock          Clock
}

// WorkflowRunner executes a Formula as a composite step. It deliberately
// lives in the steps package so FormulaCallStep does not depend on the runtime
// or IR packages.
type WorkflowRunner interface {
	RunWorkflow(context.Context, WorkflowRequest) (*WorkflowResult, error)
}

type WorkflowRequest struct {
	RunID   string
	NodeID  string
	Formula string
	Inputs  map[string]Value
}

type WorkflowResult struct {
	Status  Status
	Outputs map[string]Value
	Await   *AwaitRequest
	Error   *StepError
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
	NodeID    string
	Driver    string
	Provider  string
	Model     string
	Mode      string
	Resume    string
	Workspace string
	Prompt    string
	ExtraArgs []string
	Timeout   time.Duration
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
