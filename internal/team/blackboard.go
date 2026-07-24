package team

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

type BlackboardKind string

const (
	BlackboardFact           BlackboardKind = "fact"
	BlackboardProposal       BlackboardKind = "proposal"
	BlackboardQuestion       BlackboardKind = "question"
	BlackboardDecision       BlackboardKind = "decision"
	BlackboardObjection      BlackboardKind = "objection"
	BlackboardArtifact       BlackboardKind = "artifact"
	BlackboardActionUpsert                  = "upsert"
	BlackboardActionResolve                 = "resolve"
	BlackboardStatusActive                  = "active"
	BlackboardStatusResolved                = "resolved"
)

const blackboardPrefix = "[TEAM_BLACKBOARD]"

type BlackboardOperation struct {
	Action  string         `json:"action"`
	Kind    BlackboardKind `json:"kind"`
	Key     string         `json:"key"`
	Content string         `json:"content,omitempty"`
}

type BlackboardRevision struct {
	EventID int64  `json:"event_id"`
	Ref     int64  `json:"ref,omitempty"`
	Action  string `json:"action"`
	By      string `json:"by,omitempty"`
	At      string `json:"at,omitempty"`
	Content string `json:"content,omitempty"`
}

type BlackboardEntry struct {
	Kind             BlackboardKind       `json:"kind"`
	Key              string               `json:"key"`
	Content          string               `json:"content,omitempty"`
	Status           string               `json:"status"`
	UpdatedBy        string               `json:"updated_by,omitempty"`
	UpdatedAtEventID int64                `json:"updated_at_event_id"`
	Revisions        []BlackboardRevision `json:"revisions"`
}

type BlackboardProjection struct {
	Round   int               `json:"round"`
	Entries []BlackboardEntry `json:"entries"`
}

func blackboardInstructions() string {
	return `你们共享一个由事件流维护的结构化工作黑板。需要记录或更新关键信息时，可在正文末尾追加一行或多行:
[TEAM_BLACKBOARD] {"action":"upsert","kind":"fact","key":"stable-key","content":"需要共享的内容"}

kind 只能是 fact、proposal、question、decision、objection、artifact。
要关闭问题或异议时使用:
[TEAM_BLACKBOARD] {"action":"resolve","kind":"question","key":"stable-key"}

key 应简短稳定；更新相同 kind + key 会保留全部来源记录。黑板用于本轮协作，不等同于跨轮次团队记忆。`
}

func parseBlackboardOperations(content string) (string, []BlackboardOperation) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	clean := make([]string, 0, len(lines))
	var operations []BlackboardOperation
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, blackboardPrefix) {
			clean = append(clean, line)
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(trimmed, blackboardPrefix))
		var operation BlackboardOperation
		if payload == "" || json.Unmarshal([]byte(payload), &operation) != nil {
			clean = append(clean, line)
			continue
		}
		normalized, err := normalizeBlackboardOperation(operation)
		if err != nil {
			clean = append(clean, line)
			continue
		}
		operations = append(operations, normalized)
	}
	return strings.TrimSpace(strings.Join(clean, "\n")), operations
}

func normalizeBlackboardOperation(operation BlackboardOperation) (BlackboardOperation, error) {
	operation.Action = strings.ToLower(strings.TrimSpace(operation.Action))
	operation.Kind = BlackboardKind(strings.ToLower(strings.TrimSpace(string(operation.Kind))))
	operation.Key = strings.ToLower(strings.TrimSpace(operation.Key))
	operation.Content = strings.TrimSpace(operation.Content)
	if operation.Action != BlackboardActionUpsert && operation.Action != BlackboardActionResolve {
		return BlackboardOperation{}, fmt.Errorf("unsupported blackboard action %q", operation.Action)
	}
	switch operation.Kind {
	case BlackboardFact, BlackboardProposal, BlackboardQuestion, BlackboardDecision, BlackboardObjection, BlackboardArtifact:
	default:
		return BlackboardOperation{}, fmt.Errorf("unsupported blackboard kind %q", operation.Kind)
	}
	if operation.Key == "" || strings.ContainsAny(operation.Key, "\r\n") || utf8.RuneCountInString(operation.Key) > 80 {
		return BlackboardOperation{}, fmt.Errorf("invalid blackboard key")
	}
	if operation.Action == BlackboardActionUpsert && operation.Content == "" {
		return BlackboardOperation{}, fmt.Errorf("blackboard upsert content is required")
	}
	if utf8.RuneCountInString(operation.Content) > 4000 {
		return BlackboardOperation{}, fmt.Errorf("blackboard content exceeds 4000 characters")
	}
	if operation.Action == BlackboardActionResolve {
		operation.Content = ""
	}
	return operation, nil
}

func ProjectBlackboard(events []Event, round int) BlackboardProjection {
	projection := BlackboardProjection{Round: round, Entries: []BlackboardEntry{}}
	entries := map[string]*BlackboardEntry{}
	ordered := append([]Event(nil), events...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].ID < ordered[j].ID
	})
	for _, event := range ordered {
		if event.Round != round || event.Blackboard == nil {
			continue
		}
		operation, err := normalizeBlackboardOperation(*event.Blackboard)
		if err != nil {
			continue
		}
		mapKey := string(operation.Kind) + "\x00" + operation.Key
		entry := entries[mapKey]
		if entry == nil {
			entry = &BlackboardEntry{
				Kind:   operation.Kind,
				Key:    operation.Key,
				Status: BlackboardStatusActive,
			}
			entries[mapKey] = entry
		}
		entry.Revisions = append(entry.Revisions, BlackboardRevision{
			EventID: event.ID,
			Ref:     event.Ref,
			Action:  operation.Action,
			By:      event.From,
			At:      event.At,
			Content: operation.Content,
		})
		entry.UpdatedBy = event.From
		entry.UpdatedAtEventID = event.ID
		switch operation.Action {
		case BlackboardActionUpsert:
			entry.Content = operation.Content
			entry.Status = BlackboardStatusActive
		case BlackboardActionResolve:
			entry.Status = BlackboardStatusResolved
		}
	}
	for _, entry := range entries {
		projection.Entries = append(projection.Entries, *entry)
	}
	sort.Slice(projection.Entries, func(i, j int) bool {
		if projection.Entries[i].Kind != projection.Entries[j].Kind {
			return projection.Entries[i].Kind < projection.Entries[j].Kind
		}
		return projection.Entries[i].Key < projection.Entries[j].Key
	})
	return projection
}

func formatBlackboard(events []Event, round int) string {
	projection := ProjectBlackboard(events, round)
	if len(projection.Entries) == 0 {
		return "(empty)"
	}
	var builder strings.Builder
	for _, entry := range projection.Entries {
		fmt.Fprintf(
			&builder,
			"- [%s/%s] %s · @%s · event %d\n  %s\n",
			entry.Kind,
			entry.Key,
			entry.Status,
			entry.UpdatedBy,
			entry.UpdatedAtEventID,
			fallback(compact(entry.Content, 1500), "(no content)"),
		)
	}
	return compact(strings.TrimSpace(builder.String()), 12000)
}
