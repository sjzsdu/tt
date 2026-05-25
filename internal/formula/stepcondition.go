package formula

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	stepCondVarPattern        = regexp.MustCompile(`^\{\{(\w+)\}\}$`)
	stepCondNegatedVarPattern = regexp.MustCompile(`^!\{\{(\w+)\}\}$`)
	stepCondComparePattern    = regexp.MustCompile(`^\{\{(\w+)\}\}\s*(==|!=)\s*(.+)$`)
	stepCondRuntimePattern    = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_-]*(?:\.[a-zA-Z_][a-zA-Z0-9_-]*)*)\s*(==|!=|=~)\s*(.+)$`)
)

func EvaluateStepCondition(condition string, vars map[string]string) (bool, error) {
	condition = strings.TrimSpace(condition)

	if condition == "" {
		return true, nil
	}

	if m := stepCondVarPattern.FindStringSubmatch(condition); m != nil {
		varName := m[1]
		value := vars[varName]
		return isTruthy(value), nil
	}

	if m := stepCondNegatedVarPattern.FindStringSubmatch(condition); m != nil {
		varName := m[1]
		value := vars[varName]
		return !isTruthy(value), nil
	}

	if m := stepCondComparePattern.FindStringSubmatch(condition); m != nil {
		varName := m[1]
		operator := m[2]
		expected := strings.TrimSpace(m[3])
		expected = unquoteValue(expected)

		actual := vars[varName]

		switch operator {
		case "==":
			return actual == expected, nil
		case "!=":
			return actual != expected, nil
		}
	}

	if m := stepCondRuntimePattern.FindStringSubmatch(condition); m != nil {
		return true, nil
	}

	return false, fmt.Errorf("invalid step condition format: %q", condition)
}

func isTruthy(value string) bool {
	if value == "" {
		return false
	}
	lower := strings.ToLower(value)
	switch lower {
	case "false", "0", "no", "off":
		return false
	}
	return true
}

func unquoteValue(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func FilterStepsByCondition(steps []*Step, vars map[string]string) ([]*Step, error) {
	if vars == nil {
		vars = make(map[string]string)
	}

	result := make([]*Step, 0, len(steps))

	for _, step := range steps {
		include, err := EvaluateStepCondition(step.Condition, vars)
		if err != nil {
			return nil, fmt.Errorf("step %q: %w", step.ID, err)
		}

		if !include {
			continue
		}

		clone := cloneStep(step)

		if len(step.Children) > 0 {
			filteredChildren, err := FilterStepsByCondition(step.Children, vars)
			if err != nil {
				return nil, err
			}
			clone.Children = filteredChildren
		}

		result = append(result, clone)
	}

	return result, nil
}
