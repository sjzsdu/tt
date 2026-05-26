package runtime

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/sjzsdu/tt/internal/formula/steps"
)

type ContextStore struct {
	mu     sync.RWMutex
	values map[string]steps.Value
}

func NewContextStore() *ContextStore { return &ContextStore{values: map[string]steps.Value{}} }

func (s *ContextStore) Get(path string) (steps.Value, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	path = normalizePath(path)
	if value, ok := s.values[path]; ok {
		return value, true
	}
	root, fieldPath, ok := strings.Cut(path, ".")
	if !ok {
		return steps.Value{}, false
	}
	value, ok := s.values[root]
	if !ok {
		return steps.Value{}, false
	}
	return lookupValuePath(value, fieldPath)
}

func (s *ContextStore) Set(path string, value steps.Value) error {
	path = normalizePath(path)
	if path == "" {
		return fmt.Errorf("context path is required")
	}
	s.mu.Lock()
	s.values[path] = value
	s.mu.Unlock()
	return nil
}

func (s *ContextStore) Snapshot() map[string]steps.Value {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]steps.Value, len(s.values))
	for k, v := range s.values {
		out[k] = v
	}
	return out
}

func normalizePath(path string) string { return strings.Trim(strings.TrimSpace(path), ".") }

func lookupValuePath(value steps.Value, path string) (steps.Value, bool) {
	var data any
	if err := json.Unmarshal(value.Raw, &data); err != nil {
		return steps.Value{}, false
	}
	current := data
	for _, part := range strings.Split(path, ".") {
		if part == "" {
			return steps.Value{}, false
		}
		object, ok := current.(map[string]any)
		if !ok {
			return steps.Value{}, false
		}
		current, ok = object[part]
		if !ok {
			return steps.Value{}, false
		}
	}
	raw, err := json.Marshal(current)
	if err != nil {
		return steps.Value{}, false
	}
	return steps.Value{Type: "json", Raw: raw}, true
}
