package spec

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// UnmarshalJSON accepts the canonical public "check" spelling on Step.
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

// UnmarshalTOML accepts the canonical public "check" spelling on Step.
func (s *Step) UnmarshalTOML(data interface{}) error {
	raw, ok := data.(map[string]interface{})
	if !ok {
		return fmt.Errorf("type mismatch for spec.Step: expected table but found %T", data)
	}

	encoded, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("encode spec.Step: %w", err)
	}

	var decoded stepTOMLAlias
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return fmt.Errorf("decode spec.Step: %w", err)
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
		return fmt.Errorf("type mismatch for spec.VarDef: expected string or table but found %T", data)
	}
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
	ID              string               `json:"id"`
	Title           string               `json:"title"`
	Description     string               `json:"description,omitempty"`
	DescriptionFile string               `json:"description_file,omitempty"`
	Notes           string               `json:"notes,omitempty"`
	Type            string               `json:"type,omitempty"`
	Priority        *int                 `json:"priority,omitempty"`
	Labels          []string             `json:"tags,omitempty"`
	Metadata        map[string]string    `json:"metadata,omitempty"`
	DependsOn       []string             `json:"depends_on,omitempty"`
	Needs           []string             `json:"needs,omitempty"`
	WaitsFor        string               `json:"waits_for,omitempty"`
	Assignee        string               `json:"assignee,omitempty"`
	Expand          string               `json:"expand,omitempty"`
	ExpandVars      map[string]string    `json:"expand_vars,omitempty"`
	Embed           string               `json:"embed,omitempty"`
	EmbedVars       map[string]string    `json:"embed_vars,omitempty"`
	Condition       string               `json:"condition,omitempty"`
	Children        []*stepTOMLAlias     `json:"children,omitempty"`
	Gate            *Gate                `json:"gate,omitempty"`
	Loop            *loopTOMLAlias       `json:"loop,omitempty"`
	OnComplete      *OnCompleteSpec      `json:"on_complete,omitempty"`
	Check           json.RawMessage      `json:"check,omitempty"`
	Ralph           json.RawMessage      `json:"ralph,omitempty"`
	Retry           *RetrySpec           `json:"retry,omitempty"`
	Timeout         string               `json:"timeout,omitempty"`
	Agent           *AgentConfig         `json:"agent,omitempty"`
	Script          *ScriptSpec          `json:"script,omitempty"`
	ExternalAgent   *ExternalAgentConfig `json:"external_agent,omitempty"`
	Aggregate       *AggregateSpec       `json:"aggregate,omitempty"`
	Tool            *ToolSpec            `json:"tool,omitempty"`
	WriteFiles      *WriteFilesSpec      `json:"write_files,omitempty"`
	Form            json.RawMessage      `json:"form,omitempty"`
	DynamicForm     bool                 `json:"dynamic_form,omitempty"`
	Validate        *ValidateSpec        `json:"validate,omitempty"`
	OutputKey       string               `json:"output_key,omitempty"`
	InputCtx        []string             `json:"input_context,omitempty"`
	Execution       string               `json:"execution,omitempty"`
}

type loopTOMLAlias struct {
	Count          int              `json:"count,omitempty"`
	Until          string           `json:"until,omitempty"`
	Max            flexibleIntExpr  `json:"max,omitempty"`
	Range          string           `json:"range,omitempty"`
	ForEach        string           `json:"for_each,omitempty"`
	Var            string           `json:"var,omitempty"`
	Parallel       bool             `json:"parallel,omitempty"`
	MaxConcurrency int              `json:"max_concurrency,omitempty"`
	Body           []*stepTOMLAlias `json:"body"`
}

type flexibleIntExpr struct {
	Int  int
	Expr string
}

func (v *flexibleIntExpr) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	if strings.HasPrefix(trimmed, "\"") {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return nil
		}
		if n, err := strconv.Atoi(s); err == nil {
			v.Int = n
			v.Expr = ""
			return nil
		}
		v.Expr = s
		return nil
	}
	var n int
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	v.Int = n
	v.Expr = ""
	return nil
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
		ExternalAgent:   a.ExternalAgent,
		Aggregate:       a.Aggregate,
		Tool:            a.Tool,
		WriteFiles:      a.WriteFiles,
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
		Max:            a.Max.Int,
		MaxExpr:        a.Max.Expr,
		Range:          a.Range,
		ForEach:        a.ForEach,
		Var:            a.Var,
		Parallel:       a.Parallel,
		MaxConcurrency: a.MaxConcurrency,
		Body:           body,
	}, nil
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
