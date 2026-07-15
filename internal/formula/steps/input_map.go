package steps

import (
	"encoding/json"
	"strconv"
	"strings"
)

// InputMap is the stable input contract shared by all step kinds.
// Get supports both exact named inputs and nested JSON paths.
type InputMap map[string]Value

func (m InputMap) Get(path string) (Value, bool) {
	path = strings.Trim(strings.TrimSpace(path), ".")
	if value, ok := m[path]; ok {
		return value, true
	}
	best := ""
	for key := range m {
		if len(key) > len(best) && strings.HasPrefix(path, key+".") {
			best = key
		}
	}
	if best == "" {
		return Value{}, false
	}
	return inputMapNestedValue(m[best], strings.TrimPrefix(path, best+"."))
}

func inputMapNestedValue(value Value, path string) (Value, bool) {
	var current any
	if err := json.Unmarshal(value.Raw, &current); err != nil {
		return Value{}, false
	}
	for _, part := range strings.Split(path, ".") {
		switch node := current.(type) {
		case map[string]any:
			var ok bool
			current, ok = node[part]
			if !ok {
				return Value{}, false
			}
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(node) {
				return Value{}, false
			}
			current = node[index]
		default:
			return Value{}, false
		}
	}
	raw, err := json.Marshal(current)
	if err != nil {
		return Value{}, false
	}
	return Value{Type: "json", Raw: raw}, true
}

func (r RunRequest) InputView() ContextView {
	if r.Inputs != nil {
		return r.Inputs
	}
	return r.Context
}
