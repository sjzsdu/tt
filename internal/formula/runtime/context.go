package runtime

import (
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
	value, ok := s.values[normalizePath(path)]
	return value, ok
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
