package formula

import (
	"encoding/json"
	"fmt"
	"strings"
)

func ApplyControlFlowWithVars(steps []*Step, compose *ComposeRules, vars map[string]string) ([]*Step, error) {
	var err error

	steps, err = ApplyLoopsWithVars(steps, vars)
	if err != nil {
		return nil, fmt.Errorf("applying loops: %w", err)
	}

	stepMap := buildStepMap(steps)

	if err := applyBranchesWithMap(stepMap, compose); err != nil {
		return nil, fmt.Errorf("applying branches: %w", err)
	}

	if err := applyGatesWithMap(stepMap, compose); err != nil {
		return nil, fmt.Errorf("applying gates: %w", err)
	}

	return steps, nil
}

func ApplyLoopsWithVars(steps []*Step, vars map[string]string) ([]*Step, error) {
	result := make([]*Step, 0, len(steps))

	for _, step := range steps {
		if step.Loop == nil {
			clone := cloneStep(step)
			if len(step.Children) > 0 {
				children, err := ApplyLoopsWithVars(step.Children, vars)
				if err != nil {
					return nil, err
				}
				clone.Children = children
			}
			result = append(result, clone)
			continue
		}

		if err := validateLoopSpec(step.Loop, step.ID); err != nil {
			return nil, err
		}

		if step.Loop.Until != "" || step.Loop.ForEach != "" {
			clone := cloneStep(step)
			result = append(result, clone)
			continue
		}

		expanded, err := expandLoopWithVars(step, vars)
		if err != nil {
			return nil, err
		}
		result = append(result, expanded...)
	}

	return result, nil
}

func validateLoopSpec(loop *LoopSpec, stepID string) error {
	if len(loop.Body) == 0 {
		return fmt.Errorf("loop %q: body is required", stepID)
	}

	loopTypes := 0
	if loop.Count > 0 {
		loopTypes++
	}
	if loop.Until != "" {
		loopTypes++
	}
	if loop.Range != "" {
		loopTypes++
	}
	if loop.ForEach != "" {
		loopTypes++
	}

	if loopTypes == 0 {
		return fmt.Errorf("loop %q: one of count, until, range, or for_each is required", stepID)
	}
	if loopTypes > 1 {
		return fmt.Errorf("loop %q: only one of count, until, range, or for_each can be specified", stepID)
	}

	if loop.Until != "" && loop.Max == 0 {
		return fmt.Errorf("loop %q: max is required when until is set", stepID)
	}

	if loop.Count < 0 {
		return fmt.Errorf("loop %q: count must be positive", stepID)
	}

	if loop.Max < 0 {
		return fmt.Errorf("loop %q: max must be positive", stepID)
	}
	if loop.MaxConcurrency < 0 {
		return fmt.Errorf("loop %q: max_concurrency must be positive", stepID)
	}
	if loop.ForEach != "" && strings.TrimSpace(loop.Var) == "" {
		return fmt.Errorf("loop %q: var is required when for_each is set", stepID)
	}

	if loop.Until != "" {
		if _, err := ParseCondition(loop.Until); err != nil {
			return fmt.Errorf("loop %q: invalid until condition %q: %w", stepID, loop.Until, err)
		}
	}

	if loop.Range != "" {
		if err := ValidateRange(loop.Range); err != nil {
			return fmt.Errorf("loop %q: invalid range %q: %w", stepID, loop.Range, err)
		}
	}

	return nil
}

func expandLoopWithVars(step *Step, vars map[string]string) ([]*Step, error) {
	var result []*Step

	switch {
	case step.Loop.Count > 0:
		for i := 1; i <= step.Loop.Count; i++ {
			iterSteps, err := expandLoopIteration(step, i, nil)
			if err != nil {
				return nil, err
			}
			result = append(result, iterSteps...)
		}

		var err error
		result, err = ApplyLoopsWithVars(result, vars)
		if err != nil {
			return nil, err
		}

		if step.Loop.Count > 1 {
			result = chainExpandedIterations(result, step.ID, step.Loop.Count)
		}

	case step.Loop.Range != "":
		rangeSpec, err := ParseRange(step.Loop.Range, vars)
		if err != nil {
			return nil, fmt.Errorf("loop %q: %w", step.ID, err)
		}

		if rangeSpec.End < rangeSpec.Start {
			return nil, fmt.Errorf("loop %q: range end (%d) is less than start (%d)",
				step.ID, rangeSpec.End, rangeSpec.Start)
		}

		count := rangeSpec.End - rangeSpec.Start + 1
		iterNum := 0
		for val := rangeSpec.Start; val <= rangeSpec.End; val++ {
			iterNum++
			iterVars := make(map[string]string)
			if step.Loop.Var != "" {
				iterVars[step.Loop.Var] = fmt.Sprintf("%d", val)
			}
			iterSteps, err := expandLoopIteration(step, iterNum, iterVars)
			if err != nil {
				return nil, err
			}
			result = append(result, iterSteps...)
		}

		result, err = ApplyLoopsWithVars(result, vars)
		if err != nil {
			return nil, err
		}

		if count > 1 {
			result = chainExpandedIterations(result, step.ID, count)
		}

	default:
		iterSteps, err := expandLoopIteration(step, 1, nil)
		if err != nil {
			return nil, err
		}

		if len(iterSteps) > 0 {
			firstStep := iterSteps[0]
			loopMeta := map[string]interface{}{
				"until": step.Loop.Until,
				"max":   step.Loop.Max,
			}
			loopJSON, _ := json.Marshal(loopMeta)
			firstStep.Labels = append(firstStep.Labels, fmt.Sprintf("loop:%s", string(loopJSON)))
		}

		result, err = ApplyLoops(iterSteps)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

func ApplyLoops(steps []*Step) ([]*Step, error) {
	return ApplyLoopsWithVars(steps, nil)
}

func expandLoopIteration(step *Step, iteration int, iterVars map[string]string) ([]*Step, error) {
	result := make([]*Step, 0, len(step.Loop.Body))

	bodyStepIDs := collectBodyStepIDs(step.Loop.Body)

	for _, bodyStep := range step.Loop.Body {
		iterID := fmt.Sprintf("%s.iter%d.%s", step.ID, iteration, bodyStep.ID)

		title := substituteLoopVars(bodyStep.Title, iterVars)
		description := substituteLoopVars(bodyStep.Description, iterVars)

		clone := cloneStep(bodyStep)
		clone.ID = iterID
		clone.Title = title
		clone.Description = description
		clone.Timeout = substituteLoopVars(bodyStep.Timeout, iterVars)
		clone.SourceLocation = fmt.Sprintf("%s.iter%d", bodyStep.SourceLocation, iteration)

		if len(iterVars) > 0 {
			if clone.ExpandVars == nil {
				clone.ExpandVars = make(map[string]string)
			}
			for k, v := range iterVars {
				clone.ExpandVars[k] = v
			}
		}

		clone.DependsOn = rewriteLoopDependencies(bodyStep.DependsOn, step.ID, iteration, bodyStepIDs)
		clone.Needs = rewriteLoopDependencies(bodyStep.Needs, step.ID, iteration, bodyStepIDs)

		if len(bodyStep.Children) > 0 {
			clone.Children = expandLoopChildren(bodyStep.Children, step.ID, iteration, bodyStepIDs, iterVars)
		}
		substituteLoopVarsInTimeouts(clone, iterVars)

		result = append(result, clone)
	}

	return result, nil
}

func substituteLoopVars(s string, vars map[string]string) string {
	if vars == nil || s == "" {
		return s
	}
	for k, v := range vars {
		s = strings.ReplaceAll(s, "{"+k+"}", v)
	}
	return s
}

func collectBodyStepIDs(body []*Step) map[string]bool {
	ids := make(map[string]bool)
	var collect func([]*Step)
	collect = func(steps []*Step) {
		for _, s := range steps {
			ids[s.ID] = true
			if len(s.Children) > 0 {
				collect(s.Children)
			}
		}
	}
	collect(body)
	return ids
}

func rewriteLoopDependencies(deps []string, loopID string, iteration int, bodyStepIDs map[string]bool) []string {
	if len(deps) == 0 {
		return nil
	}

	result := make([]string, len(deps))
	for i, dep := range deps {
		if bodyStepIDs[dep] {
			result[i] = fmt.Sprintf("%s.iter%d.%s", loopID, iteration, dep)
		} else {
			result[i] = dep
		}
	}
	return result
}

func expandLoopChildren(children []*Step, loopID string, iteration int, bodyStepIDs map[string]bool, iterVars map[string]string) []*Step {
	result := make([]*Step, len(children))
	for i, child := range children {
		clone := cloneStepDeep(child)
		clone.ID = fmt.Sprintf("%s.iter%d.%s", loopID, iteration, child.ID)
		clone.Timeout = substituteLoopVars(child.Timeout, iterVars)
		clone.DependsOn = rewriteLoopDependencies(child.DependsOn, loopID, iteration, bodyStepIDs)
		clone.Needs = rewriteLoopDependencies(child.Needs, loopID, iteration, bodyStepIDs)

		if len(child.Children) > 0 {
			clone.Children = expandLoopChildren(child.Children, loopID, iteration, bodyStepIDs, iterVars)
		}
		substituteLoopVarsInTimeouts(clone, iterVars)

		result[i] = clone
	}
	return result
}

func substituteLoopVarsInTimeouts(step *Step, iterVars map[string]string) {
	if step == nil {
		return
	}
	step.Timeout = substituteLoopVars(step.Timeout, iterVars)
	for _, child := range step.Children {
		substituteLoopVarsInTimeouts(child, iterVars)
	}
	if step.Loop != nil {
		nestedVars := loopBodyTimeoutVars(iterVars, step.Loop)
		for _, bodyStep := range step.Loop.Body {
			substituteLoopVarsInTimeouts(bodyStep, nestedVars)
		}
	}
}

func loopBodyTimeoutVars(iterVars map[string]string, loop *LoopSpec) map[string]string {
	if len(iterVars) == 0 || loop == nil || loop.Var == "" {
		return iterVars
	}
	if _, shadows := iterVars[loop.Var]; !shadows {
		return iterVars
	}
	if len(iterVars) == 1 {
		return nil
	}
	nestedVars := make(map[string]string, len(iterVars)-1)
	for k, v := range iterVars {
		if k != loop.Var {
			nestedVars[k] = v
		}
	}
	return nestedVars
}

func chainExpandedIterations(steps []*Step, loopID string, count int) []*Step {
	if len(steps) == 0 || count < 2 {
		return steps
	}

	iterFirstIdx := make(map[int]int)
	iterLastIdx := make(map[int]int)

	for i, s := range steps {
		for iter := 1; iter <= count; iter++ {
			prefix := fmt.Sprintf("%s.iter%d.", loopID, iter)
			if strings.HasPrefix(s.ID, prefix) {
				if _, found := iterFirstIdx[iter]; !found {
					iterFirstIdx[iter] = i
				}
				iterLastIdx[iter] = i
				break
			}
		}
	}

	for iter := 2; iter <= count; iter++ {
		firstIdx, hasFirst := iterFirstIdx[iter]
		prevLastIdx, hasPrevLast := iterLastIdx[iter-1]

		if hasFirst && hasPrevLast {
			lastStepID := steps[prevLastIdx].ID
			steps[firstIdx].Needs = appendUnique(steps[firstIdx].Needs, lastStepID)
		}
	}

	return steps
}

func ApplyBranches(steps []*Step, compose *ComposeRules) ([]*Step, error) {
	if compose == nil || len(compose.Branch) == 0 {
		return steps, nil
	}

	cloned := cloneStepsRecursive(steps)
	stepMap := buildStepMap(cloned)

	if err := applyBranchesWithMap(stepMap, compose); err != nil {
		return nil, err
	}

	return cloned, nil
}

func applyBranchesWithMap(stepMap map[string]*Step, compose *ComposeRules) error {
	if compose == nil || len(compose.Branch) == 0 {
		return nil
	}

	for _, branch := range compose.Branch {
		if branch.From == "" {
			return fmt.Errorf("branch: from is required")
		}
		if len(branch.Steps) == 0 {
			return fmt.Errorf("branch: steps is required")
		}
		if branch.Join == "" {
			return fmt.Errorf("branch: join is required")
		}

		if _, ok := stepMap[branch.From]; !ok {
			return fmt.Errorf("branch: from step %q not found", branch.From)
		}
		if _, ok := stepMap[branch.Join]; !ok {
			return fmt.Errorf("branch: join step %q not found", branch.Join)
		}
		for _, stepID := range branch.Steps {
			if _, ok := stepMap[stepID]; !ok {
				return fmt.Errorf("branch: parallel step %q not found", stepID)
			}
		}

		for _, stepID := range branch.Steps {
			step := stepMap[stepID]
			step.Needs = appendUnique(step.Needs, branch.From)
		}

		joinStep := stepMap[branch.Join]
		for _, stepID := range branch.Steps {
			joinStep.Needs = appendUnique(joinStep.Needs, stepID)
		}
	}

	return nil
}

func ApplyGates(steps []*Step, compose *ComposeRules) ([]*Step, error) {
	if compose == nil || len(compose.Gate) == 0 {
		return steps, nil
	}

	cloned := cloneStepsRecursive(steps)
	stepMap := buildStepMap(cloned)

	if err := applyGatesWithMap(stepMap, compose); err != nil {
		return nil, err
	}

	return cloned, nil
}

func applyGatesWithMap(stepMap map[string]*Step, compose *ComposeRules) error {
	if compose == nil || len(compose.Gate) == 0 {
		return nil
	}

	for _, gate := range compose.Gate {
		if gate.Before == "" {
			return fmt.Errorf("gate: before is required")
		}
		if gate.Condition == "" {
			return fmt.Errorf("gate: condition is required")
		}

		if _, err := ParseCondition(gate.Condition); err != nil {
			return fmt.Errorf("gate: invalid condition %q: %w", gate.Condition, err)
		}

		step, ok := stepMap[gate.Before]
		if !ok {
			return fmt.Errorf("gate: target step %q not found", gate.Before)
		}

		gateMeta := map[string]string{"condition": gate.Condition}
		gateJSON, _ := json.Marshal(gateMeta)
		gateLabel := fmt.Sprintf("gate:%s", string(gateJSON))
		step.Labels = appendUnique(step.Labels, gateLabel)
	}

	return nil
}

func buildStepMap(steps []*Step) map[string]*Step {
	result := make(map[string]*Step)
	var walk func([]*Step)
	walk = func(s []*Step) {
		for _, step := range s {
			result[step.ID] = step
			if len(step.Children) > 0 {
				walk(step.Children)
			}
		}
	}
	walk(steps)
	return result
}

func cloneStep(s *Step) *Step {
	if s == nil {
		return nil
	}
	clone := *s
	if s.Labels != nil {
		clone.Labels = append([]string(nil), s.Labels...)
	}
	if s.DependsOn != nil {
		clone.DependsOn = append([]string(nil), s.DependsOn...)
	}
	if s.Needs != nil {
		clone.Needs = append([]string(nil), s.Needs...)
	}
	if s.Metadata != nil {
		m := make(map[string]string, len(s.Metadata))
		for k, v := range s.Metadata {
			m[k] = v
		}
		clone.Metadata = m
	}
	if s.ExpandVars != nil {
		v := make(map[string]string, len(s.ExpandVars))
		for k, val := range s.ExpandVars {
			v[k] = val
		}
		clone.ExpandVars = v
	}
	if s.EmbedVars != nil {
		v := make(map[string]string, len(s.EmbedVars))
		for k, val := range s.EmbedVars {
			v[k] = val
		}
		clone.EmbedVars = v
	}
	if s.Agent != nil {
		agentClone := *s.Agent
		clone.Agent = &agentClone
	}
	if s.InputCtx != nil {
		clone.InputCtx = append([]string(nil), s.InputCtx...)
	}
	return &clone
}

func cloneStepDeep(s *Step) *Step {
	clone := cloneStep(s)
	if len(s.Children) > 0 {
		clone.Children = make([]*Step, len(s.Children))
		for i, child := range s.Children {
			clone.Children[i] = cloneStepDeep(child)
		}
	}
	return clone
}

func cloneStepsRecursive(steps []*Step) []*Step {
	result := make([]*Step, len(steps))
	for i, step := range steps {
		result[i] = cloneStepDeep(step)
	}
	return result
}

func appendUnique(slice []string, items ...string) []string {
	for _, item := range items {
		found := false
		for _, existing := range slice {
			if existing == item {
				found = true
				break
			}
		}
		if !found {
			slice = append(slice, item)
		}
	}
	return slice
}
