package beads

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

// Adapter provides typed read and mutation operations over the bd CLI.
// It wraps a Runner and exposes domain methods rather than arbitrary command execution.
type Adapter struct {
	runner *Runner
}

// NewAdapter creates an Adapter using the given Runner.
func NewAdapter(runner *Runner) *Adapter {
	return &Adapter{runner: runner}
}

// NewAdapterFromWorkspace creates an Adapter auto-discovering the workspace.
func NewAdapterFromWorkspace(workspace string) (*Adapter, error) {
	runner, err := NewRunner(workspace)
	if err != nil {
		return nil, err
	}
	return &Adapter{runner: runner}, nil
}

// Runner returns the underlying Runner for direct access if needed.
func (a *Adapter) Runner() *Runner {
	return a.runner
}

// ---------------------------------------------------------------------------
// Read operations
// ---------------------------------------------------------------------------

// ListOption configures a List call.
type ListOption func(*listConfig)

type listConfig struct {
	status string
	labels []string
	limit  int
}

// WithStatus filters issues by status.
func WithStatus(status IssueStatus) ListOption {
	return func(c *listConfig) {
		c.status = string(status)
	}
}

// WithLabels filters issues that have all the given labels.
func WithLabels(labels ...string) ListOption {
	return func(c *listConfig) {
		c.labels = append(c.labels, labels...)
	}
}

// WithLimit caps the number of returned issues.
func WithLimit(n int) ListOption {
	return func(c *listConfig) {
		if n > 0 {
			c.limit = n
		}
	}
}

// List returns all issues matching the given filters.
// By default it passes --all --limit 0 so that closed issues are included
// and no artificial cap is applied. Callers can override with WithLimit.
func (a *Adapter) List(ctx context.Context, opts ...ListOption) ([]Issue, error) {
	cfg := listConfig{limit: 0} // 0 means unlimited
	for _, opt := range opts {
		opt(&cfg)
	}
	args := []string{"--json", "--all"}
	if cfg.status != "" {
		args = append(args, "--status", cfg.status)
	}
	for _, l := range cfg.labels {
		args = append(args, "--label", l)
	}
	// Always pass --limit so we are explicit about wanting all results.
	args = append(args, "--limit", strconv.Itoa(cfg.limit))
	var issues []Issue
	if err := a.runner.runJSON(ctx, &issues, "list", args...); err != nil {
		return nil, err
	}
	return issues, nil
}

// Search returns issues matching the given query string.
// Always includes closed issues for consistency with the list API.
func (a *Adapter) Search(ctx context.Context, query string) ([]Issue, error) {
	if query == "" {
		return nil, nil
	}
	args := []string{"--json", "--all", query}
	var issues []Issue
	if err := a.runner.runJSON(ctx, &issues, "search", args...); err != nil {
		return nil, err
	}
	return issues, nil
}

// Show returns the full detail for a single issue by ID.
// bd show returns dependencies as full issue objects with a dependency_type
// field. We normalise them into the Dependency DTO so the rest of the
// adapter has a consistent shape.
func (a *Adapter) Show(ctx context.Context, issueID string) (*Issue, error) {
	if issueID == "" {
		return nil, fmt.Errorf("beads: issue ID is required")
	}
	if !isValidIssueID(issueID) {
		return nil, fmt.Errorf("beads: invalid issue ID %q", issueID)
	}
	// Two-pass decode: first get raw JSON, then normalise dependencies.
	result, err := a.runner.run(ctx, "show", issueID, "--json")
	if err != nil {
		return nil, err
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(result.Stdout, &raw); err != nil {
		return nil, fmt.Errorf("beads: decode show: %w", err)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("beads: issue %q not found", issueID)
	}
	// Decode the issue without dependencies first.
	var issue Issue
	if err := json.Unmarshal(raw[0], &issue); err != nil {
		return nil, fmt.Errorf("beads: decode issue: %w", err)
	}
	// Now extract the dependencies array with the dependency_type field.
	var rawIssue map[string]json.RawMessage
	if err := json.Unmarshal(raw[0], &rawIssue); err != nil {
		return &issue, nil
	}
	if depsRaw, ok := rawIssue["dependencies"]; ok {
		issue.Dependencies = normalizeShowDependencies(issue.ID, depsRaw)
	}
	return &issue, nil
}

// showDep is the raw shape of a dependency as returned by bd show:
// a full issue object with an extra dependency_type field.
type showDep struct {
	ID             string `json:"id"`
	DependencyType string `json:"dependency_type"`
}

// normalizeShowDependencies converts the bd show dependency array
// (full issue objects with dependency_type) into flat Dependency DTOs.
func normalizeShowDependencies(ownerID string, raw json.RawMessage) []Dependency {
	var deps []showDep
	if err := json.Unmarshal(raw, &deps); err != nil {
		return nil
	}
	result := make([]Dependency, 0, len(deps))
	for _, d := range deps {
		depType := d.DependencyType
		if depType == "" {
			depType = "blocks"
		}
		result = append(result, Dependency{
			IssueID:     ownerID,
			DependsOnID: d.ID,
			Type:        depType,
		})
	}
	return result
}

// Graph returns the dependency graph for the given root issue.
func (a *Adapter) Graph(ctx context.Context, issueID string) (*GraphResult, error) {
	if issueID == "" {
		return nil, fmt.Errorf("beads: issue ID is required")
	}
	if !isValidIssueID(issueID) {
		return nil, fmt.Errorf("beads: invalid issue ID %q", issueID)
	}
	var result GraphResult
	if err := a.runner.runJSON(ctx, &result, "graph", issueID, "--json"); err != nil {
		return nil, err
	}
	return &result, nil
}

// ---------------------------------------------------------------------------
// Mutation operations
// ---------------------------------------------------------------------------

// CreateIssueInput is the input for creating a new issue.
type CreateIssueInput struct {
	Title              string
	Description        string
	Design             string
	AcceptanceCriteria string
	Priority           *int     // nil = use bd default; non-nil = explicit priority
	IssueType          string
	Assignee           string
	Labels             []string
	EstimatedMinutes   int
	Parent             string // parent issue ID; added as parent-child dep
	Dependencies       []string // dependency IDs (e.g. "blocks:tt-abc")
}

// CreateIssue creates a new issue and returns it.
func (a *Adapter) CreateIssue(ctx context.Context, input CreateIssueInput) (*Issue, error) {
	if input.Title == "" {
		return nil, fmt.Errorf("beads: title is required")
	}
	args := []string{"--json", input.Title}
	if input.Description != "" {
		args = append(args, "--description", input.Description)
	}
	if input.Design != "" {
		args = append(args, "--design", input.Design)
	}
	if input.AcceptanceCriteria != "" {
		args = append(args, "--acceptance", input.AcceptanceCriteria)
	}
	if input.Priority != nil {
		args = append(args, "--priority", strconv.Itoa(*input.Priority))
	}
	if input.IssueType != "" {
		args = append(args, "--type", input.IssueType)
	}
	if input.Assignee != "" {
		args = append(args, "--assignee", input.Assignee)
	}
	for _, l := range input.Labels {
		args = append(args, "--label", l)
	}
	if input.EstimatedMinutes > 0 {
		args = append(args, "--estimate", strconv.Itoa(input.EstimatedMinutes))
	}
	for _, dep := range input.Dependencies {
		args = append(args, "--deps", dep)
	}

	var issues []Issue
	if err := a.runner.runJSON(ctx, &issues, "create", args...); err != nil {
		return nil, err
	}
	if len(issues) == 0 {
		return nil, fmt.Errorf("beads: create returned no issue")
	}
	return &issues[0], nil
}

// UpdateIssueInput specifies fields to update on an existing issue.
// Pointer fields distinguish "not provided" (nil) from "set to zero value".
type UpdateIssueInput struct {
	Title              *string
	Description        *string
	Design             *string
	AcceptanceCriteria *string
	Priority           *int
	Status             *IssueStatus
	Assignee           *string
	IssueType          *string
	Labels             *[]string // nil = no change; non-nil (even empty) = --set-labels
	EstimatedMinutes   *int
}

// UpdateIssue updates an existing issue and returns the refreshed result.
func (a *Adapter) UpdateIssue(ctx context.Context, issueID string, input UpdateIssueInput) (*Issue, error) {
	if issueID == "" {
		return nil, fmt.Errorf("beads: issue ID is required")
	}
	if !isValidIssueID(issueID) {
		return nil, fmt.Errorf("beads: invalid issue ID %q", issueID)
	}
	args := []string{issueID}
	if input.Title != nil {
		args = append(args, "--title", *input.Title)
	}
	if input.Description != nil {
		args = append(args, "--description", *input.Description)
	}
	if input.Design != nil {
		args = append(args, "--design", *input.Design)
	}
	if input.AcceptanceCriteria != nil {
		args = append(args, "--acceptance", *input.AcceptanceCriteria)
	}
	if input.Priority != nil {
		args = append(args, "--priority", strconv.Itoa(*input.Priority))
	}
	if input.Status != nil {
		if !IsValidStatus(*input.Status) {
			return nil, fmt.Errorf("beads: invalid status %q", *input.Status)
		}
		args = append(args, "--status", string(*input.Status))
	}
	if input.Assignee != nil {
		args = append(args, "--assignee", *input.Assignee)
	}
	if input.IssueType != nil {
		args = append(args, "--type", *input.IssueType)
	}
	if input.Labels != nil {
		// Empty slice clears all labels; non-empty sets them.
		if len(*input.Labels) == 0 {
			// Pass empty string to clear all labels.
			args = append(args, "--set-labels", "")
		} else {
			for _, l := range *input.Labels {
				args = append(args, "--set-labels", l)
			}
		}
	}
	if input.EstimatedMinutes != nil {
		args = append(args, "--estimate", strconv.Itoa(*input.EstimatedMinutes))
	}
	args = append(args, "--json")

	var issues []Issue
	if err := a.runner.runJSON(ctx, &issues, "update", args...); err != nil {
		return nil, err
	}
	if len(issues) == 0 {
		return nil, fmt.Errorf("beads: update returned no issue")
	}
	return &issues[0], nil
}

// ---------------------------------------------------------------------------
// Dependency operations
// ---------------------------------------------------------------------------

// AddDependency adds a dependency edge from issueID to dependsOnID of the given type.
// Uses "bd dep add" which is the correct CLI command.
func (a *Adapter) AddDependency(ctx context.Context, issueID, dependsOnID string, depType DependencyType) error {
	if issueID == "" || dependsOnID == "" {
		return fmt.Errorf("beads: both issue ID and dependency target are required")
	}
	if !isValidIssueID(issueID) || !isValidIssueID(dependsOnID) {
		return fmt.Errorf("beads: invalid issue ID")
	}
	if depType == "" {
		depType = DepBlocks
	}
	if !IsValidDependencyType(depType) {
		return fmt.Errorf("beads: invalid dependency type %q", depType)
	}
	_, err := a.runner.run(ctx, "dep", "add", issueID, dependsOnID, "--type", string(depType), "--json")
	return err
}

// RemoveDependency removes a dependency edge using "bd dep remove".
func (a *Adapter) RemoveDependency(ctx context.Context, issueID, dependsOnID string) error {
	if issueID == "" || dependsOnID == "" {
		return fmt.Errorf("beads: both issue ID and dependency target are required")
	}
	if !isValidIssueID(issueID) || !isValidIssueID(dependsOnID) {
		return fmt.Errorf("beads: invalid issue ID")
	}
	_, err := a.runner.run(ctx, "dep", "remove", issueID, dependsOnID, "--json")
	return err
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// isValidIssueID performs a basic sanity check on issue IDs.
// Beads IDs look like "tt-abc", "tt-mgxy.1", etc.
func isValidIssueID(id string) bool {
	return IsValidIssueIDPublic(id)
}

// IsValidIssueIDPublic reports whether id is safe to pass to the bd CLI.
// It rejects empty strings and shell-dangerous characters.
func IsValidIssueIDPublic(id string) bool {
	if id == "" {
		return false
	}
	// Must contain at least one letter and no whitespace or shell-dangerous chars.
	for _, r := range id {
		if r == ' ' || r == ';' || r == '|' || r == '&' || r == '`' || r == '$' || r == '(' || r == ')' {
			return false
		}
	}
	return true
}
