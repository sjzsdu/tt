package formula

import (
	"fmt"
	spec "github.com/sjzsdu/tt/internal/formula/spec"
	"strings"
)

const DefaultMaxExpansionDepth = 5

func ApplyExpansionsWithVars(steps []*spec.Step, compose *spec.ComposeRules, parser *Parser, parentVars map[string]string) ([]*spec.Step, error) {
	if compose == nil || parser == nil {
		return steps, nil
	}

	if len(compose.Expand) == 0 && len(compose.Map) == 0 {
		return steps, nil
	}

	stepMap := buildStepMap(steps)
	expanded := make(map[string]bool)

	result := steps
	for _, rule := range compose.Expand {
		targetStep, ok := stepMap[rule.Target]
		if !ok {
			return nil, fmt.Errorf("expand: target step %q not found", rule.Target)
		}

		if expanded[rule.Target] {
			continue
		}

		expFormula, err := parser.LoadByName(rule.With)
		if err != nil {
			return nil, fmt.Errorf("expand: loading formula %q: %w", rule.With, err)
		}

		if expFormula.Type != spec.TypeExpansion {
			return nil, fmt.Errorf("expand: %q is not an expansion formula (type=%s)", rule.With, expFormula.Type)
		}

		vars := applyVarDefaults(rule.Vars, expFormula.Vars)
		for k, v := range parentVars {
			vars[k] = v
		}

		expandedSteps, err := expandStep(targetStep, expFormula.Template, 0, vars)
		if err != nil {
			return nil, fmt.Errorf("expand %q: %w", rule.Target, err)
		}

		propagateTargetDeps(targetStep, expandedSteps)

		result = replaceStep(result, rule.Target, expandedSteps)
		expanded[rule.Target] = true

		if len(expandedSteps) > 0 {
			lastExpanded := expandedSteps[len(expandedSteps)-1]
			result = updateDownstreamDeps(result, rule.Target, lastExpanded.ID)
		}
	}

	for _, rule := range compose.Map {
		matchedIDs := matchStepsByGlob(result, rule.Select)
		for _, targetID := range matchedIDs {
			if expanded[targetID] {
				continue
			}

			targetStep := findStepInList(result, targetID)
			if targetStep == nil {
				continue
			}

			expFormula, err := parser.LoadByName(rule.With)
			if err != nil {
				return nil, fmt.Errorf("map: loading formula %q: %w", rule.With, err)
			}

			if expFormula.Type != spec.TypeExpansion {
				return nil, fmt.Errorf("map: %q is not an expansion formula (type=%s)", rule.With, expFormula.Type)
			}

			vars := applyVarDefaults(rule.Vars, expFormula.Vars)
			for k, v := range parentVars {
				vars[k] = v
			}

			expandedSteps, err := expandStep(targetStep, expFormula.Template, 0, vars)
			if err != nil {
				return nil, fmt.Errorf("map %q: %w", targetID, err)
			}

			propagateTargetDeps(targetStep, expandedSteps)

			result = replaceStep(result, targetID, expandedSteps)
			expanded[targetID] = true

			if len(expandedSteps) > 0 {
				lastExpanded := expandedSteps[len(expandedSteps)-1]
				result = updateDownstreamDeps(result, targetID, lastExpanded.ID)
			}
		}
	}

	return result, nil
}

func ApplyExpansions(steps []*spec.Step, compose *spec.ComposeRules, parser *Parser) ([]*spec.Step, error) {
	return ApplyExpansionsWithVars(steps, compose, parser, nil)
}

func ApplyInlineExpansionsWithVars(steps []*spec.Step, parser *Parser, vars map[string]string) ([]*spec.Step, error) {
	if parser == nil {
		return steps, nil
	}

	result := make([]*spec.Step, 0, len(steps))
	for _, step := range steps {
		if step.Expand == "" {
			clone := cloneStep(step)
			if len(step.Children) > 0 {
				children, err := ApplyInlineExpansionsWithVars(step.Children, parser, mergeVars(clone.ExpandVars, vars))
				if err != nil {
					return nil, err
				}
				clone.Children = children
			}
			result = append(result, clone)
			continue
		}

		expFormula, err := parser.LoadByName(step.Expand)
		if err != nil {
			return nil, fmt.Errorf("inline expand %q: %w", step.Expand, err)
		}

		if expFormula.Type != spec.TypeExpansion {
			return nil, fmt.Errorf("inline expand: %q is not an expansion formula", step.Expand)
		}

		expandedVars := mergeVars(step.ExpandVars, vars)
		expandedSteps, err := expandStep(step, expFormula.Template, 0, expandedVars)
		if err != nil {
			return nil, fmt.Errorf("inline expand %q: %w", step.ID, err)
		}

		result = append(result, expandedSteps...)
	}

	return result, nil
}

func ApplyInlineExpansions(steps []*spec.Step, parser *Parser) ([]*spec.Step, error) {
	return ApplyInlineExpansionsWithVars(steps, parser, nil)
}

func expandStep(target *spec.Step, template []*spec.Step, depth int, vars map[string]string) ([]*spec.Step, error) {
	if depth > DefaultMaxExpansionDepth {
		return nil, fmt.Errorf("max expansion depth (%d) exceeded", DefaultMaxExpansionDepth)
	}

	result := make([]*spec.Step, 0, len(template))
	for _, t := range template {
		expanded := expandTemplateStep(t, target, vars)

		if len(t.Children) > 0 {
			children, err := expandStep(target, t.Children, depth+1, vars)
			if err != nil {
				return nil, err
			}
			expanded.Children = children
		}

		result = append(result, expanded)
	}

	return result, nil
}

func expandTemplateStep(t *spec.Step, target *spec.Step, vars map[string]string) *spec.Step {
	clone := cloneStep(t)

	clone.ID = substituteTemplateRef(t.ID, target)
	clone.Title = substituteTemplateVars(t.Title, target, vars)
	clone.Description = substituteTemplateVars(t.Description, target, vars)
	clone.Notes = substituteTemplateVars(t.Notes, target, vars)
	clone.Assignee = substituteTemplateVars(t.Assignee, target, vars)
	clone.Timeout = substituteTemplateVars(t.Timeout, target, vars)

	clone.DependsOn = substituteDeps(t.DependsOn, target, vars)
	clone.Needs = substituteDeps(t.Needs, target, vars)

	if clone.Metadata != nil {
		newMeta := make(map[string]string, len(clone.Metadata))
		for k, v := range clone.Metadata {
			newMeta[k] = substituteTemplateVars(v, target, vars)
		}
		clone.Metadata = newMeta
	}

	if clone.Labels != nil {
		newLabels := make([]string, len(clone.Labels))
		for i, l := range clone.Labels {
			newLabels[i] = substituteTemplateVars(l, target, vars)
		}
		clone.Labels = newLabels
	}

	return clone
}

func substituteTemplateRef(s string, target *spec.Step) string {
	s = strings.ReplaceAll(s, "{target}", target.ID)
	s = strings.ReplaceAll(s, "{target.id}", target.ID)
	s = strings.ReplaceAll(s, "{target.title}", target.Title)
	return s
}

func substituteTemplateVars(s string, target *spec.Step, vars map[string]string) string {
	s = substituteTemplateRef(s, target)
	s = spec.Substitute(s, vars)
	return s
}

func substituteDeps(deps []string, target *spec.Step, vars map[string]string) []string {
	if len(deps) == 0 {
		return nil
	}
	result := make([]string, len(deps))
	for i, dep := range deps {
		result[i] = substituteTemplateVars(dep, target, vars)
	}
	return result
}

func propagateTargetDeps(target *spec.Step, expanded []*spec.Step) {
	if len(expanded) == 0 {
		return
	}

	rootStep := expanded[0]
	if rootStep.Needs == nil {
		rootStep.Needs = make([]string, 0)
	}

	for _, dep := range target.DependsOn {
		rootStep.Needs = appendUnique(rootStep.Needs, dep)
	}
	for _, need := range target.Needs {
		rootStep.Needs = appendUnique(rootStep.Needs, need)
	}
}

func replaceStep(steps []*spec.Step, targetID string, replacement []*spec.Step) []*spec.Step {
	result := make([]*spec.Step, 0, len(steps)+len(replacement))
	for _, s := range steps {
		if s.ID == targetID {
			result = append(result, replacement...)
		} else {
			result = append(result, s)
		}
	}
	return result
}

func updateDownstreamDeps(steps []*spec.Step, oldID, newID string) []*spec.Step {
	for _, s := range steps {
		for i, dep := range s.DependsOn {
			if dep == oldID {
				s.DependsOn[i] = newID
			}
		}
		for i, need := range s.Needs {
			if need == oldID {
				s.Needs[i] = newID
			}
		}
	}
	return steps
}

func matchStepsByGlob(steps []*spec.Step, pattern string) []string {
	var matched []string
	for _, s := range steps {
		if MatchGlob(pattern, s.ID) {
			matched = append(matched, s.ID)
		}
	}
	return matched
}

func findStepInList(steps []*spec.Step, id string) *spec.Step {
	for _, s := range steps {
		if s.ID == id {
			return s
		}
	}
	return nil
}

func applyVarDefaults(overrides map[string]string, varDefs map[string]*spec.VarDef) map[string]string {
	result := make(map[string]string)
	for name, def := range varDefs {
		if def == nil {
			continue
		}
		if val, ok := overrides[name]; ok {
			result[name] = val
		} else if def.Default != nil {
			result[name] = *def.Default
		}
	}
	return result
}

func mergeVars(overrides, defaults map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range defaults {
		result[k] = v
	}
	for k, v := range overrides {
		result[k] = v
	}
	return result
}

func resolveOverrideVars(overrides, parentVars map[string]string) map[string]string {
	if len(overrides) == 0 {
		return parentVars
	}
	result := make(map[string]string)
	for k, v := range parentVars {
		result[k] = v
	}
	for k, v := range overrides {
		result[k] = spec.Substitute(v, parentVars)
	}
	return result
}

func mergeConditionVars(parentVars, vars map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range parentVars {
		result[k] = v
	}
	for k, v := range vars {
		result[k] = v
	}
	return result
}

func validateExpandedStepTimeouts(steps []*spec.Step, context string) error {
	for _, step := range steps {
		if step.Timeout != "" {
			if _, err := ParseCondition(step.Timeout); err != nil {
				_ = err
			}
		}
	}
	return nil
}

func materializeExpandedStepConditions(steps []*spec.Step, vars map[string]string) ([]*spec.Step, error) {
	return FilterStepsByCondition(steps, vars)
}
