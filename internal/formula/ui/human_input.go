package ui

import "github.com/sjzsdu/tt/internal/formula/spec"

// HumanInputRequest is the dashboard/runtime DTO for formula human input.
// It intentionally lives with formula UI state instead of the legacy executor
// package so typed runtime paths do not depend on executor data types.
type HumanInputRequest struct {
	Reason string         `json:"reason,omitempty"`
	Form   *spec.FormSpec `json:"form,omitempty"`
}
