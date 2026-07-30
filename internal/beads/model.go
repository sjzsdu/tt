// Package beads provides a Go adapter around the bd (beads) CLI.
// It normalizes CLI JSON into tt-owned DTOs and never depends on private Dolt schemas.
package beads

import "time"

// Issue is the normalized issue DTO returned by the adapter.
type Issue struct {
	ID                 string       `json:"id"`
	Title              string       `json:"title"`
	Description        string       `json:"description,omitempty"`
	Design             string       `json:"design,omitempty"`
	AcceptanceCriteria string       `json:"acceptance_criteria,omitempty"`
	Status             string       `json:"status"`
	Priority           int          `json:"priority"`
	IssueType          string       `json:"issue_type,omitempty"`
	Owner              string       `json:"owner,omitempty"`
	Assignee           string       `json:"assignee,omitempty"`
	Labels             []string     `json:"labels,omitempty"`
	EstimatedMinutes   int          `json:"estimated_minutes,omitempty"`
	CreatedAt          time.Time    `json:"created_at"`
	CreatedBy          string       `json:"created_by,omitempty"`
	UpdatedAt          time.Time    `json:"updated_at"`
	StartedAt          *time.Time   `json:"started_at,omitempty"`
	Dependencies       []Dependency `json:"dependencies,omitempty"`
	DependencyCount    int          `json:"dependency_count,omitempty"`
	DependentCount     int          `json:"dependent_count,omitempty"`
	CommentCount       int          `json:"comment_count,omitempty"`
	Parent             string       `json:"parent,omitempty"`
}

// Dependency is the normalized dependency edge DTO.
type Dependency struct {
	IssueID     string    `json:"issue_id"`
	DependsOnID string    `json:"depends_on_id"`
	Type        string    `json:"type"`
	CreatedAt   time.Time `json:"created_at"`
	CreatedBy   string    `json:"created_by,omitempty"`
}

// GraphResult is the normalized response from bd graph.
type GraphResult struct {
	Issues       []Issue        `json:"issues"`
	Dependencies []Dependency   `json:"dependencies"`
}

// IssueStatus represents valid issue statuses.
type IssueStatus string

const (
	StatusOpen       IssueStatus = "open"
	StatusInProgress IssueStatus = "in_progress"
	StatusBlocked    IssueStatus = "blocked"
	StatusClosed     IssueStatus = "closed"
	StatusDeferred   IssueStatus = "deferred"
)

// ValidStatuses lists all valid issue statuses.
var ValidStatuses = []IssueStatus{
	StatusOpen,
	StatusInProgress,
	StatusBlocked,
	StatusClosed,
	StatusDeferred,
}

// IsValidStatus reports whether s is a known issue status.
func IsValidStatus(s IssueStatus) bool {
	for _, v := range ValidStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// IssuePriority represents valid priority levels.
type IssuePriority int

const (
	PriorityP0 IssuePriority = 0
	PriorityP1 IssuePriority = 1
	PriorityP2 IssuePriority = 2
	PriorityP3 IssuePriority = 3
)

// DependencyType represents valid dependency edge types.
type DependencyType string

const (
	DepBlocks      DependencyType = "blocks"
	DepParentChild DependencyType = "parent-child"
	DepRelated     DependencyType = "related"
)

// ValidDependencyTypes lists all valid dependency types.
var ValidDependencyTypes = []DependencyType{
	DepBlocks,
	DepParentChild,
	DepRelated,
}

// IsValidDependencyType reports whether t is a known dependency type.
func IsValidDependencyType(t DependencyType) bool {
	for _, v := range ValidDependencyTypes {
		if v == t {
			return true
		}
	}
	return false
}
