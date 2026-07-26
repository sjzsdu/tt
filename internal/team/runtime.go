package team

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type Processor interface {
	Process(context.Context, AgentCall) (string, error)
}

type ExternalSessionStore interface {
	ExternalSession(key string) string
	SetExternalSession(key, sessionID string) error
}

type AgentCall struct {
	MemberID         string
	Agent            string
	Model            string
	Session          string
	Prompt           string
	External         *ExternalAgentConfig
	ExternalSessions ExternalSessionStore
	Workspace        string
}

type Engine struct {
	Definition    *Definition
	Store         *Store
	Processor     Processor
	SessionPrefix string
	Model         string
	DisableMemory bool
	OnEvent       func(Event)
}

type RunResult struct {
	ThreadID      string
	Round         int
	Answer        string
	Memory        MemoryDocument
	MemoryWarning error
}

type agentResponse struct {
	Member       Agent
	Session      string
	Content      string
	Signal       CollaborationSignal
	Targets      []string
	Blackboard   []BlackboardOperation
	Verification []VerificationResult
	Metrics      EventMetrics
	Err          error
}

const (
	promptMarkerInitial = "[TEAM_PHASE:INITIAL]"
	promptMarkerReview  = "[TEAM_PHASE:REVIEW]"
	promptMarkerFinal   = "[TEAM_PHASE:FINAL]"
	promptMarkerMemory  = "[TEAM_PHASE:MEMORY]"
)

func (e *Engine) RunRound(ctx context.Context, question string) (RunResult, error) {
	if err := e.validate(); err != nil {
		return RunResult{}, err
	}
	lease, err := e.Store.AcquireLease()
	if err != nil {
		return RunResult{}, err
	}
	defer lease.Release()
	if _, err := e.Store.StartRound(question); err != nil {
		return RunResult{}, err
	}
	return e.runCurrentRound(ctx)
}

func (e *Engine) Resume(ctx context.Context) (RunResult, error) {
	if err := e.validate(); err != nil {
		return RunResult{}, err
	}
	lease, err := e.Store.AcquireLease()
	if err != nil {
		return RunResult{}, err
	}
	defer lease.Release()
	if err := e.Store.PrepareResume(); err != nil {
		return RunResult{}, err
	}
	return e.runCurrentRound(ctx)
}

func (e *Engine) validate() error {
	if e == nil {
		return fmt.Errorf("team engine is required")
	}
	if e.Definition == nil {
		return fmt.Errorf("team definition is required")
	}
	if err := e.Definition.Validate(); err != nil {
		return err
	}
	if e.Store == nil {
		return fmt.Errorf("team store is required")
	}
	if e.Processor == nil {
		return fmt.Errorf("team processor is required")
	}
	if e.Store.Thread.DefinitionHash != "" &&
		e.Definition.DefinitionHash != "" &&
		e.Store.Thread.DefinitionHash != e.Definition.DefinitionHash {
		return fmt.Errorf("team definition hash does not match the pinned thread definition")
	}
	return nil
}

func (e *Engine) runCurrentRound(ctx context.Context) (result RunResult, err error) {
	round := e.Store.State.Current
	if round == nil {
		return RunResult{}, fmt.Errorf("team thread has no current round")
	}
	runCtx := ctx
	cancel := func() {}
	if timeout, parseErr := time.ParseDuration(e.Definition.Limits.MaxWallTime); parseErr != nil {
		return RunResult{}, fmt.Errorf("invalid limits.max_wall_time %q: %w", e.Definition.Limits.MaxWallTime, parseErr)
	} else if timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()
	defer func() {
		if err == nil {
			return
		}
		if runCtx.Err() != nil {
			if runCtx.Err() == context.DeadlineExceeded {
				if state, stateErr := e.Store.Collaboration(); stateErr == nil && state != nil && state.StopReason == "" {
					_ = e.forceStopCollaboration(*state, stopReasonMaxWallTime)
				}
			}
			_ = e.Store.MarkInterrupted(runCtx.Err())
		} else {
			_ = e.Store.MarkFailed(err)
		}
	}()

	memory := MemoryDocument{
		Team:    e.Definition.Team,
		Version: 0,
		Content: "(team memory disabled for this run)",
		Path:    e.Store.Thread.MemoryPath,
	}
	if e.Definition.MemoryEnabled() && !e.DisableMemory {
		memory, err = LoadMemory(e.Store.Thread.MemoryPath, e.Definition.Team)
		if err != nil {
			return RunResult{}, err
		}
	}

	if err := e.runInitialWave(runCtx, memory); err != nil {
		return RunResult{}, err
	}
	if err := e.runAdaptiveReview(runCtx, memory); err != nil {
		return RunResult{}, err
	}
	if e.Definition.Verification.Enabled {
		events, loadErr := e.Store.Events()
		if loadErr != nil {
			return RunResult{}, loadErr
		}
		if !verificationSatisfied(events, round.Number) {
			return RunResult{}, fmt.Errorf("team delivery verification has not passed after the latest implementation")
		}
	}
	answer, err := e.runFinalization(runCtx, memory)
	if err != nil {
		return RunResult{}, err
	}
	if err := e.recordFinalTurn(); err != nil {
		return RunResult{}, err
	}

	memoryVersion := memory.Version
	var memoryWarning error
	if e.Definition.MemoryEnabled() && !e.DisableMemory {
		updated, updateErr := e.runMemoryMaintenance(runCtx, memory, answer)
		if updateErr != nil {
			memoryWarning = updateErr
			_ = e.appendEvent(Event{
				Type:  "memory_update_failed",
				Round: round.Number,
				Phase: PhaseMemory,
				From:  e.Definition.Memory.Maintainer,
				Error: updateErr.Error(),
			})
		} else {
			memory = updated
			memoryVersion = updated.Version
		}
	}
	if err := e.Store.CompleteRound(answer, memoryVersion); err != nil {
		return RunResult{}, err
	}
	return RunResult{
		ThreadID:      e.Store.Thread.ID,
		Round:         round.Number,
		Answer:        answer,
		Memory:        memory,
		MemoryWarning: memoryWarning,
	}, nil
}

func (e *Engine) runInitialWave(ctx context.Context, memory MemoryDocument) error {
	if err := e.Store.SetPhase(PhaseInitial, 0); err != nil {
		return err
	}
	for attempt := 1; attempt <= e.Definition.Limits.MaxReviewTurnsPerAgent; attempt++ {
		events, err := e.Store.Events()
		if err != nil {
			return err
		}
		completed := completedMembers(events, e.Store.State.Current.Number, PhaseInitial, 0)
		var pending []Agent
		for _, member := range e.Definition.Agents {
			if !completed[strings.ToLower(member.ID)] {
				pending = append(pending, member)
			}
		}
		if len(pending) == 0 {
			return nil
		}
		prompt := e.initialPrompt(memory, events)
		responses := e.callMembers(ctx, pending, func(member Agent) string {
			return strings.ReplaceAll(prompt, "{{MEMBER_CONTEXT}}", memberContext(member))
		})
		commitErr := e.commitResponses(PhaseInitial, 0, responses)
		if commitErr == nil {
			continue
		}
		if attempt == e.Definition.Limits.MaxReviewTurnsPerAgent {
			return fmt.Errorf("initial team wave still has failed members after %d attempts: %w", attempt, commitErr)
		}
		if err := e.appendEvent(Event{
			Type: "initial_retry", Round: e.Store.State.Current.Number, Phase: PhaseInitial,
			Content: fmt.Sprintf("retrying incomplete initial members (attempt %d)", attempt+1),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) runReviewActivations(ctx context.Context, memory MemoryDocument, wave int, activations []Activation) ([]agentResponse, error) {
	if err := e.Store.SetPhase(PhaseReview, wave); err != nil {
		return nil, err
	}
	events, err := e.Store.Events()
	if err != nil {
		return nil, err
	}
	members := make([]Agent, 0, len(activations))
	reasons := make(map[string]string, len(activations))
	for _, activation := range activations {
		member, ok := e.Definition.AgentByID(activation.MemberID)
		if !ok {
			continue
		}
		members = append(members, member)
		reasons[strings.ToLower(member.ID)] = activation.Reason
	}
	if len(members) == 0 {
		return nil, nil
	}
	responses := e.callMembers(ctx, members, func(member Agent) string {
		prompt := e.reviewPrompt(memory, events, wave, reasons[strings.ToLower(member.ID)])
		return strings.ReplaceAll(prompt, "{{MEMBER_CONTEXT}}", memberContext(member))
	})
	return responses, e.commitResponses(PhaseReview, wave, responses)
}

func (e *Engine) runFinalization(ctx context.Context, memory MemoryDocument) (string, error) {
	if err := e.Store.SetPhase(PhaseFinal, 0); err != nil {
		return "", err
	}
	events, err := e.Store.Events()
	if err != nil {
		return "", err
	}
	for _, event := range events {
		if event.Round == e.Store.State.Current.Number && event.Type == "final_answer" {
			return strings.TrimSpace(event.Content), nil
		}
	}
	member, ok := e.Definition.AgentByID(e.Definition.Coordination.Finalizer)
	if !ok {
		return "", fmt.Errorf("finalizer %q not found", e.Definition.Coordination.Finalizer)
	}
	response, err := e.callMember(ctx, member, e.finalPrompt(memory, events, member))
	if err != nil {
		_ = e.appendEvent(Event{
			Type:    "agent_error",
			Round:   e.Store.State.Current.Number,
			Phase:   PhaseFinal,
			From:    member.ID,
			Session: e.sessionFor(member.ID, ""),
			Error:   err.Error(),
			Metrics: &response.Metrics,
		})
		return "", fmt.Errorf("team finalizer %s failed: %w", member.ID, err)
	}
	answer := strings.TrimSpace(response.Content)
	if answer == "" {
		return "", fmt.Errorf("team finalizer %s returned an empty answer", member.ID)
	}
	event := Event{
		Type:    "final_answer",
		Round:   e.Store.State.Current.Number,
		Phase:   PhaseFinal,
		From:    member.ID,
		To:      []string{"user"},
		Session: response.Session,
		Content: answer,
		Metrics: &response.Metrics,
	}
	if err := e.appendEvent(event); err != nil {
		return "", err
	}
	return answer, nil
}

func (e *Engine) runMemoryMaintenance(ctx context.Context, previous MemoryDocument, answer string) (MemoryDocument, error) {
	if err := e.Store.SetPhase(PhaseMemory, 0); err != nil {
		return MemoryDocument{}, err
	}
	events, err := e.Store.Events()
	if err != nil {
		return MemoryDocument{}, err
	}
	for _, event := range events {
		if event.Round == e.Store.State.Current.Number && event.Type == "memory_updated" {
			return LoadMemory(e.Store.Thread.MemoryPath, e.Definition.Team)
		}
	}
	member, ok := e.Definition.AgentByID(e.Definition.Memory.Maintainer)
	if !ok {
		return MemoryDocument{}, fmt.Errorf("memory maintainer %q not found", e.Definition.Memory.Maintainer)
	}
	started := time.Now()
	model := e.modelFor(member)
	prompt := e.memoryPrompt(previous, events, answer, member)
	response, err := e.Processor.Process(ctx, AgentCall{
		MemberID:         member.ID,
		Agent:            member.Agent,
		Model:            model,
		Session:          e.sessionFor(member.ID, "memory"),
		Prompt:           prompt,
		External:         member.External,
		ExternalSessions: e.Store,
		Workspace:        e.Store.Thread.Workspace,
	})
	if err != nil {
		return MemoryDocument{}, fmt.Errorf("team memory maintainer %s failed: %w", member.ID, err)
	}
	var sourceEvents []int64
	for _, event := range events {
		if event.Round == e.Store.State.Current.Number {
			sourceEvents = append(sourceEvents, event.ID)
		}
	}
	proposal, err := ProposeMemory(
		e.Store.Thread.MemoryPath,
		e.Definition.Team,
		e.Store.Thread.ID,
		e.Store.State.Current.Number,
		sourceEvents,
		member.ID,
		response,
		e.Definition.Memory.MaxChars,
	)
	if err != nil {
		return MemoryDocument{}, err
	}
	if err := e.appendEvent(Event{
		Type:           "memory_proposed",
		Round:          e.Store.State.Current.Number,
		Phase:          PhaseMemory,
		From:           member.ID,
		Session:        e.sessionFor(member.ID, "memory"),
		MemoryProposal: proposal.ID,
		Content:        proposal.Diff,
		Metrics: &EventMetrics{
			DurationMS:  time.Since(started).Milliseconds(),
			Turn:        e.currentTurn(),
			Model:       model,
			InputChars:  len([]rune(prompt)),
			OutputChars: len([]rune(response)),
		},
	}); err != nil {
		return MemoryDocument{}, err
	}
	updated, err := PromoteMemory(e.Store.Thread.MemoryPath, proposal)
	if err != nil {
		return MemoryDocument{}, err
	}
	if err := e.appendEvent(Event{
		Type:           "memory_updated",
		Round:          e.Store.State.Current.Number,
		Phase:          PhaseMemory,
		From:           member.ID,
		Session:        e.sessionFor(member.ID, "memory"),
		MemoryProposal: proposal.ID,
		Content:        fmt.Sprintf("team memory upgraded to version %d", updated.Version),
	}); err != nil {
		return MemoryDocument{}, err
	}
	return updated, nil
}

func (e *Engine) RetryMemory(ctx context.Context) (MemoryDocument, error) {
	if err := e.validate(); err != nil {
		return MemoryDocument{}, err
	}
	lease, err := e.Store.AcquireLease()
	if err != nil {
		return MemoryDocument{}, err
	}
	defer lease.Release()
	round := e.Store.State.Current
	if round == nil || round.Status != RoundStatusCompleted || strings.TrimSpace(round.FinalAnswer) == "" {
		return MemoryDocument{}, fmt.Errorf("memory retry requires a completed team round")
	}
	events, err := e.Store.Events()
	if err != nil {
		return MemoryDocument{}, err
	}
	for _, event := range events {
		if event.Round == round.Number && event.Type == "memory_updated" {
			return MemoryDocument{}, fmt.Errorf("team memory was already updated for round %d", round.Number)
		}
	}
	previous, err := LoadMemory(e.Store.Thread.MemoryPath, e.Definition.Team)
	if err != nil {
		return MemoryDocument{}, err
	}
	updated, err := e.runMemoryMaintenance(ctx, previous, round.FinalAnswer)
	if err != nil {
		_ = e.appendEvent(Event{
			Type:  "memory_update_failed",
			Round: round.Number,
			Phase: PhaseMemory,
			From:  e.Definition.Memory.Maintainer,
			Error: err.Error(),
		})
		return MemoryDocument{}, err
	}
	if err := e.Store.SetMemoryVersion(updated.Version); err != nil {
		return MemoryDocument{}, err
	}
	return updated, nil
}

func (e *Engine) RollbackMemory(version int) (MemoryDocument, error) {
	if e == nil || e.Store == nil || e.Definition == nil {
		return MemoryDocument{}, fmt.Errorf("team runtime is unavailable")
	}
	lease, err := e.Store.AcquireLease()
	if err != nil {
		return MemoryDocument{}, err
	}
	defer lease.Release()
	round := 0
	if e.Store.State.Current != nil {
		round = e.Store.State.Current.Number
	}
	updated, err := RollbackMemory(e.Store.Thread.MemoryPath, e.Definition.Team, e.Store.Thread.ID, round, version)
	if err != nil {
		return MemoryDocument{}, err
	}
	if round > 0 {
		if err := e.Store.SetMemoryVersion(updated.Version); err != nil {
			return MemoryDocument{}, err
		}
	}
	if err := e.appendEvent(Event{
		Type:    "memory_rolled_back",
		Round:   round,
		Phase:   PhaseMemory,
		From:    "user",
		Content: fmt.Sprintf("team memory restored from version %d as version %d", version, updated.Version),
	}); err != nil {
		return MemoryDocument{}, err
	}
	return updated, nil
}

func (e *Engine) callMembers(ctx context.Context, members []Agent, prompt func(Agent) string) []agentResponse {
	responses := make([]agentResponse, len(members))
	maxConcurrency := e.Definition.Coordination.MaxConcurrency
	if maxConcurrency > len(members) {
		maxConcurrency = len(members)
	}
	if maxConcurrency < 1 {
		maxConcurrency = 1
	}
	semaphore := make(chan struct{}, maxConcurrency)
	var wait sync.WaitGroup
	for i, member := range members {
		i, member := i, member
		wait.Add(1)
		go func() {
			defer wait.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				responses[i] = agentResponse{Member: member, Err: ctx.Err()}
				return
			}
			responses[i], _ = e.callMember(ctx, member, prompt(member))
		}()
	}
	wait.Wait()
	return responses
}

func (e *Engine) callMember(ctx context.Context, member Agent, prompt string) (agentResponse, error) {
	session := e.sessionFor(member.ID, "")
	model := e.modelFor(member)
	if e.Definition.Verification.Enabled &&
		strings.EqualFold(member.ID, e.Definition.Verification.Verifier) &&
		strings.Contains(prompt, promptMarkerReview) {
		prompt += "\n\n" + verificationInstructions()
	}
	started := time.Now()
	phase := phaseFromPrompt(prompt)
	wave := 0
	if e.Store.State.Current != nil {
		wave = e.Store.State.Current.ReviewWave
	}
	_ = e.appendEvent(Event{
		Type: "agent_started", Round: e.Store.State.Current.Number, Phase: phase, Wave: wave,
		From: "runtime", To: []string{member.ID}, Session: session,
	})
	content, err := e.Processor.Process(ctx, AgentCall{
		MemberID:         member.ID,
		Agent:            member.Agent,
		Model:            model,
		Session:          session,
		Prompt:           prompt,
		External:         member.External,
		ExternalSessions: e.Store,
		Workspace:        e.Store.Thread.Workspace,
	})
	_ = e.appendEvent(Event{
		Type: "agent_completed", Round: e.Store.State.Current.Number, Phase: phase, Wave: wave,
		From: "runtime", To: []string{member.ID}, Session: session,
		Content: fallbackErrorStatus(err), Metrics: &EventMetrics{DurationMS: time.Since(started).Milliseconds(), Model: model},
	})
	cleanContent, verificationRequest := parseVerificationRequest(content)
	cleanContent, blackboard := parseBlackboardOperations(cleanContent)
	cleanContent, signal, _ := parseCollaborationResponse(cleanContent, e.Definition)
	var verification []VerificationResult
	var verificationErr error
	if strings.Contains(prompt, promptMarkerReview) {
		verification, verificationErr = e.runVerification(ctx, member, verificationRequest)
	}
	if verificationErr != nil {
		signal = SignalObject
		cleanContent = strings.TrimSpace(cleanContent + "\n\n运行时独立验证失败：" + verificationErr.Error())
	}
	cleanContent = dedupeResponseParagraphs(cleanContent)
	cleanContent = limitTeamResponse(cleanContent, e.Definition.Limits.MaxResponseChars)
	targets := mentionedMembers(cleanContent, e.Definition)
	response := agentResponse{
		Member:       member,
		Session:      session,
		Content:      cleanContent,
		Signal:       signal,
		Targets:      targets,
		Blackboard:   blackboard,
		Verification: verification,
		Metrics: EventMetrics{
			DurationMS:  time.Since(started).Milliseconds(),
			Turn:        e.currentTurn(),
			Model:       model,
			InputChars:  len([]rune(prompt)),
			OutputChars: len([]rune(content)),
		},
		Err: err,
	}
	if verificationErr != nil {
		response.Targets = []string{firstNonEmpty(
			e.Definition.Coordination.DeliveryOwner,
			e.Definition.Coordination.InitialHandoff,
			e.Definition.Coordination.Facilitator,
		)}
	}
	return response, err
}

func phaseFromPrompt(prompt string) string {
	switch {
	case strings.Contains(prompt, promptMarkerReview):
		return PhaseReview
	case strings.Contains(prompt, promptMarkerFinal):
		return PhaseFinal
	case strings.Contains(prompt, promptMarkerMemory):
		return PhaseMemory
	default:
		return PhaseInitial
	}
}

func fallbackErrorStatus(err error) string {
	if err != nil {
		return "failed"
	}
	return "completed"
}

func dedupeResponseParagraphs(content string) string {
	paragraphs := strings.Split(strings.ReplaceAll(strings.TrimSpace(content), "\r\n", "\n"), "\n\n")
	seen := map[string]bool{}
	result := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}
		key := strings.Join(strings.Fields(paragraph), " ")
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, paragraph)
	}
	return strings.Join(result, "\n\n")
}

func limitTeamResponse(content string, maxChars int) string {
	content = strings.TrimSpace(content)
	if maxChars <= 0 {
		return content
	}
	runes := []rune(content)
	if len(runes) <= maxChars {
		return content
	}
	return strings.TrimSpace(string(runes[:maxChars])) + "\n\n[内容因达到 Team 单次回复长度限制而截断]"
}

func (e *Engine) commitResponses(phase string, wave int, responses []agentResponse) error {
	var firstErr error
	for i := range responses {
		response := &responses[i]
		if response.Err != nil {
			event := Event{
				Type:    "agent_error",
				Round:   e.Store.State.Current.Number,
				Phase:   phase,
				Wave:    wave,
				From:    response.Member.ID,
				Session: response.Session,
				Error:   response.Err.Error(),
				Metrics: &response.Metrics,
			}
			_ = e.appendEvent(event)
			if firstErr == nil {
				firstErr = fmt.Errorf("team agent %s failed: %w", response.Member.ID, response.Err)
			}
			continue
		}
		if response.Content == "" && response.Signal != SignalYield && len(response.Blackboard) == 0 {
			emptyErr := fmt.Errorf("team agent %s returned an empty response", response.Member.ID)
			_ = e.appendEvent(Event{
				Type:    "agent_error",
				Round:   e.Store.State.Current.Number,
				Phase:   phase,
				Wave:    wave,
				From:    response.Member.ID,
				Session: response.Session,
				Error:   emptyErr.Error(),
				Metrics: &response.Metrics,
			})
			if firstErr == nil {
				firstErr = emptyErr
			}
			continue
		}
		if response.Signal == SignalObject && len(response.Targets) == 0 {
			response.Targets = e.defaultObjectionTargets(response.Member.ID)
		}
		eventType := "agent_message"
		if response.Signal == SignalYield {
			eventType = "agent_yield"
		}
		content := response.Content
		if eventType == "agent_yield" && content == "" {
			content = "[YIELD]"
		} else if content == "" && len(response.Blackboard) > 0 {
			content = "[BLACKBOARD UPDATE]"
		}
		targets := response.Targets
		if len(targets) == 0 {
			targets = []string{"team"}
		}
		sourceEventID, err := e.appendEventWithID(Event{
			Type:    eventType,
			Round:   e.Store.State.Current.Number,
			Phase:   phase,
			Wave:    wave,
			From:    response.Member.ID,
			To:      targets,
			Session: response.Session,
			Signal:  string(response.Signal),
			Content: content,
			Metrics: &response.Metrics,
		})
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, operation := range response.Blackboard {
			operation := operation
			if err := e.appendEvent(Event{
				Type:       "blackboard_" + operation.Action,
				Round:      e.Store.State.Current.Number,
				Phase:      phase,
				Wave:       wave,
				From:       response.Member.ID,
				To:         []string{"team"},
				Ref:        sourceEventID,
				Blackboard: &operation,
				Content:    operation.Content,
			}); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		for _, verification := range response.Verification {
			verification := verification
			eventType := "verification_passed"
			if verification.ExitCode != 0 {
				eventType = "verification_failed"
			}
			if err := e.appendEvent(Event{
				Type: eventType, Round: e.Store.State.Current.Number, Phase: phase, Wave: wave,
				From: response.Member.ID, To: []string{"team"}, Ref: sourceEventID,
				Verification: &verification, Content: strings.Join(verification.Command, " "),
			}); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (e *Engine) currentTurn() int {
	if e == nil || e.Store == nil || e.Store.State.Current == nil || e.Store.State.Current.Collaboration == nil {
		return 0
	}
	return e.Store.State.Current.Collaboration.TurnCount
}

func (e *Engine) modelFor(member Agent) string {
	if e == nil {
		return strings.TrimSpace(member.Model)
	}
	defaultModel := ""
	if e.Definition != nil {
		defaultModel = e.Definition.DefaultModel
	}
	return firstNonEmpty(member.Model, e.Model, defaultModel)
}

func (e *Engine) appendEvent(event Event) error {
	_, err := e.appendEventWithID(event)
	return err
}

func (e *Engine) appendEventWithID(event Event) (int64, error) {
	id, err := e.Store.AppendEventWithID(event)
	if err != nil {
		return 0, err
	}
	event.ID = id
	if e.OnEvent != nil {
		e.OnEvent(event)
	}
	return id, nil
}

func (e *Engine) sessionFor(memberID, purpose string) string {
	prefix := strings.Trim(strings.TrimSpace(e.SessionPrefix), ":")
	if prefix == "" {
		prefix = "cli:team"
	}
	thread := strings.ReplaceAll(e.Store.Thread.ID, "/", ":")
	session := prefix + ":" + thread + ":agent:" + strings.ToLower(strings.TrimSpace(memberID))
	if strings.TrimSpace(purpose) != "" {
		session += ":" + strings.ToLower(strings.TrimSpace(purpose))
	}
	return session
}

func (e *Engine) initialPrompt(memory MemoryDocument, events []Event) string {
	return fmt.Sprintf(`[TEAM_PHASE:INITIAL]
你正在参与一个持久化的专业团队协作。

团队: %s
团队成员:
%s

{{MEMBER_CONTEXT}}

%s

当前用户问题（本轮唯一有效目标，优先级高于记忆和历史）:
%s

运行时自动记录的工作区基线:
%s

本轮共享工作黑板:
%s

团队共享记忆 (版本 %d，仅作为可能过时的背景):
%s

本线程之前的相关轮次:
%s

请给出你独立的专业判断。
- 只处理当前用户问题。不得把记忆或历史中的旧任务当作本轮待办。
- 如果当前问题宽泛但可以通过仓库检查安全收敛，请选择一个有证据、边界明确、
  高价值且低风险的默认方案继续推进，不要把非关键选择退回给用户。
- 自然、深入地表达观点。
- 可以用 @成员id 的方式向其他成员提问。
- 提出假设、风险、缺失信息和具体建议。
- 不要假设其他成员已经在这个轮次中回答过。
- 除非你的角色特别要求，否则不要撰写面向用户的最终总结。

%s

%s
`, e.Definition.TitleOrName(), e.teamDirectory(), e.languageInstruction(), e.Store.State.Current.Question, e.workspaceBaselineContext(), formatBlackboard(events, e.Store.State.Current.Number), memory.Version, stableMemoryContext(memory.Content), priorRoundContext(events, e.Store.State.Current.Number), collaborationSignalInstructions(), blackboardInstructions())
}

func (e *Engine) reviewPrompt(memory MemoryDocument, events []Event, wave int, reason string) string {
	return fmt.Sprintf(`[TEAM_PHASE:REVIEW]
你正在参与第 %d 轮同行评审。

团队: %s
{{MEMBER_CONTEXT}}

%s

当前用户问题（本轮唯一有效目标，优先级高于记忆和历史）:
%s

本轮共享工作黑板:
%s

团队共享记忆 (版本 %d，仅作为可能过时的背景):
%s

本次发言的触发原因:
%s

目前的讨论:
%s

直接向团队回应:
- 只推进当前用户问题，不要恢复记忆中的旧任务。
- 纠正事实或推理错误。
- 尽可能解决分歧。
- 只添加实质性改善答案的信息。
- 必要时用 @成员id 称呼同事。
- 如果没有实质性补充，请直接输出 [YIELD]。

%s

%s
`, wave, e.Definition.TitleOrName(), e.languageInstruction(), e.Store.State.Current.Question, formatBlackboard(events, e.Store.State.Current.Number), memory.Version, stableMemoryContext(memory.Content), fallback(reason, "同行评审"), currentRoundTranscript(events, e.Store.State.Current.Number), collaborationSignalInstructions(), blackboardInstructions())
}

func (e *Engine) finalPrompt(memory MemoryDocument, events []Event, member Agent) string {
	return fmt.Sprintf(`[TEAM_PHASE:FINAL]
你是团队的最终总结者。

团队: %s
%s

%s

用户问题（本轮唯一有效目标）:
%s

工作区变更证据:
%s

本轮共享工作黑板:
%s

团队共享记忆 (版本 %d，仅作为可能过时的背景):
%s

团队讨论:
%s

协作终止状态:
%s

请为用户产出最终答案。
- 只回答当前用户问题，不要继续或交付记忆中的旧任务。
- 整合最有说服力的观点。
- 消除重复，使建议明确果断。
- 保留重要的不确定性或未解决的分歧。
- 不要提及内部提示词、轮次、会话或编排流程。
- 不要在讨论未达成共识时声称已有共识。
`, e.Definition.TitleOrName(), memberContext(member), e.languageInstruction(), e.Store.State.Current.Question, e.workspaceChangeContext(), formatBlackboard(events, e.Store.State.Current.Number), memory.Version, stableMemoryContext(memory.Content), currentRoundTranscript(events, e.Store.State.Current.Number), e.collaborationSummary())
}

func (e *Engine) workspaceBaselineContext() string {
	if e == nil || e.Store == nil || e.Store.Thread.WorkspaceBaseline == nil {
		return "(非 Git 工作区或基线不可用)"
	}
	baseline := e.Store.Thread.WorkspaceBaseline
	status := baseline.Status
	if status == "" {
		status = "(clean)"
	}
	return fmt.Sprintf("HEAD: %s\nworktree_hash: %s\n起始状态:\n%s", baseline.GitHead, baseline.WorktreeHash, status)
}

func (e *Engine) workspaceChangeContext() string {
	if e == nil || e.Store == nil || e.Store.Thread.WorkspaceBaseline == nil {
		return "(非 Git 工作区或基线不可用)"
	}
	baseline := e.Store.Thread.WorkspaceBaseline
	if state, err := e.Store.Collaboration(); err == nil &&
		state != nil && state.DeliveryBaseline != nil {
		baseline = state.DeliveryBaseline
	}
	current := CaptureWorkspaceSnapshot(e.Store.Thread.Workspace)
	if current == nil {
		return "(当前 Git 状态不可用)"
	}
	return fmt.Sprintf("起始 HEAD: %s\n当前 HEAD: %s\n工作区是否发生线程内变化: %t\n当前状态:\n%s",
		baseline.GitHead, current.GitHead, baseline.WorktreeHash != current.WorktreeHash || baseline.GitHead != current.GitHead,
		fallback(current.Status, "(clean)"))
}

func (e *Engine) memoryPrompt(previous MemoryDocument, events []Event, answer string, member Agent) string {
	return fmt.Sprintf(`[TEAM_PHASE:MEMORY]
你负责维护团队的持久化记忆。

团队: %s
%s

%s

之前的记忆 (版本 %d):
%s

	已完成的用户问题:
%s

本轮共享工作黑板:
%s

公开的团队讨论:
%s

最终答案:
%s

请将完整的团队记忆重写为 Markdown 格式。
文档必须只包含以下长期稳定章节（没有内容的章节可以省略）:
- ## Stable User Preferences
- ## Stable Repository Facts
- ## Durable Decisions
- ## Team Working Agreements

不得保存活动任务、未完成任务、下一步、当前关注点或具体用户请求。已完成或中断的
具体请求也不得作为下一轮待办；新的当前用户问题始终是唯一活动目标。
不要存储隐藏的推理过程、临时闲聊、访问令牌、密码、私钥或未经验证的推测。
不要包含 front matter 或版本号；运行时会自动管理这些。
只返回完整的 Markdown 记忆文档。
`, e.Definition.TitleOrName(), memberContext(member), e.languageInstruction(), previous.Version, previous.Content, e.Store.State.Current.Question, formatBlackboard(events, e.Store.State.Current.Number), currentRoundTranscript(events, e.Store.State.Current.Number), answer)
}

func (e *Engine) languageInstruction() string {
	if e == nil || e.Definition == nil {
		return ""
	}
	var instructions []string
	if language := strings.TrimSpace(e.Definition.Language); language != "" {
		instructions = append(instructions, fmt.Sprintf(`输出语言要求（强制）:
- 所有面向用户或团队成员的自然语言内容必须使用%s。
- 分析、建议、评审、总结和记忆文档都必须遵守此要求。
- 代码、命令、文件路径、API 名称和必要的技术标识符可以保留原文。
- 即使用户或其他成员使用不同语言，也不要改变输出语言。`, language))
	}
	if maxChars := e.Definition.Limits.MaxResponseChars; maxChars > 0 {
		instructions = append(instructions, fmt.Sprintf(`回复长度要求（强制）:
- 正文不得超过 %d 个字符。
- 引用黑板和既有讨论时只提炼新增结论，不要复述已有长段落。
- 优先输出可执行动作、产物和阻塞证据。`, maxChars))
	}
	return strings.Join(instructions, "\n\n")
}

func (d Definition) TitleOrName() string {
	if strings.TrimSpace(d.Title) != "" {
		return d.Title
	}
	return d.Team
}

func (e *Engine) teamDirectory() string {
	lines := make([]string, 0, len(e.Definition.Agents))
	for _, member := range e.Definition.Agents {
		role := strings.TrimSpace(member.Role)
		if role == "" {
			role = "team member"
		}
		lines = append(lines, fmt.Sprintf("- @%s: %s", member.ID, role))
	}
	return strings.Join(lines, "\n")
}

func memberContext(member Agent) string {
	role := strings.TrimSpace(member.Role)
	if role == "" {
		role = "团队成员"
	}
	return fmt.Sprintf(`你是 @%s。
角色: %s
工作指令:
%s`, member.ID, role, fallback(member.Prompt, "运用你的专业知识，与其他成员协作完成任务。"))
}

func completedMembers(events []Event, round int, phase string, wave int) map[string]bool {
	completed := map[string]bool{}
	for _, event := range events {
		if event.Round != round || event.Phase != phase || event.Wave != wave {
			continue
		}
		switch event.Type {
		case "agent_message", "agent_yield":
			completed[strings.ToLower(event.From)] = true
		}
	}
	return completed
}

func priorRoundContext(events []Event, currentRound int) string {
	type pair struct {
		question string
		answer   string
	}
	rounds := map[int]*pair{}
	for _, event := range events {
		if event.Round <= 0 || event.Round >= currentRound {
			continue
		}
		item := rounds[event.Round]
		if item == nil {
			item = &pair{}
			rounds[event.Round] = item
		}
		switch event.Type {
		case "user_message":
			item.question = event.Content
		case "final_answer":
			item.answer = event.Content
		}
	}
	var keys []int
	for round := range rounds {
		keys = append(keys, round)
	}
	sort.Ints(keys)
	if len(keys) > 3 {
		keys = keys[len(keys)-3:]
	}
	var builder strings.Builder
	for _, round := range keys {
		item := rounds[round]
		fmt.Fprintf(&builder, "Round %d user: %s\nRound %d answer: %s\n\n", round, compact(item.question, 1500), round, compact(item.answer, 3000))
	}
	if builder.Len() == 0 {
		return "(none)"
	}
	return strings.TrimSpace(builder.String())
}

func currentRoundTranscript(events []Event, round int) string {
	var builder strings.Builder
	for _, event := range events {
		if event.Round != round {
			continue
		}
		switch event.Type {
		case "user_message":
			fmt.Fprintf(&builder, "user: %s\n\n", event.Content)
		case "agent_message":
			signal := ""
			if event.Signal != "" {
				signal = " [" + event.Signal + "]"
			}
			fmt.Fprintf(&builder, "@%s%s: %s\n\n", event.From, signal, event.Content)
		case "agent_yield":
			fmt.Fprintf(&builder, "@%s: [YIELD]\n\n", event.From)
		case "agent_error":
			fmt.Fprintf(&builder, "@%s: [ERROR] %s\n\n", event.From, event.Error)
		}
	}
	if builder.Len() == 0 {
		return "(none)"
	}
	return compactTail(strings.TrimSpace(builder.String()), 18000)
}

func isYield(content string) bool {
	content = strings.ToUpper(strings.TrimSpace(content))
	return content == "YIELD" || content == "[YIELD]"
}

func compact(value string, max int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if max <= 0 || len(runes) <= max {
		return value
	}
	return strings.TrimSpace(string(runes[:max])) + "\n[truncated]"
}

func compactTail(value string, max int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if max <= 0 || len(runes) <= max {
		return value
	}
	return "[earlier discussion truncated]\n" + strings.TrimSpace(string(runes[len(runes)-max:]))
}

func fallback(value, fallbackValue string) string {
	if strings.TrimSpace(value) == "" {
		return fallbackValue
	}
	return strings.TrimSpace(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
