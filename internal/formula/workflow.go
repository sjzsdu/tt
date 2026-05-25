package formula

import (
	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

// WorkflowFromRecipe adapts the legacy compiled Recipe into the new workflow IR.
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
	meta := steps.Metadata{ID: steps.ID(step.ID), Title: step.Title, DependsOn: nil, Labels: append([]string(nil), step.Labels...)}
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
		return steps.AgentStep{Base: steps.Base{Metadata: meta}, Agent: agentName, Model: model, Prompt: step.Description, OutputKey: step.OutputKey}
	}
}
