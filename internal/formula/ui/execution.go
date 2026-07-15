package ui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxExecutionInstances = 500
	maxExecutionEvents    = 500
	maxExecutionOutput    = 8192
	maxExecutionDetail    = 4096
)

var loopIterationPattern = regexp.MustCompile(`\.iter(\d+)\.`)

// ExecutionInstance is one concrete runtime address. Top-level steps have an
// address equal to their definition ID; loop bodies use parent.iterN.body.
type ExecutionInstance struct {
	Address          string `json:"address"`
	DefinitionStepID string `json:"definition_step_id"`
	ParentLoopID     string `json:"parent_loop_id,omitempty"`
	BodyStepID       string `json:"body_step_id,omitempty"`
	IterationPath    []int  `json:"iteration_path,omitempty"`
	Title            string `json:"title,omitempty"`
	Status           string `json:"status"`
	Attempt          int    `json:"attempt,omitempty"`
	StartedAt        string `json:"started_at,omitempty"`
	FinishedAt       string `json:"finished_at,omitempty"`
	UpdatedAt        string `json:"updated_at,omitempty"`
	DurationMS       int64  `json:"duration_ms,omitempty"`
	Session          string `json:"session,omitempty"`
	Detail           string `json:"detail,omitempty"`
	Output           string `json:"output,omitempty"`
	Error            string `json:"error,omitempty"`
}

// ExecutionEvent is immutable event-time history. It intentionally records
// the status at the transition rather than relying on a step's current state.
type ExecutionEvent struct {
	ID               string `json:"id"`
	At               string `json:"at"`
	InstanceAddress  string `json:"instance_address,omitempty"`
	DefinitionStepID string `json:"definition_step_id,omitempty"`
	ParentLoopID     string `json:"parent_loop_id,omitempty"`
	Type             string `json:"type"`
	FromStatus       string `json:"from_status,omitempty"`
	Status           string `json:"status"`
	Attempt          int    `json:"attempt,omitempty"`
	Title            string `json:"title,omitempty"`
	Detail           string `json:"detail,omitempty"`
	DurationMS       int64  `json:"duration_ms,omitempty"`
	Session          string `json:"session,omitempty"`
	Error            string `json:"error,omitempty"`
}

type ExecutionTransition struct {
	Address    string
	Title      string
	Status     string
	Session    string
	Detail     string
	Output     string
	Error      string
	DurationMS int64
	At         time.Time
}

func ParseExecutionAddress(address string) (parentLoopID string, iterationPath []int, bodyStepID string) {
	matches := loopIterationPattern.FindAllStringSubmatchIndex(address, -1)
	if len(matches) == 0 {
		return "", nil, ""
	}
	parentLoopID = address[:matches[0][0]]
	for _, match := range matches {
		if value, err := strconv.Atoi(address[match[2]:match[3]]); err == nil {
			iterationPath = append(iterationPath, value)
		}
	}
	bodyStepID = address[matches[len(matches)-1][1]:]
	return parentLoopID, iterationPath, bodyStepID
}

func RecordExecutionTransition(snapshot *Snapshot, transition ExecutionTransition) {
	if snapshot == nil || strings.TrimSpace(transition.Address) == "" || strings.TrimSpace(transition.Status) == "" {
		return
	}
	now := transition.At
	if now.IsZero() {
		now = time.Now()
	}
	parentLoopID, iterationPath, bodyStepID := ParseExecutionAddress(transition.Address)
	definitionStepID := transition.Address
	if bodyStepID != "" {
		definitionStepID = bodyStepID
	}

	index := -1
	for i := range snapshot.ExecutionInstances {
		if snapshot.ExecutionInstances[i].Address == transition.Address {
			index = i
			break
		}
	}
	previousStatus := ""
	if index < 0 {
		snapshot.ExecutionInstances = append(snapshot.ExecutionInstances, ExecutionInstance{
			Address: transition.Address, DefinitionStepID: definitionStepID,
			ParentLoopID: parentLoopID, BodyStepID: bodyStepID,
			IterationPath: append([]int(nil), iterationPath...),
		})
		index = len(snapshot.ExecutionInstances) - 1
	}
	instance := &snapshot.ExecutionInstances[index]
	previousStatus = instance.Status
	if instance.Attempt == 0 {
		instance.Attempt = 1
	} else if transition.Status == StatusRunning && isTerminalExecutionStatus(previousStatus) {
		instance.Attempt++
	}
	if transition.Title != "" {
		instance.Title = transition.Title
	}
	if transition.Session != "" {
		instance.Session = transition.Session
	}
	instance.Status = transition.Status
	instance.UpdatedAt = now.Format(time.RFC3339Nano)
	instance.Detail = truncateExecutionText(transition.Detail, maxExecutionDetail)
	instance.Output = truncateExecutionText(transition.Output, maxExecutionOutput)
	instance.Error = truncateExecutionText(transition.Error, maxExecutionDetail)
	if transition.Status == StatusRunning {
		instance.StartedAt = now.Format(time.RFC3339Nano)
		instance.FinishedAt = ""
		instance.DurationMS = 0
	} else if isTerminalExecutionStatus(transition.Status) {
		instance.FinishedAt = now.Format(time.RFC3339Nano)
		instance.DurationMS = transition.DurationMS
		if instance.DurationMS == 0 && instance.StartedAt != "" {
			if started, err := time.Parse(time.RFC3339Nano, instance.StartedAt); err == nil {
				instance.DurationMS = now.Sub(started).Milliseconds()
			}
		}
	} else if transition.DurationMS > 0 {
		instance.DurationMS = transition.DurationMS
	}

	eventType := executionEventType(transition.Status)
	snapshot.ExecutionEvents = append(snapshot.ExecutionEvents, ExecutionEvent{
		ID: fmt.Sprintf("%d-%d", now.UnixNano(), len(snapshot.ExecutionEvents)),
		At: now.Format(time.RFC3339Nano), InstanceAddress: transition.Address,
		DefinitionStepID: definitionStepID, ParentLoopID: parentLoopID,
		Type: eventType, FromStatus: previousStatus, Status: transition.Status,
		Attempt: instance.Attempt,
		Title:   instance.Title, Detail: instance.Detail, DurationMS: instance.DurationMS,
		Session: instance.Session, Error: instance.Error,
	})
	if len(snapshot.ExecutionEvents) > maxExecutionEvents {
		snapshot.ExecutionEvents = append([]ExecutionEvent(nil), snapshot.ExecutionEvents[len(snapshot.ExecutionEvents)-maxExecutionEvents:]...)
	}
	if len(snapshot.ExecutionInstances) > maxExecutionInstances {
		trimExecutionInstances(snapshot)
	}
}

func truncateExecutionText(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	return value[:limit] + "\n… truncated in dashboard execution snapshot"
}

func executionEventType(status string) string {
	switch status {
	case StatusRunning:
		return "started"
	case StatusCompleted:
		return "completed"
	case StatusFailed:
		return "failed"
	case StatusSkipped:
		return "skipped"
	case StatusWaitingInput:
		return "waiting_input"
	default:
		return status
	}
}

func isTerminalExecutionStatus(status string) bool {
	return status == StatusCompleted || status == StatusFailed || status == StatusSkipped || status == "interrupted"
}

func trimExecutionInstances(snapshot *Snapshot) {
	active := make([]ExecutionInstance, 0, len(snapshot.ExecutionInstances))
	terminal := make([]ExecutionInstance, 0, len(snapshot.ExecutionInstances))
	for _, instance := range snapshot.ExecutionInstances {
		if instance.Status == StatusRunning || instance.Status == StatusWaitingInput {
			active = append(active, instance)
		} else {
			terminal = append(terminal, instance)
		}
	}
	keepTerminal := maxExecutionInstances - len(active)
	if keepTerminal < 0 {
		keepTerminal = 0
	}
	if len(terminal) > keepTerminal {
		terminal = terminal[len(terminal)-keepTerminal:]
	}
	snapshot.ExecutionInstances = append(active, terminal...)
}
