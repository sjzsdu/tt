package formula

import (
	"path/filepath"
	"strings"
)

func MatchGlob(pattern, stepID string) bool {
	matched, err := filepath.Match(pattern, stepID)
	if err == nil && matched {
		return true
	}

	if pattern == "*" {
		return true
	}

	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:]
		return strings.HasSuffix(stepID, suffix)
	}

	if strings.HasSuffix(pattern, ".*") {
		prefix := pattern[:len(pattern)-1]
		return strings.HasPrefix(stepID, prefix)
	}

	return pattern == stepID
}

func ApplyAdvice(steps []*Step, advice []*AdviceRule) []*Step {
	if len(advice) == 0 {
		return steps
	}

	originalIDs := collectStepIDs(steps)
	return applyAdviceWithGuard(steps, advice, originalIDs)
}

func applyAdviceWithGuard(steps []*Step, advice []*AdviceRule, originalIDs map[string]bool) []*Step {
	result := make([]*Step, 0, len(steps)*2)

	for _, step := range steps {
		if !originalIDs[step.ID] {
			result = append(result, step)
			continue
		}

		var beforeSteps []*Step
		var afterSteps []*Step

		for _, rule := range advice {
			if !MatchGlob(rule.Target, step.ID) {
				continue
			}

			if rule.Before != nil {
				beforeSteps = append(beforeSteps, adviceStepToStep(rule.Before, step))
			}
			if rule.Around != nil {
				for _, as := range rule.Around.Before {
					beforeSteps = append(beforeSteps, adviceStepToStep(as, step))
				}
			}

			if rule.After != nil {
				afterSteps = append(afterSteps, adviceStepToStep(rule.After, step))
			}
			if rule.Around != nil {
				for _, as := range rule.Around.After {
					afterSteps = append(afterSteps, adviceStepToStep(as, step))
				}
			}
		}

		result = append(result, beforeSteps...)

		clonedStep := cloneStep(step)

		if len(beforeSteps) > 0 {
			lastBefore := beforeSteps[len(beforeSteps)-1]
			clonedStep.Needs = appendUnique(clonedStep.Needs, lastBefore.ID)
		}

		for i := 1; i < len(beforeSteps); i++ {
			beforeSteps[i].Needs = appendUnique(beforeSteps[i].Needs, beforeSteps[i-1].ID)
		}

		result = append(result, clonedStep)

		for i, as := range afterSteps {
			if i == 0 {
				as.Needs = appendUnique(as.Needs, step.ID)
			} else {
				as.Needs = appendUnique(as.Needs, afterSteps[i-1].ID)
			}
			result = append(result, as)
		}

		if len(step.Children) > 0 {
			clonedStep.Children = ApplyAdvice(step.Children, advice)
		}
	}

	return result
}

func adviceStepToStep(as *AdviceStep, target *Step) *Step {
	id := substituteStepRef(as.ID, target)
	title := substituteStepRef(as.Title, target)
	if title == "" {
		title = id
	}
	desc := substituteStepRef(as.Description, target)

	return &Step{
		ID:             id,
		Title:          title,
		Description:    desc,
		Type:           as.Type,
		SourceFormula:  target.SourceFormula,
		SourceLocation: "advice",
	}
}

func substituteStepRef(s string, target *Step) string {
	s = strings.ReplaceAll(s, "{step.id}", target.ID)
	s = strings.ReplaceAll(s, "{step.title}", target.Title)
	return s
}

func collectStepIDs(steps []*Step) map[string]bool {
	ids := make(map[string]bool)
	var collect func([]*Step)
	collect = func(s []*Step) {
		for _, step := range s {
			ids[step.ID] = true
			if len(step.Children) > 0 {
				collect(step.Children)
			}
		}
	}
	collect(steps)
	return ids
}

func MatchPointcut(pc *Pointcut, step *Step) bool {
	if pc.Glob != "" && !MatchGlob(pc.Glob, step.ID) {
		return false
	}
	if pc.Type != "" && step.Type != pc.Type {
		return false
	}
	if pc.Label != "" {
		found := false
		for _, l := range step.Labels {
			if l == pc.Label {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func MatchAnyPointcut(pointcuts []*Pointcut, step *Step) bool {
	if len(pointcuts) == 0 {
		return true
	}
	for _, pc := range pointcuts {
		if MatchPointcut(pc, step) {
			return true
		}
	}
	return false
}
