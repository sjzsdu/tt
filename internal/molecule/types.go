// Package molecule instantiates compiled formula recipes as structured task trees.
package molecule

type Options struct {
	Title          string
	Vars           map[string]string
	ParentID       string
	IdempotencyKey string
	PriorityOverride *int
}

type Result struct {
	RootID    string
	IDMapping map[string]string
	Created   int
	Tasks     []TaskItem
}

type TaskItem struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description,omitempty"`
	Notes       string            `json:"notes,omitempty"`
	Type        string            `json:"type"`
	Priority    *int              `json:"priority,omitempty"`
	Labels      []string          `json:"labels,omitempty"`
	Assignee    string            `json:"assignee,omitempty"`
	ParentID    string            `json:"parent_id,omitempty"`
	Dependencies []string         `json:"dependencies,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	IsRoot      bool              `json:"is_root"`
}
