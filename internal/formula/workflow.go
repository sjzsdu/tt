package formula

import (
	"context"
	"fmt"
	"strings"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

// CompileWorkflowByName is the primary formula compile path: it resolves the
// formula source and compiles it directly into typed Workflow IR.
func CompileWorkflowByName(_ context.Context, name string, searchPaths []string, vars map[string]string) (*ir.Workflow, error) {
	f, err := resolveFormulaForWorkflow(name, searchPaths, vars, true)
	if err != nil {
		return nil, err
	}
	return WorkflowFromFormula(f), nil
}

func resolveFormulaForWorkflow(name string, searchPaths []string, vars map[string]string, validateRuntimeVars bool) (*Formula, error) {
	parser := NewParser(searchPaths...)
	f, err := parser.LoadByName(name)
	if err != nil {
		return nil, fmt.Errorf("loading formula %q: %w", name, err)
	}
	resolved, err := parser.Resolve(f)
	if err != nil {
		return nil, fmt.Errorf("resolving formula %q: %w", name, err)
	}
	if validateRuntimeVars && len(vars) > 0 {
		if err := ValidateVars(resolved, vars); err != nil {
			return nil, err
		}
	}
	compileVars := make(map[string]string)
	for vname, def := range resolved.Vars {
		if def != nil && def.Default != nil {
			compileVars[vname] = *def.Default
		}
	}
	for k, v := range vars {
		compileVars[k] = v
	}
	if err := validateCompileTimeVars(resolved, vars); err != nil {
		return nil, err
	}
	controlFlowSteps, err := ApplyControlFlowWithVars(resolved.Steps, resolved.Compose, compileVars)
	if err != nil {
		return nil, fmt.Errorf("applying control flow to %q: %w", name, err)
	}
	resolved.Steps = controlFlowSteps
	if len(resolved.Advice) > 0 {
		resolved.Steps = ApplyAdvice(resolved.Steps, resolved.Advice)
	}
	inlineExpandedSteps, err := ApplyInlineExpansionsWithVars(resolved.Steps, parser, compileVars)
	if err != nil {
		return nil, fmt.Errorf("applying inline expansions to %q: %w", name, err)
	}
	resolved.Steps = inlineExpandedSteps
	if resolved.Compose != nil && (len(resolved.Compose.Expand) > 0 || len(resolved.Compose.Map) > 0) {
		expandedSteps, err := ApplyExpansionsWithVars(resolved.Steps, resolved.Compose, parser, compileVars)
		if err != nil {
			return nil, fmt.Errorf("applying expansions to %q: %w", name, err)
		}
		resolved.Steps = expandedSteps
	}
	embeddedSteps, err := ApplyEmbedsWithVars(resolved.Steps, parser, compileVars, []string{name})
	if err != nil {
		return nil, fmt.Errorf("applying embeds to %q: %w", name, err)
	}
	resolved.Steps = embeddedSteps
	filteredSteps, err := FilterStepsByCondition(resolved.Steps, compileVars)
	if err != nil {
		return nil, fmt.Errorf("filtering steps by condition: %w", err)
	}
	resolved.Steps = filteredSteps
	return resolved, nil
}

func WorkflowFromFormula(f *Formula) *ir.Workflow {
	if f == nil {
		return nil
	}
	wf := &ir.Workflow{ID: ir.WorkflowID(f.Formula), Name: f.Formula, Description: f.Description, Vars: make(map[string]ir.VarSchema, len(f.Vars)), Graph: ir.NewGraph()}
	for name, def := range f.Vars {
		if def == nil {
			continue
		}
		wf.Vars[name] = ir.VarSchema{Type: def.Type, Required: def.Required, Default: def.Default}
	}
	for _, step := range f.Steps {
		addFormulaStepToWorkflow(wf, step)
	}
	return wf
}

func addFormulaStepToWorkflow(wf *ir.Workflow, step *Step) {
	if wf == nil || step == nil {
		return
	}
	wf.Graph.AddNode(&ir.Node{ID: ir.NodeID(step.ID), Step: typedStepFromFormulaStep(step)})
	for _, dep := range append([]string{}, step.DependsOn...) {
		if strings.TrimSpace(dep) != "" {
			wf.Graph.AddEdge(ir.NodeID(dep), ir.NodeID(step.ID), "blocks")
		}
	}
	for _, dep := range step.Needs {
		if strings.TrimSpace(dep) != "" {
			wf.Graph.AddEdge(ir.NodeID(dep), ir.NodeID(step.ID), "blocks")
		}
	}
}

func typedStepFromFormulaStep(step *Step) steps.Step {
	dependsOn := make([]steps.ID, 0, len(step.DependsOn)+len(step.Needs))
	for _, dep := range append(append([]string(nil), step.DependsOn...), step.Needs...) {
		if strings.TrimSpace(dep) != "" {
			dependsOn = append(dependsOn, steps.ID(dep))
		}
	}
	meta := steps.Metadata{ID: steps.ID(step.ID), Title: step.Title, DependsOn: dependsOn, Labels: append([]string(nil), step.Labels...), Condition: step.Condition}
	if step.Loop != nil {
		meta.Kind = steps.KindLoop
		body := make([]steps.Step, 0, len(step.Loop.Body))
		for _, child := range step.Loop.Body {
			if child == nil {
				continue
			}
			body = append(body, typedStepFromFormulaStep(child))
		}
		return steps.LoopStep{Base: steps.Base{Metadata: meta}, Body: body, Parallel: step.Loop.Parallel, MaxConcurrency: step.Loop.MaxConcurrency, Until: step.Loop.Until, Max: step.Loop.Max}
	}
	execution := strings.TrimSpace(step.Execution)
	switch execution {
	case "noop":
		meta.Kind = steps.KindNoop
		return steps.NoopStep{Base: steps.Base{Metadata: meta}}
	case "script":
		meta.Kind = steps.KindScript
		command := []string(nil)
		cwd := ""
		env := map[string]string(nil)
		if step.Script != nil {
			command = append([]string(nil), step.Script.Command...)
			cwd = step.Script.Cwd
			env = step.Script.Env
		}
		return steps.ScriptStep{Base: steps.Base{Metadata: meta}, Command: command, Cwd: cwd, Env: env, OutputKey: step.OutputKey, Validation: outputValidationSpec(step)}
	case "human_input":
		meta.Kind = steps.KindHumanInput
		return steps.HumanInputStep{Base: steps.Base{Metadata: meta}, Reason: step.Description, Form: step.Form, OutputKey: step.OutputKey, Validation: outputValidationSpec(step)}
	default:
		meta.Kind = steps.KindAgent
		agentName := ""
		model := ""
		if step.Agent != nil {
			agentName = step.Agent.Name
			model = step.Agent.Model
		}
		return steps.AgentStep{Base: steps.Base{Metadata: meta}, Agent: agentName, Model: model, Prompt: step.Description, InputCtx: append([]string(nil), step.InputCtx...), DynamicForm: step.DynamicForm, OutputKey: step.OutputKey, Validation: outputValidationSpec(step)}
	}
}

func outputValidationSpec(step *Step) *steps.OutputValidationSpec {
	if step == nil || step.Validate == nil {
		return nil
	}
	return &steps.OutputValidationSpec{
		Format:       step.Validate.Format,
		Required:     append([]string(nil), step.Validate.Required...),
		ItemRequired: append([]string(nil), step.Validate.ItemRequired...),
		MinItems:     step.Validate.MinItems,
	}
}
