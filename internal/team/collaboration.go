package team

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type CollaborationSignal string

const (
	SignalNone         CollaborationSignal = ""
	SignalAgree        CollaborationSignal = "agree"
	SignalObject       CollaborationSignal = "object"
	SignalYield        CollaborationSignal = "yield"
	SignalProposeFinal CollaborationSignal = "propose_final"
	SignalResolved     CollaborationSignal = "resolved"
)

const (
	activationPeerReview        = "peer_review"
	activationDirectMention     = "direct_mention"
	activationAnswerObjection   = "answer_objection"
	activationReconsider        = "reconsider_objection"
	activationObjectionWindow   = "objection_window"
	stopReasonConverged         = "converged"
	stopReasonMaxAgentTurns     = "max_agent_turns"
	stopReasonMaxWallTime       = "max_wall_time"
	stopReasonUnresolvedNoReply = "unresolved_objections_no_progress"
)

var collaborationSignalPattern = regexp.MustCompile(`(?im)^\s*\[TEAM_SIGNAL:(AGREE|OBJECT|YIELD|PROPOSE_FINAL|RESOLVED)\]\s*$`)

func collaborationSignalInstructions() string {
	return `在正文最后另起一行，按当前立场选择至多一个协作信号:
- [TEAM_SIGNAL:AGREE]：认可当前方向，没有阻塞性异议。
- [TEAM_SIGNAL:OBJECT]：存在会阻止最终结论的异议；正文中必须 @需要回应的成员。
- [TEAM_SIGNAL:YIELD]：没有新的实质贡献。
- [TEAM_SIGNAL:PROPOSE_FINAL]：认为信息已经足够，可以开启最终异议窗口。
- [TEAM_SIGNAL:RESOLVED]：你之前提出的异议已经解决。
协作信号只用于调度，不替代自然、完整的专业表达。`
}

func parseCollaborationResponse(content string, definition *Definition) (string, CollaborationSignal, []string) {
	content = strings.TrimSpace(content)
	signal := SignalNone
	matches := collaborationSignalPattern.FindAllStringSubmatch(content, -1)
	if len(matches) > 0 {
		signal = CollaborationSignal(strings.ToLower(matches[len(matches)-1][1]))
		content = strings.TrimSpace(collaborationSignalPattern.ReplaceAllString(content, ""))
	} else if isYield(content) {
		signal = SignalYield
		content = ""
	}
	return content, signal, mentionedMembers(content, definition)
}

func mentionedMembers(content string, definition *Definition) []string {
	if definition == nil {
		return nil
	}
	lower := strings.ToLower(content)
	var targets []string
	for _, member := range definition.Agents {
		id := strings.ToLower(strings.TrimSpace(member.ID))
		if id == "" {
			continue
		}
		pattern := regexp.MustCompile(`@` + regexp.QuoteMeta(id) + `(?:\b|$)`)
		if pattern.FindStringIndex(lower) != nil {
			targets = append(targets, member.ID)
		}
	}
	return targets
}

func (e *Engine) runAdaptiveReview(ctx context.Context, memory MemoryDocument) error {
	state, err := e.ensureCollaborationState()
	if err != nil {
		return err
	}
	if state.Converged || state.StopReason != "" {
		return nil
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		events, err := e.Store.Events()
		if err != nil {
			return err
		}
		state.Pending = limitPendingReviewTurns(
			state.Pending,
			events,
			e.Store.State.Current.Number,
			e.Definition.Limits.MaxReviewTurnsPerAgent,
		)
		if len(state.Pending) == 0 {
			switch {
			case hasOpenObjections(state):
				return e.forceStopCollaboration(state, stopReasonUnresolvedNoReply)
			case state.BroadReviewWaves < e.Definition.Coordination.ReviewWaves:
				e.scheduleBroadReview(&state)
				if err := e.Store.SetCollaboration(state); err != nil {
					return err
				}
				continue
			default:
				return e.markCollaborationConverged(state)
			}
		}

		turns := countAgentTurns(events, e.Store.State.Current.Number)
		remaining := e.Definition.Limits.MaxAgentTurns - turns - 1
		if remaining <= 0 {
			return e.forceStopCollaboration(state, stopReasonMaxAgentTurns)
		}

		batchSize := e.Definition.Coordination.MaxConcurrency
		if batchSize > len(state.Pending) {
			batchSize = len(state.Pending)
		}
		if batchSize > remaining {
			batchSize = remaining
		}
		batch := append([]Activation(nil), state.Pending[:batchSize]...)
		state.Pending = append([]Activation(nil), state.Pending[batchSize:]...)
		state.Cycle++
		state.TurnCount = turns
		if err := e.Store.SetCollaboration(state); err != nil {
			return err
		}

		for _, activation := range batch {
			if err := e.appendEvent(Event{
				Type:    "agent_activated",
				Round:   e.Store.State.Current.Number,
				Phase:   PhaseReview,
				Wave:    state.Cycle,
				From:    "runtime",
				To:      []string{activation.MemberID},
				Ref:     activation.SourceEventID,
				Content: activation.Reason,
			}); err != nil {
				return err
			}
		}
		before, err := e.Store.Events()
		if err != nil {
			return err
		}
		beforeID := lastEventID(before)
		responses, callErr := e.runReviewActivations(ctx, memory, state.Cycle, batch)
		after, loadErr := e.Store.Events()
		if loadErr != nil {
			return loadErr
		}
		state.TurnCount = countAgentTurns(after, e.Store.State.Current.Number)
		lifecycle := e.applyReviewResponses(&state, batch, responses, responseEventsAfter(after, beforeID))
		for _, event := range lifecycle {
			if err := e.appendEvent(event); err != nil {
				return err
			}
		}
		if callErr != nil {
			for i, response := range responses {
				if response.Err != nil || (response.Content == "" && response.Signal != SignalYield) {
					addActivation(&state, batch[i])
				}
			}
		}
		if err := e.Store.SetCollaboration(state); err != nil {
			return err
		}
		if callErr != nil {
			return callErr
		}
	}
}

func limitPendingReviewTurns(pending []Activation, events []Event, round, maxTurns int) []Activation {
	if maxTurns <= 0 || len(pending) == 0 {
		return pending
	}
	counts := map[string]int{}
	for _, event := range events {
		if event.Round != round || event.Phase != PhaseReview {
			continue
		}
		switch event.Type {
		case "agent_message", "agent_yield", "agent_error":
			counts[strings.ToLower(event.From)]++
		}
	}
	filtered := pending[:0]
	for _, activation := range pending {
		if counts[strings.ToLower(activation.MemberID)] < maxTurns {
			filtered = append(filtered, activation)
		}
	}
	return filtered
}

func (e *Engine) ensureCollaborationState() (CollaborationState, error) {
	existing, err := e.Store.Collaboration()
	if err != nil {
		return CollaborationState{}, err
	}
	if existing != nil {
		return *existing, nil
	}
	events, err := e.Store.Events()
	if err != nil {
		return CollaborationState{}, err
	}
	state := CollaborationState{
		TurnCount:            countAgentTurns(events, e.Store.State.Current.Number),
		InitializedAtEventID: lastEventID(events),
	}
	initialHandoff := strings.TrimSpace(e.Definition.Coordination.InitialHandoff)
	var lifecycle []Event
	for _, event := range events {
		if event.Round != e.Store.State.Current.Number {
			continue
		}
		if event.Phase == PhaseReview && event.Wave > state.BroadReviewWaves {
			state.BroadReviewWaves = event.Wave
		}
		if event.Type != "agent_message" && event.Type != "agent_yield" {
			continue
		}
		if initialHandoff != "" && event.Phase == PhaseInitial {
			continue
		}
		lifecycle = append(lifecycle, e.applyResponseEvent(&state, Activation{}, event)...)
	}
	if state.BroadReviewWaves > e.Definition.Coordination.ReviewWaves {
		state.BroadReviewWaves = e.Definition.Coordination.ReviewWaves
	}
	if len(state.Pending) == 0 && initialHandoff != "" {
		addActivation(&state, Activation{MemberID: initialHandoff, Reason: "initial_handoff"})
	} else if len(state.Pending) == 0 && state.BroadReviewWaves < e.Definition.Coordination.ReviewWaves {
		e.scheduleBroadReview(&state)
	}
	if err := e.appendEvent(Event{
		Type:    "collaboration_started",
		Round:   e.Store.State.Current.Number,
		Phase:   PhaseReview,
		Content: "adaptive routing initialized",
	}); err != nil {
		return CollaborationState{}, err
	}
	for _, event := range lifecycle {
		if err := e.appendEvent(event); err != nil {
			return CollaborationState{}, err
		}
	}
	if err := e.Store.SetCollaboration(state); err != nil {
		return CollaborationState{}, err
	}
	return state, nil
}

func (e *Engine) applyReviewResponses(state *CollaborationState, activations []Activation, responses []agentResponse, events map[string]Event) []Event {
	var lifecycle []Event
	for i, response := range responses {
		if response.Err != nil || (response.Content == "" && response.Signal != SignalYield) {
			continue
		}
		event, ok := events[strings.ToLower(response.Member.ID)]
		if !ok {
			continue
		}
		activation := Activation{}
		if i < len(activations) {
			activation = activations[i]
		}
		lifecycle = append(lifecycle, e.applyResponseEvent(state, activation, event)...)
	}
	return lifecycle
}

func (e *Engine) applyResponseEvent(state *CollaborationState, activation Activation, event Event) []Event {
	var lifecycle []Event
	signal := CollaborationSignal(event.Signal)
	if signal == SignalAgree || signal == SignalYield || signal == SignalResolved || signal == SignalProposeFinal || signal == SignalObject {
		lifecycle = append(lifecycle, resolveObjectionsBy(state, event.From, event.ID)...)
	}

	if signal == SignalObject {
		objection := Objection{
			EventID: event.ID,
			From:    event.From,
			Targets: validEventTargets(event.To, e.Definition),
			Content: event.Content,
		}
		state.Objections = append(state.Objections, objection)
		lifecycle = append(lifecycle, Event{
			Type:    "objection_opened",
			Round:   event.Round,
			Phase:   PhaseReview,
			From:    event.From,
			To:      objection.Targets,
			Ref:     event.ID,
			Content: event.Content,
		})
		for _, target := range objection.Targets {
			if !strings.EqualFold(target, event.From) {
				addActivation(state, Activation{MemberID: target, Reason: activationAnswerObjection, SourceEventID: event.ID})
			}
		}
	}

	if signal == SignalProposeFinal {
		state.ProposalBy = event.From
		for _, member := range e.Definition.Agents {
			if !strings.EqualFold(member.ID, event.From) {
				addActivation(state, Activation{MemberID: member.ID, Reason: activationObjectionWindow, SourceEventID: event.ID})
			}
		}
	}

	for _, target := range validEventTargets(event.To, e.Definition) {
		if !strings.EqualFold(target, event.From) {
			addActivation(state, Activation{MemberID: target, Reason: activationDirectMention, SourceEventID: event.ID})
		}
	}

	if strings.Contains(activation.Reason, activationAnswerObjection) && activation.SourceEventID > 0 {
		if objection := objectionByID(state, activation.SourceEventID); objection != nil && !objection.Resolved {
			addActivation(state, Activation{
				MemberID:      objection.From,
				Reason:        activationReconsider,
				SourceEventID: objection.EventID,
			})
		}
	}
	return lifecycle
}

func (e *Engine) scheduleBroadReview(state *CollaborationState) {
	state.BroadReviewWaves++
	finalizer := e.Definition.Coordination.Finalizer
	for _, member := range e.Definition.Agents {
		if strings.EqualFold(member.ID, finalizer) {
			continue
		}
		addActivation(state, Activation{MemberID: member.ID, Reason: activationPeerReview})
	}
}

func addActivation(state *CollaborationState, activation Activation) {
	activation.MemberID = strings.TrimSpace(activation.MemberID)
	if activation.MemberID == "" {
		return
	}
	for i := range state.Pending {
		if !strings.EqualFold(state.Pending[i].MemberID, activation.MemberID) {
			continue
		}
		if !strings.Contains(state.Pending[i].Reason, activation.Reason) {
			state.Pending[i].Reason += "+" + activation.Reason
		}
		if state.Pending[i].SourceEventID == 0 {
			state.Pending[i].SourceEventID = activation.SourceEventID
		}
		return
	}
	state.Pending = append(state.Pending, activation)
}

func (e *Engine) defaultObjectionTargets(from string) []string {
	if !strings.EqualFold(from, e.Definition.Coordination.Finalizer) {
		return []string{e.Definition.Coordination.Finalizer}
	}
	var targets []string
	for _, member := range e.Definition.Agents {
		if !strings.EqualFold(member.ID, from) {
			targets = append(targets, member.ID)
		}
	}
	return targets
}

func resolveObjectionsBy(state *CollaborationState, memberID string, eventID int64) []Event {
	var events []Event
	for i := range state.Objections {
		objection := &state.Objections[i]
		if objection.Resolved || !strings.EqualFold(objection.From, memberID) {
			continue
		}
		objection.Resolved = true
		objection.ResolvedByEventID = eventID
		events = append(events, Event{
			Type:    "objection_resolved",
			From:    memberID,
			Ref:     objection.EventID,
			Content: fmt.Sprintf("objection %d resolved", objection.EventID),
		})
	}
	return events
}

func objectionByID(state *CollaborationState, eventID int64) *Objection {
	for i := range state.Objections {
		if state.Objections[i].EventID == eventID {
			return &state.Objections[i]
		}
	}
	return nil
}

func hasOpenObjections(state CollaborationState) bool {
	for _, objection := range state.Objections {
		if !objection.Resolved {
			return true
		}
	}
	return false
}

func (e *Engine) markCollaborationConverged(state CollaborationState) error {
	if err := e.appendEvent(Event{
		Type:    "convergence_reached",
		Round:   e.Store.State.Current.Number,
		Phase:   PhaseReview,
		From:    "tt",
		Content: stopReasonConverged,
	}); err != nil {
		return err
	}
	state.Converged = true
	state.StopReason = stopReasonConverged
	return e.Store.SetCollaboration(state)
}

func (e *Engine) forceStopCollaboration(state CollaborationState, reason string) error {
	if err := e.appendEvent(Event{
		Type:    "forced_stop",
		Round:   e.Store.State.Current.Number,
		Phase:   PhaseReview,
		From:    "tt",
		Content: reason,
	}); err != nil {
		return err
	}
	state.StopReason = reason
	return e.Store.SetCollaboration(state)
}

func (e *Engine) recordFinalTurn() error {
	state, err := e.Store.Collaboration()
	if err != nil || state == nil {
		return err
	}
	events, err := e.Store.Events()
	if err != nil {
		return err
	}
	state.TurnCount = countAgentTurns(events, e.Store.State.Current.Number)
	return e.Store.SetCollaboration(*state)
}

func (e *Engine) collaborationSummary() string {
	state, err := e.Store.Collaboration()
	if err != nil || state == nil {
		return "未记录协作状态；根据讨论内容谨慎总结。"
	}
	var lines []string
	if state.StopReason == stopReasonConverged {
		lines = append(lines, "团队已达到运行时收敛条件。")
	} else if state.StopReason != "" {
		lines = append(lines, "团队因边界条件停止: "+state.StopReason+"。")
	}
	for _, objection := range state.Objections {
		if objection.Resolved {
			continue
		}
		lines = append(lines, fmt.Sprintf("未解决异议（@%s）: %s", objection.From, compact(objection.Content, 1000)))
	}
	if len(lines) == 0 {
		return "没有未解决的阻塞性异议。"
	}
	return strings.Join(lines, "\n")
}

func countAgentTurns(events []Event, round int) int {
	count := 0
	for _, event := range events {
		if event.Round != round {
			continue
		}
		switch event.Type {
		case "agent_message", "agent_yield", "agent_error", "final_answer":
			count++
		}
	}
	return count
}

func responseEventsAfter(events []Event, afterID int64) map[string]Event {
	out := map[string]Event{}
	for _, event := range events {
		if event.ID <= afterID || (event.Type != "agent_message" && event.Type != "agent_yield" && event.Type != "agent_error") {
			continue
		}
		out[strings.ToLower(event.From)] = event
	}
	return out
}

func validEventTargets(targets []string, definition *Definition) []string {
	if definition == nil {
		return nil
	}
	seen := map[string]bool{}
	var valid []string
	for _, target := range targets {
		member, ok := definition.AgentByID(target)
		if !ok {
			continue
		}
		key := strings.ToLower(member.ID)
		if seen[key] {
			continue
		}
		seen[key] = true
		valid = append(valid, member.ID)
	}
	sort.SliceStable(valid, func(i, j int) bool {
		return definitionAgentIndex(definition, valid[i]) < definitionAgentIndex(definition, valid[j])
	})
	return valid
}

func definitionAgentIndex(definition *Definition, id string) int {
	for i, member := range definition.Agents {
		if strings.EqualFold(member.ID, id) {
			return i
		}
	}
	return len(definition.Agents)
}

func lastEventID(events []Event) int64 {
	if len(events) == 0 {
		return 0
	}
	return events[len(events)-1].ID
}
