// Package formula provides parsing, validation, and compilation of formula
// templates — structured workflow definitions that compile into ordered task
// graphs with variable substitution, dependency tracking, and control flow.
//
// A formula is a TOML or JSON file that defines:
//   - Variables with defaults and validation
//   - Steps (tasks) with titles, descriptions, priorities, and dependencies
//   - Control flow operators (loops, branches, gates)
//   - Composition rules (expansion, advice/aspect transforms)
//
// Example formula (TOML):
//
//	formula = "add-feature"
//	description = "Standard feature workflow"
//	version = 1
//	type = "workflow"
//
//	[vars]
//	component = { description = "Component name", required = true }
//
//	[[steps]]
//	id = "design"
//	title = "Design {{component}}"
//	type = "task"
//
//	[[steps]]
//	id = "implement"
//	title = "Implement {{component}}"
//	type = "task"
//	depends_on = ["design"]
package formula

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Type categorizes formulas by their purpose.
type Type string

const (
	// TypeWorkflow is a standard workflow template (sequence of steps).
	TypeWorkflow Type = "workflow"

	// TypeExpansion is a macro that expands into multiple steps.
	TypeExpansion Type = "expansion"

	// TypeAspect is a cross-cutting concern that can be applied to other formulas.
	TypeAspect Type = "aspect"
)

// IsValid checks if the formula type is recognized.
func (t Type) IsValid() bool {
	switch t {
	case TypeWorkflow, TypeExpansion, TypeAspect:
		return true
	}
	return false
}

// Formula is the root structure for formula definition files.
type Formula struct {
	// Formula is the unique identifier/name for this formula.
	Formula string `json:"formula" toml:"formula"`

	// Description explains what this formula does.
	Description string `json:"description,omitempty" toml:"description,omitempty"`

	// Title is a human-friendly display name.
	Title string `json:"title,omitempty" toml:"title,omitempty"`

	// Category groups formulas in catalogs and list output.
	Category string `json:"category,omitempty" toml:"category,omitempty"`

	// Tags provide lightweight discovery keywords.
	Tags []string `json:"tags,omitempty" toml:"tags,omitempty"`

	// Version is the formula revision.
	Version int `json:"version" toml:"version"`

	// Contract opts the formula into a specific runtime contract.
	// "graph.v2" enables graph-first workflow compilation.
	Contract string `json:"contract,omitempty" toml:"contract,omitempty"`

	// Type categorizes the formula: workflow, expansion, or aspect.
	Type Type `json:"type" toml:"type"`

	// Extends is a list of parent formulas to inherit from.
	Extends []string `json:"extends,omitempty" toml:"extends,omitempty"`

	// Vars defines template variables with defaults and validation.
	Vars map[string]*VarDef `json:"vars,omitempty" toml:"vars,omitempty"`

	// Steps defines the work items to create.
	Steps []*Step `json:"steps,omitempty" toml:"steps,omitempty"`

	// Template defines expansion template steps (for TypeExpansion formulas).
	Template []*Step `json:"template,omitempty" toml:"template,omitempty"`

	// Compose defines composition/bonding rules.
	Compose *ComposeRules `json:"compose,omitempty" toml:"compose,omitempty"`

	// Advice defines step transformations (before/after/around).
	Advice []*AdviceRule `json:"advice,omitempty" toml:"advice,omitempty"`

	// Pointcuts defines target patterns for aspect formulas.
	Pointcuts []*Pointcut `json:"pointcuts,omitempty" toml:"pointcuts,omitempty"`

	// Phase indicates the recommended instantiation phase: "liquid" or "vapor".
	Phase string `json:"phase,omitempty" toml:"phase,omitempty"`

	// Pour controls whether steps are materialized as individual child items.
	Pour bool `json:"pour,omitempty" toml:"pour,omitempty"`

	// Source tracks where this formula was loaded from (set by parser).
	Source string `json:"-" toml:"-"`
}

// VarDef defines a template variable with optional validation.
type VarDef struct {
	// Description explains what this variable is for.
	Description string `json:"description,omitempty" toml:"description,omitempty"`

	// Default is the value to use if not provided.
	Default *string `json:"default,omitempty" toml:"default,omitempty"`

	// Required indicates the variable must be provided (no default).
	Required bool `json:"required,omitempty" toml:"required,omitempty"`

	// Enum lists the allowed values (if non-empty).
	Enum []string `json:"enum,omitempty" toml:"enum,omitempty"`

	// Pattern is a regex pattern the value must match.
	Pattern string `json:"pattern,omitempty" toml:"pattern,omitempty"`

	// Type is the expected value type: string (default), int, bool.
	Type string `json:"type,omitempty" toml:"type,omitempty"`
}

// UnmarshalTOML implements toml.Unmarshaler for VarDef.
func (v *VarDef) UnmarshalTOML(data interface{}) error {
	switch val := data.(type) {
	case string:
		v.Default = &val
		return nil
	case map[string]interface{}:
		if desc, ok := val["description"].(string); ok {
			v.Description = desc
		}
		if def, ok := val["default"].(string); ok {
			v.Default = &def
		}
		if req, ok := val["required"].(bool); ok {
			v.Required = req
		}
		if enum, ok := val["enum"].([]interface{}); ok {
			for _, e := range enum {
				if s, ok := e.(string); ok {
					v.Enum = append(v.Enum, s)
				}
			}
		}
		if pattern, ok := val["pattern"].(string); ok {
			v.Pattern = pattern
		}
		if typ, ok := val["type"].(string); ok {
			v.Type = typ
		}
		return nil
	default:
		return fmt.Errorf("type mismatch for formula.VarDef: expected string or table but found %T", data)
	}
}

// Step defines a workflow node in a formula document.
type Step struct {
	// ID is the unique identifier within this formula.
	ID string `json:"id" toml:"id"`

	// Title is the issue title (supports {{variable}} substitution).
	Title string `json:"title" toml:"title"`

	// Description is the issue description (supports substitution).
	Description string `json:"description,omitempty" toml:"description,omitempty"`

	// DescriptionFile is a path to a file whose contents replace Description.
	DescriptionFile string `json:"description_file,omitempty" toml:"description_file,omitempty"`

	// Notes are additional notes (supports substitution).
	Notes string `json:"notes,omitempty" toml:"notes,omitempty"`

	// Type is the item type: task, bug, feature, epic, chore.
	Type string `json:"type,omitempty" toml:"type,omitempty"`

	// Priority is the item priority (0-4, 0=highest).
	Priority *int `json:"priority,omitempty" toml:"priority,omitempty"`

	// Labels are applied to the created item.
	// TOML key is "tags"; Go/JSON name is "labels".
	Labels []string `json:"labels,omitempty" toml:"tags,omitempty"`

	// Metadata is copied to the output item metadata.
	Metadata map[string]string `json:"metadata,omitempty" toml:"metadata,omitempty"`

	// DependsOn lists step IDs this step blocks on.
	DependsOn []string `json:"depends_on,omitempty" toml:"depends_on,omitempty"`

	// Needs is a simpler alias for DependsOn.
	Needs []string `json:"needs,omitempty" toml:"needs,omitempty"`

	// WaitsFor specifies a fanout gate type.
	WaitsFor string `json:"waits_for,omitempty" toml:"waits_for,omitempty"`

	// Assignee is the default assignee (supports substitution).
	Assignee string `json:"assignee,omitempty" toml:"assignee,omitempty"`

	// Expand references an expansion formula to inline here.
	Expand string `json:"expand,omitempty" toml:"expand,omitempty"`

	// ExpandVars are variable overrides for the expansion.
	ExpandVars map[string]string `json:"expand_vars,omitempty" toml:"expand_vars,omitempty"`

	// Embed references a workflow formula to inline here as a reusable sub-flow.
	Embed string `json:"embed,omitempty" toml:"embed,omitempty"`

	// EmbedVars are variable overrides for the embedded workflow.
	EmbedVars map[string]string `json:"embed_vars,omitempty" toml:"embed_vars,omitempty"`

	// Condition makes this step optional based on a variable.
	Condition string `json:"condition,omitempty" toml:"condition,omitempty"`

	// Children are nested steps (for creating epic hierarchies).
	Children []*Step `json:"children,omitempty" toml:"children,omitempty"`

	// Gate defines an async wait condition.
	Gate *Gate `json:"gate,omitempty" toml:"gate,omitempty"`

	// Loop defines iteration for this step.
	Loop *LoopSpec `json:"loop,omitempty" toml:"loop,omitempty"`

	// OnComplete defines actions triggered when this step completes.
	OnComplete *OnCompleteSpec `json:"on_complete,omitempty" toml:"on_complete,omitempty"`

	// Retry wraps a step in an attempt/eval retry loop.
	Retry *RetrySpec `json:"retry,omitempty" toml:"retry,omitempty"`

	// Timeout is the maximum duration for this step.
	Timeout string `json:"timeout,omitempty" toml:"timeout,omitempty"`

	// Agent specifies the agent configuration for executing this step.
	Agent *AgentConfig `json:"agent,omitempty" toml:"agent,omitempty"`

	// Script specifies a deterministic local command for execution="script" steps.
	Script *ScriptSpec `json:"script,omitempty" toml:"script,omitempty"`

	// Aggregate projects and collects data from prior step outputs.
	Aggregate *AggregateSpec `json:"aggregate,omitempty" toml:"aggregate,omitempty"`

	// Form describes fields required by execution="human_input" steps.
	Form *FormSpec `json:"form,omitempty" toml:"form,omitempty"`

	// DynamicForm allows an agent step to emit a dynamic human-input request.
	DynamicForm bool `json:"dynamic_form,omitempty" toml:"dynamic_form,omitempty"`

	// Validate checks a step output before it is marked completed.
	Validate *ValidateSpec `json:"validate,omitempty" toml:"validate,omitempty"`

	// OutputKey stores the step's output for downstream steps to reference.
	OutputKey string `json:"output_key,omitempty" toml:"output_key,omitempty"`

	// InputCtx lists output keys from upstream steps to inject into this step's prompt.
	InputCtx []string `json:"input_context,omitempty" toml:"input_context,omitempty"`

	// Execution controls how this step runs: sync, async, or fire-and-forget.
	Execution string `json:"execution,omitempty" toml:"execution,omitempty"`

	// Source tracing fields.
	SourceFormula  string `json:"-" toml:"-"`
	SourceLocation string `json:"-" toml:"-"`
}

// UnmarshalJSON accepts the canonical public "check" spelling.
func (s *Step) UnmarshalJSON(data []byte) error {
	type stepAlias Step

	var decoded stepAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*s = Step(decoded)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	rawCheck, hasCheck := raw["check"]
	rawRalph, hasRalph := raw["ralph"]
	return s.normalizeCheckAlias(hasCheck, rawCheck, hasRalph, rawRalph)
}

// UnmarshalTOML accepts the canonical public "check" spelling.
func (s *Step) UnmarshalTOML(data interface{}) error {
	raw, ok := data.(map[string]interface{})
	if !ok {
		return fmt.Errorf("type mismatch for formula.Step: expected table but found %T", data)
	}

	encoded, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("encode formula.Step: %w", err)
	}

	var decoded stepTOMLAlias
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return fmt.Errorf("decode formula.Step: %w", err)
	}
	step, err := decoded.toStep()
	if err != nil {
		return err
	}
	*s = step

	rawCheck, hasCheck := raw["check"]
	rawRalph, hasRalph := raw["ralph"]
	return s.normalizeCheckAlias(hasCheck, rawCheck, hasRalph, rawRalph)
}

func (s *Step) normalizeCheckAlias(hasCheck bool, rawCheck interface{}, hasRalph bool, rawRalph interface{}) error {
	if hasCheck && hasRalph {
		return fmt.Errorf("step.check: cannot be specified more than once")
	}

	switch {
	case hasCheck:
		spec, err := decodePublicCheckSpec(rawCheck)
		if err != nil {
			return err
		}
		// Map to Retry for simplified tt usage
		if spec != nil {
			s.Retry = &RetrySpec{MaxAttempts: spec.MaxAttempts}
		}
	case hasRalph:
		if err := validatePublicCheckSpecShape(rawRalph); err != nil {
			return err
		}
	}

	return nil
}

type stepTOMLAlias struct {
	ID              string            `json:"id"`
	Title           string            `json:"title"`
	Description     string            `json:"description,omitempty"`
	DescriptionFile string            `json:"description_file,omitempty"`
	Notes           string            `json:"notes,omitempty"`
	Type            string            `json:"type,omitempty"`
	Priority        *int              `json:"priority,omitempty"`
	Labels          []string          `json:"tags,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	DependsOn       []string          `json:"depends_on,omitempty"`
	Needs           []string          `json:"needs,omitempty"`
	WaitsFor        string            `json:"waits_for,omitempty"`
	Assignee        string            `json:"assignee,omitempty"`
	Expand          string            `json:"expand,omitempty"`
	ExpandVars      map[string]string `json:"expand_vars,omitempty"`
	Embed           string            `json:"embed,omitempty"`
	EmbedVars       map[string]string `json:"embed_vars,omitempty"`
	Condition       string            `json:"condition,omitempty"`
	Children        []*stepTOMLAlias  `json:"children,omitempty"`
	Gate            *Gate             `json:"gate,omitempty"`
	Loop            *loopTOMLAlias    `json:"loop,omitempty"`
	OnComplete      *OnCompleteSpec   `json:"on_complete,omitempty"`
	Check           json.RawMessage   `json:"check,omitempty"`
	Ralph           json.RawMessage   `json:"ralph,omitempty"`
	Retry           *RetrySpec        `json:"retry,omitempty"`
	Timeout         string            `json:"timeout,omitempty"`
	Agent           *AgentConfig      `json:"agent,omitempty"`
	Script          *ScriptSpec       `json:"script,omitempty"`
	Aggregate       *AggregateSpec    `json:"aggregate,omitempty"`
	Form            json.RawMessage   `json:"form,omitempty"`
	DynamicForm     bool              `json:"dynamic_form,omitempty"`
	Validate        *ValidateSpec     `json:"validate,omitempty"`
	OutputKey       string            `json:"output_key,omitempty"`
	InputCtx        []string          `json:"input_context,omitempty"`
	Execution       string            `json:"execution,omitempty"`
}

type loopTOMLAlias struct {
	Count          int              `json:"count,omitempty"`
	Until          string           `json:"until,omitempty"`
	Max            int              `json:"max,omitempty"`
	Range          string           `json:"range,omitempty"`
	ForEach        string           `json:"for_each,omitempty"`
	Var            string           `json:"var,omitempty"`
	Parallel       bool             `json:"parallel,omitempty"`
	MaxConcurrency int              `json:"max_concurrency,omitempty"`
	Body           []*stepTOMLAlias `json:"body"`
}

func (a stepTOMLAlias) toStep() (Step, error) {
	hasCheck := len(a.Check) > 0
	hasRalph := len(a.Ralph) > 0
	if hasCheck && hasRalph {
		return Step{}, fmt.Errorf("step.check: cannot be specified more than once")
	}

	children := make([]*Step, 0, len(a.Children))
	for _, child := range a.Children {
		if child == nil {
			continue
		}
		step, err := child.toStep()
		if err != nil {
			return Step{}, err
		}
		children = append(children, &step)
	}

	var retry *RetrySpec
	switch {
	case hasCheck:
		spec, err := decodePublicCheckSpec(a.Check)
		if err != nil {
			return Step{}, err
		}
		if spec != nil {
			retry = &RetrySpec{MaxAttempts: spec.MaxAttempts}
		}
	case hasRalph:
		spec, err := decodePublicCheckSpec(a.Ralph)
		if err != nil {
			return Step{}, err
		}
		if spec != nil {
			retry = &RetrySpec{MaxAttempts: spec.MaxAttempts}
		}
	}
	if a.Retry != nil {
		retry = a.Retry
	}

	loop, err := a.Loop.toLoopSpec()
	if err != nil {
		return Step{}, err
	}

	form, dynamicForm, err := decodeStepForm(a.Form)
	if err != nil {
		return Step{}, err
	}
	if a.DynamicForm {
		dynamicForm = true
	}

	return Step{
		ID:              a.ID,
		Title:           a.Title,
		Description:     a.Description,
		DescriptionFile: a.DescriptionFile,
		Notes:           a.Notes,
		Type:            a.Type,
		Priority:        a.Priority,
		Labels:          a.Labels,
		Metadata:        a.Metadata,
		DependsOn:       a.DependsOn,
		Needs:           a.Needs,
		WaitsFor:        a.WaitsFor,
		Assignee:        a.Assignee,
		Expand:          a.Expand,
		ExpandVars:      a.ExpandVars,
		Embed:           a.Embed,
		EmbedVars:       a.EmbedVars,
		Condition:       a.Condition,
		Children:        children,
		Gate:            a.Gate,
		Loop:            loop,
		OnComplete:      a.OnComplete,
		Retry:           retry,
		Timeout:         a.Timeout,
		Agent:           a.Agent,
		Script:          a.Script,
		Aggregate:       a.Aggregate,
		Form:            form,
		DynamicForm:     dynamicForm,
		Validate:        a.Validate,
		OutputKey:       a.OutputKey,
		InputCtx:        a.InputCtx,
		Execution:       a.Execution,
	}, nil
}

func decodeStepForm(raw json.RawMessage) (*FormSpec, bool, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false, nil
	}
	var dynamic bool
	if err := json.Unmarshal(raw, &dynamic); err == nil {
		return nil, dynamic, nil
	}
	var form FormSpec
	if err := json.Unmarshal(raw, &form); err != nil {
		return nil, false, fmt.Errorf("step.form: expected true or form object: %w", err)
	}
	return &form, false, nil
}

func (a *loopTOMLAlias) toLoopSpec() (*LoopSpec, error) {
	if a == nil {
		return nil, nil
	}

	body := make([]*Step, 0, len(a.Body))
	for _, child := range a.Body {
		if child == nil {
			continue
		}
		step, err := child.toStep()
		if err != nil {
			return nil, err
		}
		body = append(body, &step)
	}

	return &LoopSpec{
		Count:          a.Count,
		Until:          a.Until,
		Max:            a.Max,
		Range:          a.Range,
		ForEach:        a.ForEach,
		Var:            a.Var,
		Parallel:       a.Parallel,
		MaxConcurrency: a.MaxConcurrency,
		Body:           body,
	}, nil
}

// RalphSpec defines an inline run/check retry loop (mapped to Retry).
type RalphSpec struct {
	MaxAttempts int             `json:"max_attempts,omitempty" toml:"max_attempts,omitempty"`
	Check       *RalphCheckSpec `json:"check,omitempty" toml:"check,omitempty"`
}

// RalphCheckSpec defines the validation step.
type RalphCheckSpec struct {
	Mode    string `json:"mode,omitempty" toml:"mode,omitempty"`
	Path    string `json:"path,omitempty" toml:"path,omitempty"`
	Timeout string `json:"timeout,omitempty" toml:"timeout,omitempty"`
}

func decodePublicCheckSpec(raw interface{}) (*RalphSpec, error) {
	if err := validatePublicCheckSpecShape(raw); err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("step.check: encode spec: %w", err)
	}
	if string(encoded) == "null" {
		return nil, nil
	}

	var spec RalphSpec
	if err := json.Unmarshal(encoded, &spec); err != nil {
		return nil, fmt.Errorf("step.check: decode spec: %w", err)
	}
	return &spec, nil
}

func validatePublicCheckSpecShape(raw interface{}) error {
	if raw == nil {
		return nil
	}

	encoded, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("step.check: encode spec: %w", err)
	}
	if string(encoded) == "null" {
		return nil
	}

	var spec map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &spec); err != nil {
		return fmt.Errorf("step.check: expected an object")
	}

	for key, value := range spec {
		switch key {
		case "max_attempts":
			continue
		case "check":
			if err := validatePublicCheckBodyShape(value); err != nil {
				return err
			}
		case "exec", "inference":
			return fmt.Errorf("step.check: unsupported key %q", key)
		default:
			continue
		}
	}

	return nil
}

func validatePublicCheckBodyShape(raw json.RawMessage) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		return fmt.Errorf("step.check.check: expected an object")
	}

	for key := range body {
		switch key {
		case "exec", "inference":
			return fmt.Errorf("step.check.check: unsupported key %q", key)
		default:
			continue
		}
	}

	return nil
}

// Gate defines an async wait condition for formula steps.
type Gate struct {
	Type    string `json:"type" toml:"type"`
	ID      string `json:"id,omitempty" toml:"id,omitempty"`
	Timeout string `json:"timeout,omitempty" toml:"timeout,omitempty"`
}

// AgentConfig specifies which agent executes a step and how.
type AgentConfig struct {
	Name    string `json:"name" toml:"name"`
	Model   string `json:"model,omitempty" toml:"model,omitempty"`
	Session string `json:"session,omitempty" toml:"session,omitempty"`
	Timeout string `json:"timeout,omitempty" toml:"timeout,omitempty"`
	Retries int    `json:"retries,omitempty" toml:"retries,omitempty"`
}

// ScriptSpec describes a deterministic local command step. Prefer Command argv
// form for safety; Shell is an explicit opt-in for shell evaluation.
type ScriptSpec struct {
	Command         []string          `json:"command,omitempty" toml:"command,omitempty"`
	Shell           string            `json:"shell,omitempty" toml:"shell,omitempty"`
	Cwd             string            `json:"cwd,omitempty" toml:"cwd,omitempty"`
	Env             map[string]string `json:"env,omitempty" toml:"env,omitempty"`
	Format          string            `json:"format,omitempty" toml:"format,omitempty"`
	Timeout         string            `json:"timeout,omitempty" toml:"timeout,omitempty"`
	ContinueOnError bool              `json:"continue_on_error,omitempty" toml:"continue_on_error,omitempty"`
}

// AggregateSpec describes a deterministic projection/collection over JSON context.
type AggregateSpec struct {
	Source  string   `json:"source" toml:"source"`
	As      string   `json:"as,omitempty" toml:"as,omitempty"`
	Require []string `json:"require,omitempty" toml:"require,omitempty"`
	Include []string `json:"include,omitempty" toml:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty" toml:"exclude,omitempty"`
	Flatten bool     `json:"flatten,omitempty" toml:"flatten,omitempty"`
}

// FormSpec describes a human input form for execution="human_input" steps.
// Runtime support is layered on top of this schema: the compiled recipe carries
// the form so CLI and dashboard implementations can pause, render, validate, and
// resume a workflow around structured user input.
type FormSpec struct {
	Title       string       `json:"title,omitempty" toml:"title,omitempty"`
	Description string       `json:"description,omitempty" toml:"description,omitempty"`
	SubmitLabel string       `json:"submit_label,omitempty" toml:"submit_label,omitempty"`
	Fields      []*FormField `json:"fields,omitempty" toml:"fields,omitempty"`
}

// FormField describes one input control in a human input form.
// Supported Type values are intentionally small for the first version:
// input, textarea, radio, checkbox, and select.
type FormField struct {
	Name        string   `json:"name" toml:"name"`
	Label       string   `json:"label" toml:"label"`
	Type        string   `json:"type" toml:"type"`
	Required    bool     `json:"required,omitempty" toml:"required,omitempty"`
	Placeholder string   `json:"placeholder,omitempty" toml:"placeholder,omitempty"`
	Default     string   `json:"default,omitempty" toml:"default,omitempty"`
	Options     []string `json:"options,omitempty" toml:"options,omitempty"`
	Help        string   `json:"help,omitempty" toml:"help,omitempty"`
}

// ValidateSpec describes runtime validation for step output.
type ValidateSpec struct {
	Format       string   `json:"format,omitempty" toml:"format,omitempty"`
	Required     []string `json:"required,omitempty" toml:"required,omitempty"`
	ItemRequired []string `json:"item_required,omitempty" toml:"item_required,omitempty"`
	MinItems     int      `json:"min_items,omitempty" toml:"min_items,omitempty"`
}

// RetrySpec defines first-class transient retry semantics.
type RetrySpec struct {
	MaxAttempts int    `json:"max_attempts,omitempty" toml:"max_attempts,omitempty"`
	OnExhausted string `json:"on_exhausted,omitempty" toml:"on_exhausted,omitempty"`
}

// LoopSpec defines iteration over a body of steps.
type LoopSpec struct {
	Count          int     `json:"count,omitempty" toml:"count,omitempty"`
	Until          string  `json:"until,omitempty" toml:"until,omitempty"`
	Max            int     `json:"max,omitempty" toml:"max,omitempty"`
	Range          string  `json:"range,omitempty" toml:"range,omitempty"`
	ForEach        string  `json:"for_each,omitempty" toml:"for_each,omitempty"`
	Var            string  `json:"var,omitempty" toml:"var,omitempty"`
	Parallel       bool    `json:"parallel,omitempty" toml:"parallel,omitempty"`
	MaxConcurrency int     `json:"max_concurrency,omitempty" toml:"max_concurrency,omitempty"`
	Body           []*Step `json:"body" toml:"body"`
}

// OnCompleteSpec defines actions triggered when a step completes.
type OnCompleteSpec struct {
	ForEach    string            `json:"for_each,omitempty" toml:"for_each,omitempty"`
	Bond       string            `json:"bond,omitempty" toml:"bond,omitempty"`
	Vars       map[string]string `json:"vars,omitempty" toml:"vars,omitempty"`
	Parallel   bool              `json:"parallel,omitempty" toml:"parallel,omitempty"`
	Sequential bool              `json:"sequential,omitempty" toml:"sequential,omitempty"`
}

// BranchRule defines parallel execution paths that rejoin.
type BranchRule struct {
	From  string   `json:"from" toml:"from"`
	Steps []string `json:"steps" toml:"steps"`
	Join  string   `json:"join" toml:"join"`
}

// GateRule defines a condition that must be satisfied before a step proceeds.
type GateRule struct {
	Before    string `json:"before" toml:"before"`
	Condition string `json:"condition" toml:"condition"`
}

// ComposeRules define how formulas can be bonded together.
type ComposeRules struct {
	BondPoints []*BondPoint  `json:"bond_points,omitempty" toml:"bond_points,omitempty"`
	Hooks      []*Hook       `json:"hooks,omitempty" toml:"hooks,omitempty"`
	Expand     []*ExpandRule `json:"expand,omitempty" toml:"expand,omitempty"`
	Map        []*MapRule    `json:"map,omitempty" toml:"map,omitempty"`
	Branch     []*BranchRule `json:"branch,omitempty" toml:"branch,omitempty"`
	Gate       []*GateRule   `json:"gate,omitempty" toml:"gate,omitempty"`
	Aspects    []string      `json:"aspects,omitempty" toml:"aspects,omitempty"`
}

// ExpandRule applies an expansion template to a single target step.
type ExpandRule struct {
	Target string            `json:"target" toml:"target"`
	With   string            `json:"with" toml:"with"`
	Vars   map[string]string `json:"vars,omitempty" toml:"vars,omitempty"`
}

// MapRule applies an expansion template to all matching steps.
type MapRule struct {
	Select string            `json:"select" toml:"select"`
	With   string            `json:"with" toml:"with"`
	Vars   map[string]string `json:"vars,omitempty" toml:"vars,omitempty"`
}

// BondPoint is a named attachment site for composition.
type BondPoint struct {
	ID          string `json:"id" toml:"id"`
	Description string `json:"description,omitempty" toml:"description,omitempty"`
	AfterStep   string `json:"after_step,omitempty" toml:"after_step,omitempty"`
	BeforeStep  string `json:"before_step,omitempty" toml:"before_step,omitempty"`
	Parallel    bool   `json:"parallel,omitempty" toml:"parallel,omitempty"`
}

// Hook defines automatic formula attachment based on conditions.
type Hook struct {
	Trigger string            `json:"trigger" toml:"trigger"`
	Attach  string            `json:"attach" toml:"attach"`
	At      string            `json:"at,omitempty" toml:"at,omitempty"`
	Vars    map[string]string `json:"vars,omitempty" toml:"vars,omitempty"`
}

// Pointcut defines a target pattern for advice application.
type Pointcut struct {
	Glob  string `json:"glob,omitempty" toml:"glob,omitempty"`
	Type  string `json:"type,omitempty" toml:"type,omitempty"`
	Label string `json:"label,omitempty" toml:"label,omitempty"`
}

// AdviceRule defines a step transformation rule.
type AdviceRule struct {
	Target string        `json:"target" toml:"target"`
	Before *AdviceStep   `json:"before,omitempty" toml:"before,omitempty"`
	After  *AdviceStep   `json:"after,omitempty" toml:"after,omitempty"`
	Around *AroundAdvice `json:"around,omitempty" toml:"around,omitempty"`
}

// AdviceStep defines a step to insert via advice.
type AdviceStep struct {
	ID          string            `json:"id" toml:"id"`
	Title       string            `json:"title,omitempty" toml:"title,omitempty"`
	Description string            `json:"description,omitempty" toml:"description,omitempty"`
	Type        string            `json:"type,omitempty" toml:"type,omitempty"`
	Args        map[string]string `json:"args,omitempty" toml:"args,omitempty"`
	Output      map[string]string `json:"output,omitempty" toml:"output,omitempty"`
}

// AroundAdvice wraps a target with before and after steps.
type AroundAdvice struct {
	Before []*AdviceStep `json:"before,omitempty" toml:"before,omitempty"`
	After  []*AdviceStep `json:"after,omitempty" toml:"after,omitempty"`
}

// WaitsForSpec holds the parsed waits_for field.
type WaitsForSpec struct {
	Gate      string
	SpawnerID string
}

// ParseWaitsFor parses a waits_for value into its components.
func ParseWaitsFor(value string) *WaitsForSpec {
	if value == "" {
		return nil
	}

	if value == "all-children" || value == "any-children" {
		return &WaitsForSpec{Gate: value}
	}

	if strings.HasPrefix(value, "children-of(") && strings.HasSuffix(value, ")") {
		stepID := value[len("children-of(") : len(value)-1]
		return &WaitsForSpec{
			Gate:      "all-children",
			SpawnerID: stepID,
		}
	}

	return nil
}

func (f *Formula) RequiredVarNames() []string {
	var names []string
	for name, def := range f.Vars {
		if def != nil && def.Required {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// Validate checks the formula for structural errors.
func (f *Formula) Validate() error {
	var errs []string

	if f.Formula == "" {
		errs = append(errs, "formula: name is required")
	}

	if f.Version < 1 {
		errs = append(errs, "version: must be >= 1")
	}

	if contract := strings.TrimSpace(f.Contract); contract != "" && !strings.EqualFold(contract, "graph.v2") {
		errs = append(errs, fmt.Sprintf("contract: invalid value %q (must be graph.v2)", f.Contract))
	}

	if f.Type != "" && !f.Type.IsValid() {
		errs = append(errs, fmt.Sprintf("type: invalid value %q (must be workflow, expansion, or aspect)", f.Type))
	}

	// Validate variables
	for name, v := range f.Vars {
		if name == "" {
			errs = append(errs, "vars: variable name cannot be empty")
			continue
		}
		if v.Required && v.Default != nil {
			errs = append(errs, fmt.Sprintf("vars.%s: cannot have both required:true and default", name))
		}
	}

	// Validate steps
	stepIDLocations := make(map[string]string)
	for i, step := range f.Steps {
		prefix := fmt.Sprintf("steps[%d]", i)
		if step.ID == "" {
			errs = append(errs, fmt.Sprintf("%s: id is required", prefix))
			continue
		}
		if firstLoc, exists := stepIDLocations[step.ID]; exists {
			errs = append(errs, fmt.Sprintf("%s: duplicate id %q (first defined at %s)", prefix, step.ID, firstLoc))
		} else {
			stepIDLocations[step.ID] = prefix
		}

		if step.Title == "" && step.Expand == "" && step.Embed == "" {
			errs = append(errs, fmt.Sprintf("%s (%s): title is required (unless using expand/embed)", prefix, step.ID))
		}

		if step.Expand != "" && step.Embed != "" {
			errs = append(errs, fmt.Sprintf("%s (%s): expand and embed cannot be used together", prefix, step.ID))
		}
		if step.Embed != "" {
			if len(step.Children) > 0 {
				errs = append(errs, fmt.Sprintf("%s (%s): embed cannot be combined with children", prefix, step.ID))
			}
			if step.Loop != nil {
				errs = append(errs, fmt.Sprintf("%s (%s): embed cannot be combined with loop", prefix, step.ID))
			}
			if step.Agent != nil || step.Script != nil || step.Form != nil {
				errs = append(errs, fmt.Sprintf("%s (%s): embed cannot be combined with agent/script/form", prefix, step.ID))
			}
		}

		if step.Priority != nil && (*step.Priority < 0 || *step.Priority > 4) {
			errs = append(errs, fmt.Sprintf("%s (%s): priority must be 0-4", prefix, step.ID))
		}

		if err := validateStepTimeout(prefix, step.ID, step.Timeout, step.Retry != nil, nil, true); err != "" {
			errs = append(errs, err)
		}
		validateLoopBodyTimeouts(step.Loop, &errs, fmt.Sprintf("%s (%s).loop", prefix, step.ID), nil, true)

		if step.Retry != nil {
			validateRetry(step.Retry, &errs, fmt.Sprintf("%s (%s)", prefix, step.ID), step)
		}

		validateFormSpec(step.Form, &errs, fmt.Sprintf("%s (%s).form", prefix, step.ID))

		collectChildIDs(step.Children, stepIDLocations, &errs, prefix)
	}

	// Validate dependencies
	for i, step := range f.Steps {
		for _, dep := range step.DependsOn {
			if _, exists := stepIDLocations[dep]; !exists {
				errs = append(errs, fmt.Sprintf("steps[%d] (%s): depends_on references unknown step %q", i, step.ID, dep))
			}
		}
		for _, need := range step.Needs {
			if _, exists := stepIDLocations[need]; !exists {
				errs = append(errs, fmt.Sprintf("steps[%d] (%s): needs references unknown step %q", i, step.ID, need))
			}
		}
		if step.WaitsFor != "" {
			if err := validateWaitsFor(step.WaitsFor, stepIDLocations); err != nil {
				errs = append(errs, fmt.Sprintf("steps[%d] (%s): %s", i, step.ID, err.Error()))
			}
		}
		if step.OnComplete != nil {
			validateOnComplete(step.OnComplete, &errs, fmt.Sprintf("steps[%d] (%s)", i, step.ID))
		}
		validateChildDependsOn(step.Children, stepIDLocations, &errs, fmt.Sprintf("steps[%d]", i))
	}

	// Validate compose rules
	if f.Compose != nil {
		for i, bp := range f.Compose.BondPoints {
			if bp.ID == "" {
				errs = append(errs, fmt.Sprintf("compose.bond_points[%d]: id is required", i))
			}
			if bp.AfterStep != "" && bp.BeforeStep != "" {
				errs = append(errs, fmt.Sprintf("compose.bond_points[%d] (%s): cannot have both after_step and before_step", i, bp.ID))
			}
		}

		for i, hook := range f.Compose.Hooks {
			if hook.Trigger == "" {
				errs = append(errs, fmt.Sprintf("compose.hooks[%d]: trigger is required", i))
			}
			if hook.Attach == "" {
				errs = append(errs, fmt.Sprintf("compose.hooks[%d]: attach is required", i))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("formula validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return nil
}

func validateFormSpec(form *FormSpec, errs *[]string, prefix string) {
	if form == nil {
		return
	}
	if len(form.Fields) == 0 {
		*errs = append(*errs, fmt.Sprintf("%s: fields are required", prefix))
		return
	}
	seen := map[string]struct{}{}
	for i, field := range form.Fields {
		fieldPrefix := fmt.Sprintf("%s.fields[%d]", prefix, i)
		if field == nil {
			*errs = append(*errs, fmt.Sprintf("%s: field cannot be null", fieldPrefix))
			continue
		}
		name := strings.TrimSpace(field.Name)
		if name == "" {
			*errs = append(*errs, fmt.Sprintf("%s: name is required", fieldPrefix))
		} else if _, ok := seen[name]; ok {
			*errs = append(*errs, fmt.Sprintf("%s: duplicate field name %q", fieldPrefix, name))
		} else {
			seen[name] = struct{}{}
		}
		if strings.TrimSpace(field.Label) == "" {
			*errs = append(*errs, fmt.Sprintf("%s (%s): label is required", fieldPrefix, name))
		}
		fieldType := strings.TrimSpace(field.Type)
		if fieldType == "" {
			fieldType = "input"
		}
		if !isValidFormFieldType(fieldType) {
			*errs = append(*errs, fmt.Sprintf("%s (%s): invalid type %q (must be input, textarea, radio, checkbox, or select)", fieldPrefix, name, field.Type))
		}
		if (fieldType == "radio" || fieldType == "checkbox" || fieldType == "select") && len(field.Options) == 0 {
			*errs = append(*errs, fmt.Sprintf("%s (%s): options are required for %s fields", fieldPrefix, name, fieldType))
		}
	}
}

func isValidFormFieldType(fieldType string) bool {
	switch fieldType {
	case "input", "textarea", "radio", "checkbox", "select":
		return true
	default:
		return false
	}
}

func validateStepTimeout(prefix, stepID, raw string, hasRetry bool, allowedLoopVars map[string]struct{}, allowUnresolvedVars bool) string {
	if raw == "" {
		return ""
	}
	if err := validatePositiveTimeout(fmt.Sprintf("%s (%s)", prefix, stepID), raw, allowedLoopVars, allowUnresolvedVars); err != "" {
		return err
	}
	if !hasRetry {
		return fmt.Sprintf("%s (%s): timeout requires retry", prefix, stepID)
	}
	return ""
}

func validatePositiveTimeout(prefix, raw string, allowedLoopVars map[string]struct{}, allowUnresolvedVars bool) string {
	if raw == "" {
		return ""
	}
	parseRaw := substituteAllowedTimeoutLoopVars(raw, allowedLoopVars)
	if allowUnresolvedVars && rangeVarPattern.MatchString(parseRaw) {
		return ""
	}
	d, err := time.ParseDuration(parseRaw)
	if err != nil {
		return fmt.Sprintf("%s: invalid timeout %q: %v", prefix, raw, err)
	}
	if d <= 0 {
		return fmt.Sprintf("%s: timeout must be positive, got %v", prefix, d)
	}
	return ""
}

func substituteAllowedTimeoutLoopVars(raw string, allowedLoopVars map[string]struct{}) string {
	if len(allowedLoopVars) == 0 {
		return raw
	}
	return rangeVarPattern.ReplaceAllStringFunc(raw, func(match string) string {
		name := match[1 : len(match)-1]
		if _, ok := allowedLoopVars[name]; ok {
			return "1"
		}
		return match
	})
}

func validateLoopBodyTimeouts(loop *LoopSpec, errs *[]string, prefix string, allowedLoopVars map[string]struct{}, allowUnresolvedVars bool) {
	if loop == nil {
		return
	}
	validateNestedStepTimeoutsWithOptions(loop.Body, errs, prefix+".body", timeoutLoopVarsFor(loop, allowedLoopVars), allowUnresolvedVars)
}

func timeoutLoopVarsFor(loop *LoopSpec, parent map[string]struct{}) map[string]struct{} {
	if loop == nil || loop.Var == "" {
		return parent
	}
	vars := make(map[string]struct{}, len(parent)+1)
	for k, v := range parent {
		vars[k] = v
	}
	vars[loop.Var] = struct{}{}
	return vars
}

func validateNestedStepTimeoutsWithOptions(steps []*Step, errs *[]string, prefix string, allowedLoopVars map[string]struct{}, allowUnresolvedVars bool) {
	for i, step := range steps {
		if step == nil {
			continue
		}
		stepPrefix := fmt.Sprintf("%s[%d]", prefix, i)
		if err := validateStepTimeout(stepPrefix, step.ID, step.Timeout, step.Retry != nil, allowedLoopVars, allowUnresolvedVars); err != "" {
			*errs = append(*errs, err)
		}
		validateNestedStepTimeoutsWithOptions(step.Children, errs, stepPrefix+".children", allowedLoopVars, allowUnresolvedVars)
		validateLoopBodyTimeouts(step.Loop, errs, fmt.Sprintf("%s (%s).loop", stepPrefix, step.ID), allowedLoopVars, allowUnresolvedVars)
	}
}

func collectChildIDs(children []*Step, idLocations map[string]string, errs *[]string, prefix string) {
	for i, child := range children {
		childPrefix := fmt.Sprintf("%s.children[%d]", prefix, i)
		if child.ID == "" {
			*errs = append(*errs, fmt.Sprintf("%s: id is required", childPrefix))
			continue
		}
		if firstLoc, exists := idLocations[child.ID]; exists {
			*errs = append(*errs, fmt.Sprintf("%s: duplicate id %q (first defined at %s)", childPrefix, child.ID, firstLoc))
		} else {
			idLocations[child.ID] = childPrefix
		}

		if child.Title == "" && child.Expand == "" && child.Embed == "" {
			*errs = append(*errs, fmt.Sprintf("%s (%s): title is required", childPrefix, child.ID))
		}

		if child.Expand != "" && child.Embed != "" {
			*errs = append(*errs, fmt.Sprintf("%s (%s): expand and embed cannot be used together", childPrefix, child.ID))
		}
		if child.Embed != "" {
			if len(child.Children) > 0 {
				*errs = append(*errs, fmt.Sprintf("%s (%s): embed cannot be combined with children", childPrefix, child.ID))
			}
			if child.Loop != nil {
				*errs = append(*errs, fmt.Sprintf("%s (%s): embed cannot be combined with loop", childPrefix, child.ID))
			}
			if child.Agent != nil || child.Script != nil || child.Form != nil {
				*errs = append(*errs, fmt.Sprintf("%s (%s): embed cannot be combined with agent/script/form", childPrefix, child.ID))
			}
		}

		if child.Priority != nil && (*child.Priority < 0 || *child.Priority > 4) {
			*errs = append(*errs, fmt.Sprintf("%s (%s): priority must be 0-4", childPrefix, child.ID))
		}

		if err := validateStepTimeout(childPrefix, child.ID, child.Timeout, child.Retry != nil, nil, true); err != "" {
			*errs = append(*errs, err)
		}
		validateLoopBodyTimeouts(child.Loop, errs, fmt.Sprintf("%s (%s).loop", childPrefix, child.ID), nil, true)

		if child.Retry != nil {
			validateRetry(child.Retry, errs, fmt.Sprintf("%s (%s)", childPrefix, child.ID), child)
		}

		collectChildIDs(child.Children, idLocations, errs, childPrefix)
	}
}

func validateWaitsFor(value string, stepIDLocations map[string]string) error {
	if value == "all-children" || value == "any-children" {
		return nil
	}

	if strings.HasPrefix(value, "children-of(") && strings.HasSuffix(value, ")") {
		stepID := value[len("children-of(") : len(value)-1]
		if stepID == "" {
			return fmt.Errorf("waits_for children-of() requires a step ID")
		}
		if _, exists := stepIDLocations[stepID]; !exists {
			return fmt.Errorf("waits_for references unknown step %q in children-of()", stepID)
		}
		return nil
	}

	return fmt.Errorf("waits_for has invalid value %q (must be all-children, any-children, or children-of(step-id))", value)
}

func validateChildDependsOn(children []*Step, idLocations map[string]string, errs *[]string, prefix string) {
	for i, child := range children {
		childPrefix := fmt.Sprintf("%s.children[%d]", prefix, i)
		for _, dep := range child.DependsOn {
			if _, exists := idLocations[dep]; !exists {
				*errs = append(*errs, fmt.Sprintf("%s (%s): depends_on references unknown step %q", childPrefix, child.ID, dep))
			}
		}
		for _, need := range child.Needs {
			if _, exists := idLocations[need]; !exists {
				*errs = append(*errs, fmt.Sprintf("%s (%s): needs references unknown step %q", childPrefix, child.ID, need))
			}
		}
		if child.WaitsFor != "" {
			if err := validateWaitsFor(child.WaitsFor, idLocations); err != nil {
				*errs = append(*errs, fmt.Sprintf("%s (%s): %s", childPrefix, child.ID, err.Error()))
			}
		}
		if child.OnComplete != nil {
			validateOnComplete(child.OnComplete, errs, fmt.Sprintf("%s (%s)", childPrefix, child.ID))
		}
		validateChildDependsOn(child.Children, idLocations, errs, childPrefix)
	}
}

func validateOnComplete(oc *OnCompleteSpec, errs *[]string, prefix string) {
	if oc.ForEach != "" && oc.Bond == "" {
		*errs = append(*errs, fmt.Sprintf("%s.on_complete: bond is required when for_each is set", prefix))
	}
	if oc.ForEach == "" && oc.Bond != "" {
		*errs = append(*errs, fmt.Sprintf("%s.on_complete: for_each is required when bond is set", prefix))
	}
	if oc.ForEach != "" && !strings.HasPrefix(oc.ForEach, "output.") {
		*errs = append(*errs, fmt.Sprintf("%s.on_complete: for_each must start with 'output.' (got %q)", prefix, oc.ForEach))
	}
	if oc.Parallel && oc.Sequential {
		*errs = append(*errs, fmt.Sprintf("%s.on_complete: cannot set both parallel and sequential", prefix))
	}
}

func validateRetry(spec *RetrySpec, errs *[]string, prefix string, step *Step) {
	if spec.MaxAttempts < 1 {
		*errs = append(*errs, fmt.Sprintf("%s.retry: max_attempts must be >= 1", prefix))
	}
	switch spec.OnExhausted {
	case "", "hard_fail", "soft_fail":
	default:
		*errs = append(*errs, fmt.Sprintf("%s.retry: unsupported on_exhausted %q (want hard_fail or soft_fail)", prefix, spec.OnExhausted))
	}

	if step.Loop != nil {
		*errs = append(*errs, fmt.Sprintf("%s: retry cannot be combined with loop", prefix))
	}
	if step.OnComplete != nil {
		*errs = append(*errs, fmt.Sprintf("%s: retry cannot be combined with on_complete", prefix))
	}
	if step.Gate != nil {
		*errs = append(*errs, fmt.Sprintf("%s: retry cannot be combined with gate", prefix))
	}
	if step.Expand != "" {
		*errs = append(*errs, fmt.Sprintf("%s: retry cannot be combined with expand", prefix))
	}
	if len(step.Children) > 0 {
		*errs = append(*errs, fmt.Sprintf("%s: retry cannot be combined with children", prefix))
	}
}

// GetRequiredVars returns the names of all required variables.
func (f *Formula) GetRequiredVars() []string {
	var required []string
	for name, v := range f.Vars {
		if v.Required {
			required = append(required, name)
		}
	}
	return required
}

// GetStepByID finds a step by its ID (searches recursively).
func (f *Formula) GetStepByID(id string) *Step {
	for _, step := range f.Steps {
		if found := findStepByID(step, id); found != nil {
			return found
		}
	}
	return nil
}

func findStepByID(step *Step, id string) *Step {
	if step.ID == id {
		return step
	}
	for _, child := range step.Children {
		if found := findStepByID(child, id); found != nil {
			return found
		}
	}
	return nil
}

// StringPtr returns a pointer to s.
func StringPtr(s string) *string { return &s }

// GetBondPoint finds a bond point by ID.
func (f *Formula) GetBondPoint(id string) *BondPoint {
	if f.Compose == nil {
		return nil
	}
	for _, bp := range f.Compose.BondPoints {
		if bp.ID == id {
			return bp
		}
	}
	return nil
}

// varPattern matches {{variable}} placeholders.
var varPattern = regexp.MustCompile(`\{\{([a-zA-Z_][a-zA-Z0-9_]*)\}\}`)

// ExtractVariables finds all {{variable}} references in a formula.
func ExtractVariables(formula *Formula) []string {
	seen := make(map[string]bool)
	var vars []string

	extract := func(s string) {
		matches := varPattern.FindAllStringSubmatch(s, -1)
		for _, match := range matches {
			if len(match) >= 2 && !seen[match[1]] {
				seen[match[1]] = true
				vars = append(vars, match[1])
			}
		}
	}

	extract(formula.Description)

	var extractFromStep func(*Step)
	extractFromStep = func(step *Step) {
		extract(step.Title)
		extract(step.Description)
		extract(step.Assignee)
		extract(step.Condition)
		for _, l := range step.Labels {
			extract(l)
		}
		for _, child := range step.Children {
			extractFromStep(child)
		}
	}

	for _, step := range formula.Steps {
		extractFromStep(step)
	}

	return vars
}

// Substitute replaces {{variable}} placeholders with values.
func Substitute(s string, vars map[string]string) string {
	return varPattern.ReplaceAllStringFunc(s, func(match string) string {
		name := match[2 : len(match)-2]
		if val, ok := vars[name]; ok {
			return val
		}
		return match
	})
}

// CheckResidualVars returns the names of any {{...}} placeholders remaining
// after substitution.
func CheckResidualVars(s string) []string {
	matches := varPattern.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(matches))
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		if seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		names = append(names, m[1])
	}
	return names
}

// ValidateVars checks that all required variables are provided
// and all values pass their constraints.
func ValidateVars(formula *Formula, values map[string]string) error {
	errs, _ := CollectVarValidationErrors(formula.Vars, values)
	return formatVarValidationErrors(errs)
}

// ValidateVarDefs validates explicit var definitions against provided values.
func ValidateVarDefs(defs map[string]*VarDef, values map[string]string) error {
	errs, _ := CollectVarValidationErrors(defs, values)
	return formatVarValidationErrors(errs)
}

// CollectVarValidationErrors validates explicit var definitions against the
// provided values and returns raw error strings plus the set of missing
// required vars.
func CollectVarValidationErrors(defs map[string]*VarDef, values map[string]string) ([]string, map[string]bool) {
	return collectVarValidationErrors(defs, values, true)
}

func collectVarValidationErrors(defs map[string]*VarDef, values map[string]string, requireMissing bool) ([]string, map[string]bool) {
	var errs []string
	missingRequired := make(map[string]bool)
	names := make([]string, 0, len(defs))
	for name := range defs {
		names = append(names, name)
	}

	for _, name := range names {
		def := defs[name]
		if def == nil {
			continue
		}
		val, provided := values[name]

		if requireMissing && def.Required && !provided {
			errs = append(errs, fmt.Sprintf("variable %q is required", name))
			missingRequired[name] = true
			continue
		}

		if !provided && def.Default != nil {
			val = *def.Default
		}

		if val == "" {
			continue
		}

		if len(def.Enum) > 0 {
			found := false
			for _, allowed := range def.Enum {
				if val == allowed {
					found = true
					break
				}
			}
			if !found {
				errs = append(errs, fmt.Sprintf("variable %q: value %q not in allowed values %v", name, val, def.Enum))
			}
		}

		if def.Pattern != "" {
			re, err := regexp.Compile(def.Pattern)
			if err != nil {
				errs = append(errs, fmt.Sprintf("variable %q: invalid pattern %q: %v", name, def.Pattern, err))
			} else if !re.MatchString(val) {
				errs = append(errs, fmt.Sprintf("variable %q: value %q does not match pattern %q", name, val, def.Pattern))
			}
		}
	}

	if len(missingRequired) == 0 {
		missingRequired = nil
	}
	return errs, missingRequired
}

func formatVarValidationErrors(errs []string) error {
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("variable validation failed:\n  - %s", strings.Join(errs, "\n  - "))
}

// ApplyDefaults returns a new map with default values filled in.
func ApplyDefaults(formula *Formula, values map[string]string) map[string]string {
	result := make(map[string]string)

	for k, v := range values {
		result[k] = v
	}

	for name, def := range formula.Vars {
		if _, exists := result[name]; !exists && def != nil && def.Default != nil {
			result[name] = *def.Default
		}
	}

	return result
}

// SetSourceInfo populates the SourceFormula and SourceLocation fields on each step.
func SetSourceInfo(formula *Formula) {
	setSourceInfoRecursive(formula.Steps, formula.Formula, "steps")
	setSourceInfoRecursive(formula.Template, formula.Formula, "template")
}

func setSourceInfoRecursive(steps []*Step, formulaName, pathPrefix string) {
	for i, step := range steps {
		if step == nil {
			continue
		}
		step.SourceFormula = formulaName
		step.SourceLocation = fmt.Sprintf("%s[%d]", pathPrefix, i)

		if len(step.Children) > 0 {
			childPath := fmt.Sprintf("%s[%d].children", pathPrefix, i)
			setSourceInfoRecursive(step.Children, formulaName, childPath)
		}

		if step.Loop != nil && len(step.Loop.Body) > 0 {
			bodyPath := fmt.Sprintf("%s[%d].loop.body", pathPrefix, i)
			setSourceInfoRecursive(step.Loop.Body, formulaName, bodyPath)
		}
	}
}

// resolveDescriptionFiles walks all steps and replaces DescriptionFile
// with the file's contents.
func ResolveDescriptionFiles(steps []*Step, baseDir string) {
	for _, step := range steps {
		if step == nil || step.DescriptionFile == "" {
			continue
		}
		path := step.DescriptionFile
		if !filepath.IsAbs(path) {
			path = filepath.Join(baseDir, path)
		}
		data, err := os.ReadFile(path)
		if err == nil {
			step.Description = string(data)
		}
		step.DescriptionFile = ""
		if len(step.Children) > 0 {
			ResolveDescriptionFiles(step.Children, baseDir)
		}
	}
}
