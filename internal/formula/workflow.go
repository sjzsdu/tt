package formula

import (
	"context"
	"fmt"
	"strings"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

// CompileWorkflowByName is the primary formula compile path: it resolves the
// formula source and compiles it directly into typed Workflow IR without going
// through Recipe/RecipeStep.
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
	meta := steps.Metadata{ID: steps.ID(step.ID), Title: step.Title, Labels: append([]string(nil), step.Labels...), Condition: step.Condition}
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
		return steps.ScriptStep{Base: steps.Base{Metadata: meta}, Command: command, Cwd: cwd, Env: env}
	case "human_input":
		meta.Kind = steps.KindHumanInput
		return steps.HumanInputStep{Base: steps.Base{Metadata: meta}, Reason: step.Description, Form: step.Form}
	default:
		meta.Kind = steps.KindAgent
		agentName := ""
		model := ""
		if step.Agent != nil {
			agentName = step.Agent.Name
			model = step.Agent.Model
		}
		return steps.AgentStep{Base: steps.Base{Metadata: meta}, Agent: agentName, Model: model, Prompt: step.Description, DynamicForm: step.DynamicForm}
	}
}

// WorkflowFromRecipe adapts the compiled Recipe into the new workflow IR.
// This is the bridge that lets cmd/formula keep using the existing parser while
// execution moves toward typed step runners and graph-first runtime semantics.
func WorkflowFromRecipe(recipe *Recipe) *ir.Workflow {
	if recipe == nil {
		return nil
	}
	wf := &ir.Workflow{
		ID:          ir.WorkflowID(recipe.Name),
		Name:        recipe.Name,
		Description: recipe.Description,
		Vars:        make(map[string]ir.VarSchema, len(recipe.Vars)),
		Graph:       ir.NewGraph(),
	}
	for name, def := range recipe.Vars {
		if def == nil {
			continue
		}
		wf.Vars[name] = ir.VarSchema{Type: def.Type, Required: def.Required, Default: def.Default}
	}
	for i := range recipe.Steps {
		step := recipe.Steps[i]
		wf.Graph.AddNode(&ir.Node{ID: ir.NodeID(step.ID), Step: typedStepFromRecipeStep(step)})
	}
	for _, dep := range recipe.Deps {
		wf.Graph.AddEdge(ir.NodeID(dep.DependsOnID), ir.NodeID(dep.StepID), dep.Type)
	}
	return wf
}

func typedStepFromRecipeStep(step RecipeStep) steps.Step {
	meta := steps.Metadata{ID: steps.ID(step.ID), Title: step.Title, DependsOn: nil, Labels: append([]string(nil), step.Labels...), Condition: step.Condition}
	if step.IsRoot || (step.Metadata != nil && step.Metadata["formula_boundary"] != "") {
		meta.Kind = steps.KindNoop
		return steps.NoopStep{Base: steps.Base{Metadata: meta}}
	}
	switch step.Execution {
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
		return steps.ScriptStep{Base: steps.Base{Metadata: meta}, Command: command, Cwd: cwd, Env: env, OutputKey: step.OutputKey}
	case "human_input":
		meta.Kind = steps.KindHumanInput
		return steps.HumanInputStep{Base: steps.Base{Metadata: meta}, Reason: step.Description, Form: step.Form, OutputKey: step.OutputKey}
	default:
		meta.Kind = steps.KindAgent
		agentName := ""
		model := ""
		if step.Agent != nil {
			agentName = step.Agent.Name
			model = step.Agent.Model
		}
		return steps.AgentStep{Base: steps.Base{Metadata: meta}, Agent: agentName, Model: model, Prompt: step.Description, DynamicForm: step.DynamicForm, OutputKey: step.OutputKey}
	}
}
