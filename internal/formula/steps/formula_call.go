package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sjzsdu/tt/internal/formula/ast"
)

// FormulaCallStep treats another Formula as one executable, composable step.
// With is the explicit input binding contract; the child Formula's declared
// outputs are returned as one JSON object.
type FormulaCallStep struct {
	Base
	Formula   string            `json:"formula"`
	With      map[string]string `json:"with,omitempty"`
	OutputKey string            `json:"output_key,omitempty"`
}

type FormulaCallDecoder struct{}

func (FormulaCallDecoder) Kind() Kind { return KindFormula }

func (FormulaCallDecoder) Decode(decl ast.StepDecl) (Step, error) {
	var step FormulaCallStep
	if err := json.Unmarshal(decl.Raw, &step); err != nil {
		return nil, fmt.Errorf("decode formula step: %w", err)
	}
	step.Base = Base{metadataFromDecl(decl, KindFormula)}
	return step, nil
}

func (s FormulaCallStep) Validate(ValidationContext) error {
	if strings.TrimSpace(s.Formula) == "" {
		return fmt.Errorf("formula is required")
	}
	return nil
}

func (s FormulaCallStep) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	if req.Capabilities.Workflows == nil {
		err := &StepError{Message: "workflow capability is required"}
		return &RunResult{Status: StatusFailed, Error: err}, err
	}
	inputs := make(map[string]Value, len(s.With))
	for name, binding := range s.With {
		inputs[name] = bindFormulaInput(binding, req.InputView())
	}
	result, err := req.Capabilities.Workflows.RunWorkflow(ctx, WorkflowRequest{
		RunID: req.RunID, NodeID: req.NodeID, Formula: strings.TrimSpace(s.Formula), Inputs: inputs,
	})
	if result == nil {
		result = &WorkflowResult{}
	}
	if err != nil {
		stepErr := result.Error
		if stepErr == nil {
			stepErr = &StepError{Message: "formula step failed", Cause: err}
		}
		return &RunResult{Status: StatusFailed, Error: stepErr}, err
	}
	if result.Status == StatusWaiting {
		out := &RunResult{Status: StatusWaiting, Outputs: result.Outputs, Await: result.Await, Error: result.Error}
		out.NormalizeOutputs()
		return out, nil
	}
	if result.Status == StatusFailed {
		stepErr := result.Error
		if stepErr == nil {
			stepErr = &StepError{Message: "formula step failed"}
		}
		return &RunResult{Status: StatusFailed, Error: stepErr}, stepErr
	}
	out := &RunResult{Status: StatusCompleted, Outputs: result.Outputs}
	out.NormalizeOutputs()
	return out, nil
}

func bindFormulaInput(binding string, ctx ContextView) Value {
	trimmed := strings.TrimSpace(binding)
	if match := runtimeTemplatePattern.FindStringSubmatch(trimmed); len(match) == 2 && match[0] == trimmed && ctx != nil {
		if value, ok := ctx.Get(match[1]); ok {
			return value
		}
	}
	rendered := renderContextTemplates(binding, ctx)
	raw, _ := json.Marshal(rendered)
	return Value{Type: "json", Raw: raw}
}
