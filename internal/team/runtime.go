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

type AgentCall struct {
	MemberID string
	Agent    string
	Model    string
	Session  string
	Prompt   string
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
	Member     Agent
	Session    string
	Content    string
	Signal     CollaborationSignal
	Targets    []string
	Blackboard []BlackboardOperation
	Metrics    EventMetrics
	Err        error
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
	return e.commitResponses(PhaseInitial, 0, responses)
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
		MemberID: member.ID,
		Agent:    member.Agent,
		Model:    model,
		Session:  e.sessionFor(member.ID, "memory"),
		Prompt:   prompt,
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
	started := time.Now()
	content, err := e.Processor.Process(ctx, AgentCall{
		MemberID: member.ID,
		Agent:    member.Agent,
		Model:    model,
		Session:  session,
		Prompt:   prompt,
	})
	cleanContent, blackboard := parseBlackboardOperations(content)
	cleanContent, signal, targets := parseCollaborationResponse(cleanContent, e.Definition)
	response := agentResponse{
		Member:     member,
		Session:    session,
		Content:    cleanContent,
		Signal:     signal,
		Targets:    targets,
		Blackboard: blackboard,
		Metrics: EventMetrics{
			DurationMS:  time.Since(started).Milliseconds(),
			Turn:        e.currentTurn(),
			Model:       model,
			InputChars:  len([]rune(prompt)),
			OutputChars: len([]rune(content)),
		},
		Err: err,
	}
	return response, err
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

团队共享记忆 (版本 %d):
%s

本轮共享工作黑板:
%s

本线程之前的相关轮次:
%s

当前用户问题:
%s

请给出你独立的专业判断。
- 自然、深入地表达观点。
- 可以用 @成员id 的方式向其他成员提问。
- 提出假设、风险、缺失信息和具体建议。
- 不要假设其他成员已经在这个轮次中回答过。
- 除非你的角色特别要求，否则不要撰写面向用户的最终总结。

%s

%s
`, e.Definition.TitleOrName(), e.teamDirectory(), memory.Version, memory.Content, formatBlackboard(events, e.Store.State.Current.Number), priorRoundContext(events, e.Store.State.Current.Number), e.Store.State.Current.Question, collaborationSignalInstructions(), blackboardInstructions())
}

func (e *Engine) reviewPrompt(memory MemoryDocument, events []Event, wave int, reason string) string {
	return fmt.Sprintf(`[TEAM_PHASE:REVIEW]
你正在参与第 %d 轮同行评审。

团队: %s
{{MEMBER_CONTEXT}}

团队共享记忆 (版本 %d):
%s

本轮共享工作黑板:
%s

当前用户问题:
%s

本次发言的触发原因:
%s

目前的讨论:
%s

直接向团队回应:
- 纠正事实或推理错误。
- 尽可能解决分歧。
- 只添加实质性改善答案的信息。
- 必要时用 @成员id 称呼同事。
- 如果没有实质性补充，请直接输出 [YIELD]。

%s

%s
`, wave, e.Definition.TitleOrName(), memory.Version, memory.Content, formatBlackboard(events, e.Store.State.Current.Number), e.Store.State.Current.Question, fallback(reason, "同行评审"), currentRoundTranscript(events, e.Store.State.Current.Number), collaborationSignalInstructions(), blackboardInstructions())
}

func (e *Engine) finalPrompt(memory MemoryDocument, events []Event, member Agent) string {
	return fmt.Sprintf(`[TEAM_PHASE:FINAL]
你是团队的最终总结者。

团队: %s
%s

团队共享记忆 (版本 %d):
%s

本轮共享工作黑板:
%s

用户问题:
%s

团队讨论:
%s

协作终止状态:
%s

请为用户产出最终答案。
- 整合最有说服力的观点。
- 消除重复，使建议明确果断。
- 保留重要的不确定性或未解决的分歧。
- 不要提及内部提示词、轮次、会话或编排流程。
- 不要在讨论未达成共识时声称已有共识。
`, e.Definition.TitleOrName(), memberContext(member), memory.Version, memory.Content, formatBlackboard(events, e.Store.State.Current.Number), e.Store.State.Current.Question, currentRoundTranscript(events, e.Store.State.Current.Number), e.collaborationSummary())
}

func (e *Engine) memoryPrompt(previous MemoryDocument, events []Event, answer string, member Agent) string {
	return fmt.Sprintf(`[TEAM_PHASE:MEMORY]
你负责维护团队的持久化记忆。

团队: %s
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
只保留对未来轮次有用的持久化信息，例如:
- 稳定的用户偏好和约束;
- 已验证的事实和经常出现的项目上下文;
- 决策及其理由;
- 未解决的风险或后续承诺;
- 改善团队协作的工作约定。

不要存储隐藏的推理过程、临时闲聊、访问令牌、密码、私钥或未经验证的推测。
不要包含 front matter 或版本号；运行时会自动管理这些。
只返回完整的 Markdown 记忆文档。
`, e.Definition.TitleOrName(), memberContext(member), previous.Version, previous.Content, e.Store.State.Current.Question, formatBlackboard(events, e.Store.State.Current.Number), currentRoundTranscript(events, e.Store.State.Current.Number), answer)
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
		}
	}
	if builder.Len() == 0 {
		return "(none)"
	}
	return compact(strings.TrimSpace(builder.String()), 30000)
}

func isYield(content string) bool {
	content = strings.ToUpper(strings.TrimSpace(content))
	return content == "YIELD" || content == "[YIELD]"
}

func compact(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return strings.TrimSpace(value[:max]) + "\n[truncated]"
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
