package runtime

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/sjzsdu/tt/internal/formula/steps"
)

var runtimeConditionPattern = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_-]*(?:\.[a-zA-Z_][a-zA-Z0-9_-]*)*)\s*(==|!=|=~)\s*(.+)$`)

func shouldRunStep(condition string, context *ContextStore) (bool, error) {
	condition = strings.TrimSpace(condition)
	if condition == "" {
		return true, nil
	}
	m := runtimeConditionPattern.FindStringSubmatch(condition)
	if m == nil {
		return false, fmt.Errorf("invalid runtime condition %q", condition)
	}
	actual, ok := lookupConditionValue(context, m[1])
	if !ok {
		actual = ""
	}
	expected := unquoteConditionValue(strings.TrimSpace(m[3]))
	switch m[2] {
	case "==":
		return actual == expected, nil
	case "!=":
		return actual != expected, nil
	case "=~":
		matched, err := regexp.MatchString(expected, actual)
		if err != nil {
			return false, err
		}
		return matched, nil
	default:
		return false, fmt.Errorf("unsupported runtime condition operator %q", m[2])
	}
}

func lookupConditionValue(context *ContextStore, path string) (string, bool) {
	if context == nil {
		return "", false
	}
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return "", false
	}
	value, ok := context.Get(parts[0])
	if !ok {
		return "", false
	}
	if len(parts) == 1 {
		return conditionValueString(value), true
	}
	var data any
	if err := json.Unmarshal(value.Raw, &data); err != nil {
		return "", false
	}
	current := data
	for _, part := range parts[1:] {
		object, ok := current.(map[string]any)
		if !ok {
			return "", false
		}
		current, ok = object[part]
		if !ok {
			return "", false
		}
	}
	return scalarConditionString(current), true
}

func conditionValueString(value steps.Value) string {
	var data any
	if err := json.Unmarshal(value.Raw, &data); err == nil {
		return scalarConditionString(data)
	}
	return strings.TrimSpace(string(value.Raw))
}

func scalarConditionString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		if v {
			return "true"
		}
		return "false"
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", v), "0"), ".")
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(data)
	}
}

func unquoteConditionValue(value string) string {
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}
