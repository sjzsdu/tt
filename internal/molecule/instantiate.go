package molecule

import (
	"fmt"
	"strings"

	"github.com/sjzsdu/tt/internal/formula"
)

func Instantiate(recipe *formula.Recipe, opts Options) (*Result, error) {
	if recipe == nil {
		return nil, fmt.Errorf("recipe is nil")
	}
	if len(recipe.Steps) == 0 {
		return nil, fmt.Errorf("recipe %q has no steps", recipe.Name)
	}

	vars := applyVarDefaults(opts.Vars, recipe.Vars)
	priorityOverride := clonePriority(opts.PriorityOverride)

	idMapping := make(map[string]string)
	var tasks []TaskItem

	for i, step := range recipe.Steps {
		if recipe.RootOnly && i > 0 {
			break
		}

		task := stepToTask(step, vars, priorityOverride)

		if step.IsRoot {
			if opts.Title != "" {
				task.Title = formula.Substitute(opts.Title, vars)
			}
			if opts.ParentID != "" {
				task.ParentID = opts.ParentID
			}
		} else {
			for _, dep := range recipe.Deps {
				if dep.StepID == step.ID && dep.Type == "parent-child" {
					if parentTaskID, ok := idMapping[dep.DependsOnID]; ok {
						task.ParentID = parentTaskID
					}
					break
				}
			}
		}

		for _, dep := range recipe.Deps {
			if dep.StepID != step.ID || dep.Type == "parent-child" {
				continue
			}
			if dependsOnID, ok := idMapping[dep.DependsOnID]; ok {
				task.Dependencies = append(task.Dependencies, dependsOnID)
			}
		}

		if strings.Contains(task.Title, "{{") {
			if residual := formula.CheckResidualVars(task.Title); len(residual) > 0 {
				return nil, fmt.Errorf("step %q: title contains unresolved variable(s) %s", step.ID, strings.Join(residual, ", "))
			}
		}

		idMapping[step.ID] = task.ID
		tasks = append(tasks, task)
	}

	rootID := ""
	if len(tasks) > 0 {
		rootID = tasks[0].ID
	}

	return &Result{
		RootID:    rootID,
		IDMapping: idMapping,
		Created:   len(tasks),
		Tasks:     tasks,
	}, nil
}

func stepToTask(step formula.RecipeStep, vars map[string]string, priorityOverride *int) TaskItem {
	stepType := step.Type
	if stepType == "" {
		stepType = "task"
	}

	task := TaskItem{
		ID:       step.ID,
		Title:    formula.Substitute(step.Title, vars),
		Type:     stepType,
		Priority: resolveStepPriority(step, priorityOverride),
		IsRoot:   step.IsRoot,
	}

	if step.Description != "" {
		task.Description = formula.Substitute(step.Description, vars)
	}
	if step.Notes != "" {
		task.Notes = formula.Substitute(step.Notes, vars)
	}
	if step.Assignee != "" {
		task.Assignee = formula.Substitute(step.Assignee, vars)
	}
	if len(step.Labels) > 0 {
		task.Labels = make([]string, len(step.Labels))
		for i, l := range step.Labels {
			task.Labels[i] = formula.Substitute(l, vars)
		}
	}
	if len(step.Metadata) > 0 {
		task.Metadata = make(map[string]string, len(step.Metadata))
		for k, v := range step.Metadata {
			task.Metadata[k] = formula.Substitute(v, vars)
		}
	}

	return task
}

func resolveStepPriority(step formula.RecipeStep, priorityOverride *int) *int {
	if priorityOverride != nil {
		return clonePriority(priorityOverride)
	}
	return clonePriority(step.Priority)
}

func clonePriority(v *int) *int {
	if v == nil {
		return nil
	}
	cloned := *v
	return &cloned
}

func applyVarDefaults(vars map[string]string, defs map[string]*formula.VarDef) map[string]string {
	result := make(map[string]string, len(vars)+len(defs))
	for name, def := range defs {
		if def != nil && def.Default != nil {
			result[name] = *def.Default
		}
	}
	for k, v := range vars {
		result[k] = v
	}
	return result
}
