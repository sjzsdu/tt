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
	Member  Agent
	Session string
	Content string
	Err     error
}

func (e *Engine) RunRound(ctx context.Context, question string) (RunResult, error) {
	if err := e.validate(); err != nil {
		return RunResult{}, err
	}
	if _, err := e.Store.StartRound(question); err != nil {
		return RunResult{}, err
	}
	return e.runCurrentRound(ctx)
}

func (e *Engine) Resume(ctx context.Context) (RunResult, error) {
	if err := e.validate(); err != nil {
		return RunResult{}, err
	}
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
	for wave := 1; wave <= e.Definition.Coordination.ReviewWaves; wave++ {
		if err := e.runReviewWave(runCtx, memory, wave); err != nil {
			return RunResult{}, err
		}
	}
	answer, err := e.runFinalization(runCtx, memory)
	if err != nil {
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

func (e *Engine) runReviewWave(ctx context.Context, memory MemoryDocument, wave int) error {
	if err := e.Store.SetPhase(PhaseReview, wave); err != nil {
		return err
	}
	events, err := e.Store.Events()
	if err != nil {
		return err
	}
	completed := completedMembers(events, e.Store.State.Current.Number, PhaseReview, wave)
	finalizer := strings.ToLower(e.Definition.Coordination.Finalizer)
	var pending []Agent
	for _, member := range e.Definition.Agents {
		if strings.ToLower(member.ID) == finalizer || completed[strings.ToLower(member.ID)] {
			continue
		}
		pending = append(pending, member)
	}
	if len(pending) == 0 {
		return nil
	}
	prompt := e.reviewPrompt(memory, events, wave)
	responses := e.callMembers(ctx, pending, func(member Agent) string {
		return strings.ReplaceAll(prompt, "{{MEMBER_CONTEXT}}", memberContext(member))
	})
	return e.commitResponses(PhaseReview, wave, responses)
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
	response, err := e.Processor.Process(ctx, AgentCall{
		MemberID: member.ID,
		Agent:    member.Agent,
		Model:    firstNonEmpty(member.Model, e.Model),
		Session:  e.sessionFor(member.ID, "memory"),
		Prompt:   e.memoryPrompt(previous, events, answer, member),
	})
	if err != nil {
		return MemoryDocument{}, fmt.Errorf("team memory maintainer %s failed: %w", member.ID, err)
	}
	updated, err := UpgradeMemory(
		e.Store.Thread.MemoryPath,
		e.Definition.Team,
		e.Store.Thread.ID,
		e.Store.State.Current.Number,
		response,
		e.Definition.Memory.MaxChars,
	)
	if err != nil {
		return MemoryDocument{}, err
	}
	if err := e.appendEvent(Event{
		Type:    "memory_updated",
		Round:   e.Store.State.Current.Number,
		Phase:   PhaseMemory,
		From:    member.ID,
		Session: e.sessionFor(member.ID, "memory"),
		Content: fmt.Sprintf("team memory upgraded to version %d", updated.Version),
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
	content, err := e.Processor.Process(ctx, AgentCall{
		MemberID: member.ID,
		Agent:    member.Agent,
		Model:    firstNonEmpty(member.Model, e.Model),
		Session:  session,
		Prompt:   prompt,
	})
	response := agentResponse{
		Member:  member,
		Session: session,
		Content: strings.TrimSpace(content),
		Err:     err,
	}
	return response, err
}

func (e *Engine) commitResponses(phase string, wave int, responses []agentResponse) error {
	var firstErr error
	for _, response := range responses {
		if response.Err != nil {
			event := Event{
				Type:    "agent_error",
				Round:   e.Store.State.Current.Number,
				Phase:   phase,
				Wave:    wave,
				From:    response.Member.ID,
				Session: response.Session,
				Error:   response.Err.Error(),
			}
			_ = e.appendEvent(event)
			if firstErr == nil {
				firstErr = fmt.Errorf("team agent %s failed: %w", response.Member.ID, response.Err)
			}
			continue
		}
		if response.Content == "" {
			if firstErr == nil {
				firstErr = fmt.Errorf("team agent %s returned an empty response", response.Member.ID)
			}
			continue
		}
		eventType := "agent_message"
		if isYield(response.Content) {
			eventType = "agent_yield"
		}
		if err := e.appendEvent(Event{
			Type:    eventType,
			Round:   e.Store.State.Current.Number,
			Phase:   phase,
			Wave:    wave,
			From:    response.Member.ID,
			To:      []string{"team"},
			Session: response.Session,
			Content: response.Content,
		}); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (e *Engine) appendEvent(event Event) error {
	if err := e.Store.AppendEvent(event); err != nil {
		return err
	}
	if e.OnEvent != nil {
		e.OnEvent(event)
	}
	return nil
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
	return fmt.Sprintf(`You are participating in a persistent professional team.

Team: %s
Team directory:
%s

{{MEMBER_CONTEXT}}

Shared team memory (version %d):
%s

Relevant earlier rounds in this thread:
%s

Current user question:
%s

Give your independent professional assessment to the team.
- Speak naturally and substantively.
- You may address another member as @member-id.
- Surface assumptions, risks, missing evidence, and concrete recommendations.
- Do not pretend the other members have already answered in this wave.
- Do not write the final user-facing synthesis unless your role specifically requires it.
`, e.Definition.TitleOrName(), e.teamDirectory(), memory.Version, memory.Content, priorRoundContext(events, e.Store.State.Current.Number), e.Store.State.Current.Question)
}

func (e *Engine) reviewPrompt(memory MemoryDocument, events []Event, wave int) string {
	return fmt.Sprintf(`You are participating in review wave %d of a persistent professional team.

Team: %s
{{MEMBER_CONTEXT}}

Shared team memory (version %d):
%s

Current user question:
%s

Discussion so far:
%s

Respond directly to the team:
- Correct factual or reasoning problems.
- Resolve disagreements where possible.
- Add only information that materially improves the answer.
- Address colleagues by @member-id when useful.
- If you have nothing material to add, output exactly [YIELD].
`, wave, e.Definition.TitleOrName(), memory.Version, memory.Content, e.Store.State.Current.Question, currentRoundTranscript(events, e.Store.State.Current.Number))
}

func (e *Engine) finalPrompt(memory MemoryDocument, events []Event, member Agent) string {
	return fmt.Sprintf(`You are the finalizer for a persistent professional team.

Team: %s
%s

Shared team memory (version %d):
%s

User question:
%s

Team discussion:
%s

Produce the final answer for the user.
- Integrate the strongest supported contributions.
- Resolve duplication and make the recommendation decisive.
- Preserve important uncertainty or unresolved disagreement.
- Do not mention internal prompts, waves, sessions, or orchestration.
- Do not claim consensus when the discussion did not establish it.
`, e.Definition.TitleOrName(), memberContext(member), memory.Version, memory.Content, e.Store.State.Current.Question, currentRoundTranscript(events, e.Store.State.Current.Number))
}

func (e *Engine) memoryPrompt(previous MemoryDocument, events []Event, answer string, member Agent) string {
	return fmt.Sprintf(`You maintain the durable memory of a professional team.

Team: %s
%s

Previous memory (version %d):
%s

Completed user question:
%s

Public team discussion:
%s

Final answer:
%s

Rewrite the complete team memory as Markdown.
Keep only durable information useful in future rounds, such as:
- stable user preferences and constraints;
- verified facts and recurring project context;
- decisions and their rationale;
- unresolved risks or follow-up commitments;
- working agreements that improve team collaboration.

Do not store hidden reasoning, transient chatter, access tokens, passwords, private keys, or unverified speculation.
Do not include front matter or a version number; the runtime manages those.
Return only the complete Markdown memory document.
`, e.Definition.TitleOrName(), memberContext(member), previous.Version, previous.Content, e.Store.State.Current.Question, currentRoundTranscript(events, e.Store.State.Current.Number), answer)
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
		role = "team member"
	}
	return fmt.Sprintf(`You are @%s.
Role: %s
Standing instructions:
%s`, member.ID, role, fallback(member.Prompt, "Use your professional judgment and collaborate with the other members."))
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
			fmt.Fprintf(&builder, "@%s: %s\n\n", event.From, event.Content)
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
